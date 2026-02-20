package idle

import (
	"log"
	"time"

	"github.com/affinode/gpu-idle-exporter/internal/collector"
)

// processKey uniquely identifies a process on a specific GPU.
type processKey struct {
	GPU int
	PID uint32
}

// processState tracks idle state for a single process.
type processState struct {
	LastActiveTime time.Time // last time smUtil > 0
	LastSeenTime   time.Time // last time process appeared in NVML results
	FirstSeenTime  time.Time // when we first observed this process
	IsIdle         bool      // current idle state (smUtil == 0 while holding memory)
	IdleSince      time.Time // when the process transitioned to idle
}

// ProcessIdleState is the exported view of one process's idle state.
type ProcessIdleState struct {
	GPU          int
	PID          uint32
	ProcessName  string
	UsedMemory   uint64        // bytes
	SmUtil       uint32        // percent 0-100
	IsIdle       bool          // true if smUtil==0 while holding memory
	IdleDuration time.Duration // time since process became idle; 0 if active
	IdleMemory   uint64        // bytes held while idle; 0 if active
}

// Tracker maintains per-process idle state across polling cycles.
type Tracker struct {
	states       map[processKey]*processState
	staleTimeout time.Duration // how long after disappearing before cleanup
}

// NewTracker creates a new idle tracker.
func NewTracker() *Tracker {
	return &Tracker{
		states:       make(map[processKey]*processState),
		staleTimeout: 30 * time.Second,
	}
}

// Update processes a new NVML snapshot and returns the current idle state for all processes.
func (t *Tracker) Update(snap *collector.Snapshot) []ProcessIdleState {
	now := snap.Timestamp
	seen := make(map[processKey]bool, len(snap.Processes))

	results := make([]ProcessIdleState, 0, len(snap.Processes))

	for _, p := range snap.Processes {
		key := processKey{GPU: p.GPU, PID: p.PID}
		seen[key] = true

		st, exists := t.states[key]
		if !exists {
			// New process: assume active at first sight.
			// This avoids a false idle metric spike before we've observed any activity.
			// We skip the idle transition on the first poll — the process needs to be
			// observed at least twice with smUtil=0 before being marked idle.
			st = &processState{
				LastActiveTime: now,
				FirstSeenTime:  now,
				LastSeenTime:   now,
				IsIdle:         false,
			}
			t.states[key] = st
			log.Printf("idle: new process detected: GPU=%d PID=%d name=%s mem=%d MiB",
				p.GPU, p.PID, snap.ProcessNames[p.PID], p.UsedMemory/(1024*1024))

			// Skip idle transition on first observation
			goto emit
		}

		st.LastSeenTime = now

		if p.SmUtil > 0 {
			// Process is active
			st.LastActiveTime = now
			if st.IsIdle {
				st.IsIdle = false
				log.Printf("idle: process became active: GPU=%d PID=%d", p.GPU, p.PID)
			}
		} else {
			// SmUtil == 0: process is idle (holding memory but no compute)
			if !st.IsIdle {
				st.IsIdle = true
				st.IdleSince = now
				log.Printf("idle: process became idle: GPU=%d PID=%d", p.GPU, p.PID)
			}
		}

	emit:

		var idleDuration time.Duration
		var idleMemory uint64
		if st.IsIdle {
			idleDuration = now.Sub(st.IdleSince)
			idleMemory = p.UsedMemory
		}

		results = append(results, ProcessIdleState{
			GPU:          p.GPU,
			PID:          p.PID,
			ProcessName:  snap.ProcessNames[p.PID],
			UsedMemory:   p.UsedMemory,
			SmUtil:       p.SmUtil,
			IsIdle:       st.IsIdle,
			IdleDuration: idleDuration,
			IdleMemory:   idleMemory,
		})
	}

	// Clean up stale processes (no longer in NVML results)
	for key, st := range t.states {
		if !seen[key] && now.Sub(st.LastSeenTime) > t.staleTimeout {
			log.Printf("idle: cleaning up stale process: GPU=%d PID=%d (last seen %v ago)",
				key.GPU, key.PID, now.Sub(st.LastSeenTime).Round(time.Second))
			delete(t.states, key)
		}
	}

	return results
}
