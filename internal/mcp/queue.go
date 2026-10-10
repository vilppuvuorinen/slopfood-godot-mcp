package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Submit returns it instead of blocking, shedding a burst rather than growing an unbounded backlog.
var ErrQueueFull = errors.New("operation queue is full")

// A job outlived the kill deadline and was canceled; the worker waits for it to stop before reusing the slot.
var ErrQueueTimeout = errors.New("operation queue timed out")

// A job did not stop after its timeout; the queue quarantines itself and recovery requires a restart.
var ErrQueueStuck = errors.New("operation queue stuck; restart required")

var ErrQueueClosed = errors.New("operation queue is closed")

// How long the worker waits for a canceled job to return before quarantining the queue.
const defaultStopGrace = 30 * time.Second

// Must honor cancellation; ignoring it past the stop grace quarantines the queue.
type Job func(ctx context.Context) (any, error)

// Serializes jobs onto one worker; the timeout is a kill deadline, and a job
// that will not stop after cancellation quarantines the queue.
type Queue struct {
	mu      sync.Mutex
	jobs    chan *queuedJob
	timeout time.Duration
	// stopGrace is the production default, a field so tests can shrink it.
	stopGrace time.Duration
	closed    bool
	// stuck is the quarantine error; stuckCh is closed with it so blocked submitters wake.
	stuck   error
	stuckCh chan struct{}
	// Called once from the worker after quarantine so the owner can restart the process.
	onStuck func(error)
	wg      sync.WaitGroup
}

// result is buffered so the worker never blocks on a caller that has given up.
type queuedJob struct {
	ctx    context.Context
	run    Job
	result chan jobResult
}

type jobResult struct {
	value any
	err   error
}

// A non-positive depth is raised to one; a non-positive timeout leaves jobs unbounded.
func NewQueue(depth int, timeout time.Duration) *Queue {
	if depth <= 0 {
		depth = 1
	}
	q := &Queue{
		jobs:      make(chan *queuedJob, depth),
		timeout:   timeout,
		stopGrace: defaultStopGrace,
		stuckCh:   make(chan struct{}),
	}
	q.wg.Add(1)
	go q.worker()
	return q
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for job := range q.jobs {
		if !q.runJob(job) {
			return
		}
	}
}

// Runs the job on its own goroutine so one that ignores cancellation cannot
// block the worker; a job that will not stop quarantines the queue.
func (q *Queue) runJob(job *queuedJob) bool {
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
		return true
	case <-timedOut:
	}

	cancel()

	grace := time.NewTimer(q.stopGrace)
	defer grace.Stop()
	select {
	case <-done:
		job.result <- jobResult{err: fmt.Errorf("%w after %s", ErrQueueTimeout, q.timeout)}
		return true
	case <-grace.C:
	}

	err := fmt.Errorf("%w: job did not stop within %s of the %s deadline", ErrQueueStuck, q.stopGrace, q.timeout)
	q.quarantine(err)
	job.result <- jobResult{err: err}
	return false
}

// The first caller wins; onStuck runs after the state is published.
func (q *Queue) quarantine(err error) {
	q.mu.Lock()
	if q.stuck != nil {
		q.mu.Unlock()
		return
	}
	q.stuck = err
	close(q.stuckCh)
	onStuck := q.onStuck
	q.mu.Unlock()
	if onStuck != nil {
		onStuck(err)
	}
}

// fn must return promptly and must not call back into the queue; install it
// before submitting work.
func (q *Queue) SetOnStuck(fn func(error)) {
	q.mu.Lock()
	q.onStuck = fn
	q.mu.Unlock()
}

// Depth reports the number of jobs queued behind the one running; safe to call
// concurrently with Submit and Close.
func (q *Queue) Depth() int { return len(q.jobs) }

// Fails fast with ErrQueueFull, ErrQueueClosed, or ErrQueueStuck, and returns
// the context error if ctx is canceled before the result is delivered.
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
	if q.stuck != nil {
		stuck := q.stuck
		q.mu.Unlock()
		return nil, stuck
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
	case <-q.stuckCh:
		q.mu.Lock()
		stuck := q.stuck
		q.mu.Unlock()
		return nil, stuck
	case <-ctx.Done():
		return nil, fmt.Errorf("queue submit: %w", ctx.Err())
	}
}

// Wrap serializes a tool handler through the queue. It submits the handler as a
// job, so the handler runs on the single worker with the queue's derived context
// and kill deadline. A nil queue returns the handler unchanged.
func (q *Queue) Wrap(h ToolHandler) ToolHandler {
	if q == nil {
		return h
	}
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		return q.Submit(ctx, func(ctx context.Context) (any, error) {
			return h(ctx, args)
		})
	}
}

// If the queue quarantined itself the worker is already gone and Close returns
// at once, though the stuck job may still be running. Safe to call more than once.
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
