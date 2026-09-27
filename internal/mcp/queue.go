package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrQueueFull reports that the queue's backlog is at capacity. Submit returns
// it instead of blocking, so a burst of calls is shed with a visible error
// rather than growing an unbounded backlog of pending engine operations.
var ErrQueueFull = errors.New("operation queue is full")

// ErrQueueClosed reports that Close has been called and the queue no longer
// accepts jobs.
var ErrQueueClosed = errors.New("operation queue is closed")

// Job is a unit of work the Queue runs one at a time. The context is the one
// the submitter passed to Submit; a job should honor its cancellation so a
// disconnected caller can release its queue slot.
type Job func(ctx context.Context) (any, error)

// Queue serializes jobs onto a single worker goroutine. It is the one
// serialization point for every operation that touches the engine: a caller
// submits a job and waits for its result, so concurrent callers cannot run
// engine operations simultaneously. The queue depth bounds how many jobs may
// wait behind the running one; a submission that finds the backlog full fails
// fast with ErrQueueFull.
type Queue struct {
	mu     sync.Mutex
	jobs   chan *queuedJob
	closed bool
	wg     sync.WaitGroup
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
func NewQueue(depth int) *Queue {
	if depth <= 0 {
		depth = 1
	}
	q := &Queue{jobs: make(chan *queuedJob, depth)}
	q.wg.Add(1)
	go q.worker()
	return q
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for job := range q.jobs {
		value, err := job.run(job.ctx)
		job.result <- jobResult{value: value, err: err}
	}
}

// Submit enqueues run and waits for its result. It fails fast with ErrQueueFull
// when the backlog is full and ErrQueueClosed after Close. It returns the
// context error when ctx is canceled before the result is delivered; a job
// already running still finishes, its result is simply dropped.
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
// to exit. It is safe to call more than once.
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
