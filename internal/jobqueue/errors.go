package jobqueue

import "errors"

var (
	// errQueueStopped is returned when trying to enqueue a job after the queue has been stopped.
	errQueueStopped = errors.New("jobqueue: queue is stopped")
	// errQueueFull is returned when trying to enqueue a job non-blocking and the queue is full.
	errQueueFull = errors.New("jobqueue: queue is full")
)
