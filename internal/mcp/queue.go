package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrQueueFull reports that the queue's backlog is at capacity. Submit returns
// it instead of blocking, so a burst of calls is shed with a visible error
// rather than growing an unbounded backlog of pending engine operations.
var ErrQueueFull = errors.New("operation queue is full")

// ErrQueueTimeout reports that a job outlived the queue's per-request timeout.
// The worker abandons the job, cancels its context, and moves on, so a hung
// engine cannot pin the one queue slot; Submit returns the error as an
// ordinary tool failure.
var ErrQueueTimeout = errors.New("operation queue timed out")

// ErrQueueClosed reports that Close has been called and the queue no longer
// accepts jobs.
var ErrQueueClosed = errors.New("operation queue is closed")

// Job is a unit of work the Queue runs one at a time. The context is derived
// from the one the submitter passed to Submit; the queue cancels it when the
// timeout fires or the caller gives up, so a job should honor its cancellation
// to stop any engine work it started.
type Job func(ctx context.Context) (any, error)

// Queue serializes jobs onto a single worker goroutine. It is the one
// serialization point for every operation that touches the engine: a caller
// submits a job and waits for its result, so concurrent callers cannot run
// engine operations simultaneously. The queue depth bounds how many jobs may
// wait behind the running one; a submission that finds the backlog full fails
// fast with ErrQueueFull. The per-request timeout bounds the running job, not
// the wait, so a hung job releases its slot instead of pinning the queue.
type Queue struct {
	mu      sync.Mutex
	jobs    chan *queuedJob
	timeout time.Duration
	closed  bool
	wg      sync.WaitGroup
}

// queuedJob is one submission: the caller's context and job, plus the buffered
// result channel the worker delivers to. The channel is buffered so the worker
// never blocks on a caller that has already given up waiting.
type queuedJob struct {
	ctx    context.Context
	run    Job
	result chan jobResult
}

type jobResult struct {
	value any
	err   error
}

// NewQueue returns a started Queue that lets at most depth jobs wait behind the
// one the worker is running. A non-positive depth is raised to one so the queue
// always has room for the next job.
//
// timeout bounds how long one job may run before the worker cancels it and
// returns ErrQueueTimeout to the submitter; a non-positive timeout leaves jobs
// unbounded. The bound is enforced by the queue, not merely requested from the
// job, so a job that ignores its context cannot pin the queue slot.
func NewQueue(depth int, timeout time.Duration) *Queue {
	if depth <= 0 {
		depth = 1
	}
	q := &Queue{jobs: make(chan *queuedJob, depth), timeout: timeout}
	q.wg.Add(1)
	go q.worker()
	return q
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for job := range q.jobs {
		q.runJob(job)
	}
}

// runJob runs one job on its own goroutine, so the worker can abandon a job
// that ignores cancellation once the timeout fires and start the next one. An
// abandoned goroutine still sends to its buffered done channel when it finally
// returns, so it never blocks, and its late result is discarded.
func (q *Queue) runJob(job *queuedJob) {
	ctx, cancel := context.WithCancel(job.ctx)
	defer cancel()

	done := make(chan jobResult, 1)
	go func() {
		value, err := job.run(ctx)
		done <- jobResult{value: value, err: err}
	}()

	// A nil channel blocks forever, so a disabled timeout waits on done alone.
	var timedOut <-chan time.Time
	if q.timeout > 0 {
		timer := time.NewTimer(q.timeout)
		defer timer.Stop()
		timedOut = timer.C
	}

	select {
	case res := <-done:
		job.result <- res
	case <-timedOut:
		// Tell a cooperating job to stop, then move on. A job that ignores
		// its context still finishes in the background, but it can no longer
		// delay every submission behind it.
		cancel()
		job.result <- jobResult{err: fmt.Errorf("%w after %s", ErrQueueTimeout, q.timeout)}
	}
}

// Submit enqueues run and waits for its result. It fails fast with ErrQueueFull
// when the backlog is full and ErrQueueClosed after Close. It returns the
// context error when ctx is canceled before the result is delivered, and
// ErrQueueTimeout when the job outlives the queue's timeout. In both cases the
// running job is left to wind down; its result is simply dropped.
func (q *Queue) Submit(ctx context.Context, run Job) (any, error) {
	if run == nil {
		return nil, errors.New("queue: nil job")
	}
	job := &queuedJob{ctx: ctx, run: run, result: make(chan jobResult, 1)}

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil, ErrQueueClosed
	}
	select {
	case q.jobs <- job:
		q.mu.Unlock()
	default:
		q.mu.Unlock()
		return nil, ErrQueueFull
	}

	select {
	case res := <-job.result:
		return res.value, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("queue submit: %w", ctx.Err())
	}
}

// Close stops accepting new jobs, drains the backlog, and waits for the worker
// to exit. A hung job is bounded by the queue's timeout, so Close itself cannot
// block forever on one. It is safe to call more than once.
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.jobs)
	q.mu.Unlock()
	q.wg.Wait()
}
