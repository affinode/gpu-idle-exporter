package idle

import (
	"testing"
	"time"

	"github.com/affinode/gpu-idle-exporter/internal/collector"
)

func makeSnapshot(ts time.Time, procs []collector.ProcessSample) *collector.Snapshot {
	names := make(map[uint32]string)
	for _, p := range procs {
		names[p.PID] = "python"
	}
	return &collector.Snapshot{
		Timestamp:    ts,
		Processes:    procs,
		ProcessNames: names,
	}
}

func proc(gpu int, pid uint32, mem uint64, smUtil uint32) collector.ProcessSample {
	return collector.ProcessSample{GPU: gpu, PID: pid, UsedMemory: mem, SmUtil: smUtil}
}

func TestNewProcessStartsActive(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	snap := makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0), // 1 GiB, smUtil=0
	})

	states := tracker.Update(snap)

	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	// New process should NOT be idle on first observation (benefit of the doubt)
	if states[0].IsIdle {
		t.Error("new process should start as active, not idle")
	}
	if states[0].IdleDuration != 0 {
		t.Errorf("new process idle duration should be 0, got %v", states[0].IdleDuration)
	}
}

func TestProcessBecomesIdleOnSecondPoll(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// First poll: new process, starts active
	snap1 := makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	})
	tracker.Update(snap1)

	// Second poll: still smUtil=0 → now idle
	t1 := t0.Add(5 * time.Second)
	snap2 := makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	})
	states := tracker.Update(snap2)

	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].IsIdle {
		t.Error("process should be idle after second poll with smUtil=0")
	}
	// Idle duration should be ~0 since it just transitioned
	if states[0].IdleDuration != 0 {
		t.Errorf("idle duration should be 0 on transition poll, got %v", states[0].IdleDuration)
	}
}

func TestIdleDurationIncreases(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// Poll 1: first seen, starts active
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))

	// Poll 2: transitions to idle
	t1 := t0.Add(5 * time.Second)
	tracker.Update(makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))

	// Poll 3: still idle, duration should be 10s
	t2 := t1.Add(10 * time.Second)
	states := tracker.Update(makeSnapshot(t2, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))

	if len(states) != 1 {
		t.Fatalf("expected 1 state, got %d", len(states))
	}
	if !states[0].IsIdle {
		t.Error("process should still be idle")
	}
	if states[0].IdleDuration != 10*time.Second {
		t.Errorf("expected idle duration 10s, got %v", states[0].IdleDuration)
	}
	if states[0].IdleMemory != 1<<30 {
		t.Errorf("expected idle memory 1 GiB, got %d", states[0].IdleMemory)
	}
}

func TestActiveProcessReturningToIdle(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// Poll 1: active process (smUtil=50)
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 50),
	}))

	// Poll 2: still active
	t1 := t0.Add(5 * time.Second)
	states := tracker.Update(makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 80),
	}))
	if states[0].IsIdle {
		t.Error("process with smUtil=80 should not be idle")
	}
	if states[0].SmUtil != 80 {
		t.Errorf("expected SmUtil=80, got %d", states[0].SmUtil)
	}

	// Poll 3: becomes idle
	t2 := t1.Add(5 * time.Second)
	states = tracker.Update(makeSnapshot(t2, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))
	if !states[0].IsIdle {
		t.Error("process should be idle now")
	}
	// Just transitioned, duration = 0
	if states[0].IdleDuration != 0 {
		t.Errorf("expected 0 idle duration on transition, got %v", states[0].IdleDuration)
	}

	// Poll 4: idle for 30 more seconds
	t3 := t2.Add(30 * time.Second)
	states = tracker.Update(makeSnapshot(t3, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))
	if !states[0].IsIdle {
		t.Error("process should still be idle")
	}
	if states[0].IdleDuration != 30*time.Second {
		t.Errorf("expected 30s idle duration, got %v", states[0].IdleDuration)
	}
}

func TestIdleResetsWhenActive(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// Poll 1: first seen
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))

	// Poll 2: transitions to idle
	t1 := t0.Add(5 * time.Second)
	tracker.Update(makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))

	// Poll 3: idle for 60s
	t2 := t1.Add(60 * time.Second)
	states := tracker.Update(makeSnapshot(t2, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 0),
	}))
	if states[0].IdleDuration != 60*time.Second {
		t.Fatalf("expected 60s idle, got %v", states[0].IdleDuration)
	}

	// Poll 4: becomes active (smUtil=99)
	t3 := t2.Add(5 * time.Second)
	states = tracker.Update(makeSnapshot(t3, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 99),
	}))
	if states[0].IsIdle {
		t.Error("process should be active")
	}
	if states[0].IdleDuration != 0 {
		t.Error("idle duration should be 0 when active")
	}
	if states[0].IdleMemory != 0 {
		t.Error("idle memory should be 0 when active")
	}
}

func TestMultipleProcesses(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// Poll 1: two processes, both new
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 100, 4<<30, 50), // active, 4 GiB
		proc(0, 200, 8<<30, 0),  // will become idle, 8 GiB
	}))

	// Poll 2
	t1 := t0.Add(5 * time.Second)
	states := tracker.Update(makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 100, 4<<30, 50), // still active
		proc(0, 200, 8<<30, 0),  // now idle
	}))

	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}

	// Find states by PID
	var active, idle_ *ProcessIdleState
	for i := range states {
		if states[i].PID == 100 {
			active = &states[i]
		} else if states[i].PID == 200 {
			idle_ = &states[i]
		}
	}
	if active == nil || idle_ == nil {
		t.Fatal("couldn't find both processes in states")
	}

	if active.IsIdle {
		t.Error("PID 100 should be active")
	}
	if !idle_.IsIdle {
		t.Error("PID 200 should be idle")
	}
}

func TestStaleProcessCleanup(t *testing.T) {
	tracker := NewTracker()
	tracker.staleTimeout = 10 * time.Second // short timeout for testing
	t0 := time.Now()

	// Poll 1: process appears
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 50),
	}))

	// Poll 2: process disappears (not in snapshot)
	t1 := t0.Add(5 * time.Second)
	states := tracker.Update(makeSnapshot(t1, []collector.ProcessSample{}))
	if len(states) != 0 {
		t.Errorf("expected 0 states (no processes), got %d", len(states))
	}
	// Internal state should still track it (within stale timeout)
	if len(tracker.states) != 1 {
		t.Errorf("expected 1 tracked state (within stale timeout), got %d", len(tracker.states))
	}

	// Poll 3: after stale timeout, should be cleaned up
	t2 := t1.Add(11 * time.Second) // > 10s staleTimeout
	tracker.Update(makeSnapshot(t2, []collector.ProcessSample{}))
	if len(tracker.states) != 0 {
		t.Errorf("expected 0 tracked states (after stale timeout), got %d", len(tracker.states))
	}
}

func TestMultiGPUProcesses(t *testing.T) {
	tracker := NewTracker()
	t0 := time.Now()

	// Same PID on different GPUs should be tracked independently
	tracker.Update(makeSnapshot(t0, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 50),
		proc(1, 1234, 2<<30, 0),
	}))

	t1 := t0.Add(5 * time.Second)
	states := tracker.Update(makeSnapshot(t1, []collector.ProcessSample{
		proc(0, 1234, 1<<30, 50), // active on GPU 0
		proc(1, 1234, 2<<30, 0),  // idle on GPU 1
	}))

	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}

	for _, s := range states {
		if s.GPU == 0 && s.IsIdle {
			t.Error("PID 1234 on GPU 0 should be active")
		}
		if s.GPU == 1 && !s.IsIdle {
			t.Error("PID 1234 on GPU 1 should be idle")
		}
	}
}
