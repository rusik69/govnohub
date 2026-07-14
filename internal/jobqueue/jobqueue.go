// Package jobqueue provides a simple in-process background job queue for heavy
// operations such as git archive generation and large merges. Jobs run in a
// fixed-size worker pool and are processed first-in-first-out.
package jobqueue

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// A Job is a unit of work that can be enqueued for background execution.
type Job interface {
	// Name returns a human-readable name for the job (used in logging/metrics).
	Name() string
	// Execute performs the job. It receives a context that is cancelled when
	// the queue is shutting down; long-running jobs should respect it.
	Execute(ctx context.Context) error
}

// Stats contains counters for the job queue.
type Stats struct {
	Enqueued   int64
	Started    int64
	Completed  int64
	Failed     int64
	Queued     int64 // current queue depth
	WorkerBusy int64 // currently executing workers
}

// Queue is a simple in-process background job queue with a fixed-size worker pool.
type Queue struct {
	ch      chan jobItem
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context
	stopped atomic.Bool

	enqueued  atomic.Int64
	started   atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
}

type jobItem struct {
	job Job
}

// New creates a new job queue with the given buffer size and number of workers.
// bufferSize is the maximum number of jobs that can be waiting in the queue.
// numWorkers is the number of goroutines that process jobs concurrently.
// The queue is started immediately.
func New(bufferSize, numWorkers int) *Queue {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	if numWorkers <= 0 {
		numWorkers = 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		ch:     make(chan jobItem, bufferSize),
		ctx:    ctx,
		cancel: cancel,
	}
	for i := 0; i < numWorkers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	return q
}

// Enqueue adds a job to the queue. It returns immediately if the queue is not full,
// or blocks until space is available. Returns an error if the queue has been stopped.
func (q *Queue) Enqueue(job Job) error {
	if q.stopped.Load() {
		return errQueueStopped
	}
	q.enqueued.Add(1)
	select {
	case q.ch <- jobItem{job: job}:
		return nil
	case <-q.ctx.Done():
		return errQueueStopped
	}
}

// EnqueueNonBlocking adds a job to the queue without blocking.
// Returns an error if the queue is full or stopped.
func (q *Queue) EnqueueNonBlocking(job Job) error {
	if q.stopped.Load() {
		return errQueueStopped
	}
	q.enqueued.Add(1)
	select {
	case q.ch <- jobItem{job: job}:
		return nil
	default:
		q.enqueued.Add(-1) // rollback
		return errQueueFull
	}
}

// Stop gracefully shuts down the queue. It signals all workers to stop and waits
// for in-flight jobs to complete. Pending jobs in the buffer are NOT processed.
func (q *Queue) Stop() {
	q.stopped.Store(true)
	q.cancel()
	q.wg.Wait()
}

// Stats returns current queue statistics.
func (q *Queue) Stats() Stats {
	return Stats{
		Enqueued:   q.enqueued.Load(),
		Started:    q.started.Load(),
		Completed:  q.completed.Load(),
		Failed:     q.failed.Load(),
		Queued:     int64(len(q.ch)),
		WorkerBusy: q.started.Load() - q.completed.Load() - q.failed.Load(),
	}
}

// worker processes jobs from the queue.
func (q *Queue) worker(id int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case item := <-q.ch:
			q.process(item.job)
		}
	}
}

func (q *Queue) process(job Job) {
	q.started.Add(1)
	start := time.Now()
	log.Printf("[jobqueue] starting job %q", job.Name())

	// Recover from panics so a single bad job doesn't crash the process.
	defer func() {
		if r := recover(); r != nil {
			q.failed.Add(1)
			log.Printf("[jobqueue] job %q panicked: %v", job.Name(), r)
		}
	}()

	// Run the job with a timeout context (max 10 minutes per job).
	ctx, cancel := context.WithTimeout(q.ctx, 10*time.Minute)
	defer cancel()

	if err := job.Execute(ctx); err != nil {
		q.failed.Add(1)
		log.Printf("[jobqueue] job %q failed after %v: %v", job.Name(), time.Since(start), err)
	} else {
		q.completed.Add(1)
		log.Printf("[jobqueue] job %q completed in %v", job.Name(), time.Since(start))
	}
}
