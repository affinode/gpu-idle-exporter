package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"

	"github.com/affinode/gpu-idle-exporter/internal/collector"
	"github.com/affinode/gpu-idle-exporter/internal/exporter"
	"github.com/affinode/gpu-idle-exporter/internal/idle"
)

func main() {
	// Parse configuration from environment
	pollInterval := getEnvDuration("POLL_INTERVAL", 5*time.Second)
	httpPort := getEnvOrDefault("HTTP_PORT", "9835")

	log.Printf("GPU Idle Metrics Exporter starting (poll=%v, port=%s)", pollInterval, httpPort)

	// Initialize NVML
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		log.Fatalf("Failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	defer nvml.Shutdown()
	log.Println("NVML initialized successfully")

	// Log GPU info
	count, ret := nvml.DeviceGetCount()
	if ret == nvml.SUCCESS {
		log.Printf("Found %d GPU(s)", count)
		for i := 0; i < count; i++ {
			if device, ret := nvml.DeviceGetHandleByIndex(i); ret == nvml.SUCCESS {
				name, _ := device.GetName()
				uuid, _ := device.GetUUID()
				log.Printf("  GPU %d: %s (%s)", i, name, uuid)
			}
		}
	}

	// Build constant labels from environment (for deployment mode identification)
	constLabels := prometheus.Labels{}
	for _, pair := range []struct{ env, label string }{
		{"NODE_NAME", "node"},
		{"POD_NAME", "pod"},
		{"POD_NAMESPACE", "namespace"},
	} {
		if v := os.Getenv(pair.env); v != "" {
			constLabels[pair.label] = v
		}
	}

	// Create components
	coll := collector.New()
	tracker := idle.NewTracker()
	prom := exporter.New(constLabels)
	prom.Register()

	// Context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	g, gctx := errgroup.WithContext(ctx)

	// Goroutine 1: Polling loop
	g.Go(func() error {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		// Run once immediately
		poll(coll, tracker, prom)

		for {
			select {
			case <-gctx.Done():
				return gctx.Err()
			case <-ticker.C:
				poll(coll, tracker, prom)
			}
		}
	})

	// Goroutine 2: HTTP server
	g.Go(func() error {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok\n"))
		})

		srv := &http.Server{
			Addr:    ":" + httpPort,
			Handler: mux,
		}

		errCh := make(chan error, 1)
		go func() {
			log.Printf("HTTP server listening on :%s (/metrics, /healthz)", httpPort)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("http server error: %w", err)
			}
		}()

		select {
		case err := <-errCh:
			return err
		case <-gctx.Done():
			log.Println("HTTP server shutting down...")
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("http server shutdown error: %w", err)
			}
			return gctx.Err()
		}
	})

	if err := g.Wait(); err != nil && err != context.Canceled {
		log.Fatalf("Service error: %v", err)
	}

	log.Println("GPU Idle Metrics Exporter stopped")
}

// poll runs one collection cycle: collect -> track idle -> update Prometheus.
func poll(coll *collector.Collector, tracker *idle.Tracker, prom *exporter.Exporter) {
	snap, err := coll.Collect()
	if err != nil {
		log.Printf("collection error: %v", err)
		return
	}
	states := tracker.Update(snap)
	prom.UpdateMetrics(snap, states)
}

// getEnvOrDefault returns the value of an environment variable or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// getEnvDuration parses a duration from an environment variable or returns a default.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("Invalid %s=%q, using default %v: %v", key, v, defaultValue, err)
		return defaultValue
	}
	return d
}
