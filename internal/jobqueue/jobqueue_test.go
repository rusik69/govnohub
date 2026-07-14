package jobqueue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type testJob struct {
	name    string
	delay   time.Duration
	err     error
	executed *atomic.Int64
}

func (j *testJob) Name() string { return j.name }

func (j *testJob) Execute(ctx context.Context) error {
	if j.executed != nil {
		j.executed.Add(1)
	}
	if j.delay > 0 {
		select {
		case <-time.After(j.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return j.err
}

func TestNewQueue(t *testing.T) {
	q := New(10, 2)
	if q == nil {
		t.Fatal("expected non-nil queue")
	}
	defer q.Stop()
}

func TestEnqueueAndProcess(t *testing.T) {
	q := New(10, 2)
	defer q.Stop()

	var count atomic.Int64
	job := &testJob{name: "test", executed: &count}

	if err := q.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	if n := count.Load(); n != 1 {
		t.Fatalf("expected job to be executed once, got %d", n)
	}
}

func TestEnqueueNonBlocking(t *testing.T) {
	q := New(2, 2)
	defer q.Stop()

	// EnqueueNonBlocking should succeed when buffer has space
	err := q.EnqueueNonBlocking(&testJob{name: "fast"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Give workers time to drain the buffer
	time.Sleep(100 * time.Millisecond)

	stats := q.Stats()
	if stats.Completed != 1 {
		t.Fatalf("expected 1 completed, got %d", stats.Completed)
	}
}

func TestStop(t *testing.T) {
	q := New(10, 2)

	// Enqueue a long-running job
	longJob := &testJob{name: "long", delay: 5 * time.Second}
	q.Enqueue(longJob)

	// Stop should wait for in-flight but not process remaining
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop completed
	case <-time.After(3 * time.Second):
		t.Fatal("Stop didn't return within 3 seconds (expected ~5s wait)")
	}

	// Enqueue after stop should fail
	err := q.Enqueue(&testJob{name: "after-stop"})
	if err == nil {
		t.Fatal("expected error when enqueueing after stop")
	}
}

func TestPanicRecovery(t *testing.T) {
	q := New(10, 2)
	defer q.Stop()

	panicJob := &testJob{name: "panic"}
	// Override Execute to panic
	err := q.Enqueue(panicJob)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Should not crash; wait for processing
	time.Sleep(100 * time.Millisecond)
}

func TestStats(t *testing.T) {
	q := New(10, 4)
	defer q.Stop()

	var count atomic.Int64
	job := &testJob{name: "stats-test", executed: &count}
	q.Enqueue(job)
	time.Sleep(200 * time.Millisecond)

	stats := q.Stats()
	if stats.Enqueued < 1 {
		t.Fatalf("expected enqueued >= 1, got %d", stats.Enqueued)
	}
	if stats.Completed < 1 {
		t.Fatalf("expected completed >= 1, got %d", stats.Completed)
	}
}

func TestFailedJob(t *testing.T) {
	q := New(10, 2)
	defer q.Stop()

	failJob := &testJob{name: "fail", err: errors.New("test error")}
	q.Enqueue(failJob)
	time.Sleep(200 * time.Millisecond)

	stats := q.Stats()
	if stats.Failed < 1 {
		t.Fatalf("expected failed >= 1, got %d", stats.Failed)
	}
}

func TestArchiveJobSanitizeRef(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"main", "main"},
		{"feature/foo", "feature_foo"},
		{"v1.0.0", "v1.0.0"},
		{"release/v1.0-beta", "release_v1.0-beta"},
	}
	for _, tt := range tests {
		got := sanitizeRef(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeRef(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConcurrentEnqueue(t *testing.T) {
	q := New(100, 4)
	defer q.Stop()

	var count atomic.Int64
	const n = 50
	for i := 0; i < n; i++ {
		job := &testJob{name: "conc", executed: &count}
		if err := q.Enqueue(job); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if c := count.Load(); c != int64(n) {
		t.Fatalf("expected %d executed, got %d", n, c)
	}
}

func TestJobCounters(t *testing.T) {
	q := New(10, 2)
	defer q.Stop()

	var count atomic.Int64
	for i := 0; i < 3; i++ {
		q.Enqueue(&testJob{name: "ok", executed: &count})
	}
	time.Sleep(300 * time.Millisecond)

	stats := q.Stats()
	if stats.Enqueued != 3 {
		t.Errorf("expected 3 enqueued, got %d", stats.Enqueued)
	}
	if stats.Completed != 3 {
		t.Errorf("expected 3 completed, got %d", stats.Completed)
	}
}
