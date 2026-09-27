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

// ErrQueueTimeout reports that a job outlived the queue's kill deadline. The
// worker cancels the job, which kills the engine through exec.CommandContext,
// and waits for it to actually stop before reusing the slot; Submit returns the
// error as an ordinary tool failure.
var ErrQueueTimeout = errors.New("operation queue timed out")

// ErrQueueStuck reports that a job did not stop within the stop grace after its
// timeout canceled it. The queue cannot prove that the abandoned engine work
// has stopped, so it quarantines itself: it stops running jobs and fails every
// current and future submission with this error. Recovery requires restarting
// the process; continuing risks a second writer racing the one that refused to
// die.
var ErrQueueStuck = errors.New("operation queue stuck; restart required")

// ErrQueueClosed reports that Close has been called and the queue no longer
// accepts jobs.
var ErrQueueClosed = errors.New("operation queue is closed")

// defaultStopGrace is how long the worker waits, after canceling a timed-out
// job, for the job to actually return before it quarantines the queue. The
// cancel reaches exec.CommandContext and kills the engine, so a healthy job
// returns in milliseconds; the grace absorbs only a slow process teardown. It
// is deliberately far shorter than the timeout.
const defaultStopGrace = 30 * time.Second

// Job is a unit of work the Queue runs one at a time. The context is derived
// from the one the submitter passed to Submit; the queue cancels it when the
// timeout fires or the caller gives up, so a job should honor its cancellation
// to stop any engine work it started. A job that ignores cancellation past the
// stop grace causes the queue to quarantine itself.
type Job func(ctx context.Context) (any, error)

// Queue serializes jobs onto a single worker goroutine. It is the one
// serialization point for every operation that touches the engine: a caller
// submits a job and waits for its result, so concurrent callers cannot run
// engine operations simultaneously. The queue depth bounds how many jobs may
// wait behind the running one; a submission that finds the backlog full fails
// fast with ErrQueueFull.
//
// The timeout is a kill deadline, not a budget: when a job outlives it the
// worker cancels the job, which reaches exec.CommandContext and kills the
// engine, then waits for the job to actually return before it runs anything
// else. Only once the job has stopped is its slot reused, so a timed-out job
// can never overlap the next one. If the job will not stop even after being
// killed, the queue cannot prove that the project has one writer, so it
// quarantines itself with ErrQueueStuck rather than risk two.
type Queue struct {
	mu      sync.Mutex
	jobs    chan *queuedJob
	timeout time.Duration
	// stopGrace bounds how long the worker waits for a canceled, timed-out job
	// to return before it quarantines the queue. The cancel kills the engine,
	// so a healthy job returns at once; the grace absorbs only a slow reap. It
	// is a field rather than a constant so same-package tests can shrink it;
	// NewQueue installs the production default.
	stopGrace time.Duration
	closed    bool
	// stuck, once non-nil, is the quarantine error. stuckCh is closed with it
	// so submitters already blocked on a result wake; reading stuck under mu
	// then yields the message.
	stuck   error
	stuckCh chan struct{}
	// onStuck, when set, is called once from the worker with the quarantine
	// error after the queue quarantines itself, so the owner can shut down and
	// let the process be restarted.
	onStuck func(error)
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
// timeout bounds how long one job may run before the worker kills it and
// returns ErrQueueTimeout to the submitter; a non-positive timeout leaves jobs
// unbounded. The bound is enforced by the queue, not merely requested from the
// job: after canceling the job the worker waits for it to stop, and if it will
// not, quarantines the queue with ErrQueueStuck rather than reuse its slot.
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
			// runJob quarantined the queue: stop consuming. Submitters whose
			// jobs are still queued wake on stuckCh and see ErrQueueStuck.
			return
		}
	}
}

// runJob runs one job and reports whether the queue may continue. It runs the
// job on its own goroutine so a canceled job that ignores its context cannot
// block the worker's select. On the timeout path the worker does not abandon
// the job and move on: it cancels the job, which kills the engine through
// exec.CommandContext, waits for the job to actually return, and only then
// frees the slot. A job that still will not stop quarantines the queue.
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

	// The kill deadline elapsed. Cancel the job, then wait for it to stop
	// before the worker dequeues anything else: a job still writing project
	// files while the next one starts is exactly the corruption the queue
	// exists to prevent, so the slot is not free until the job has returned.
	cancel()

	grace := time.NewTimer(q.stopGrace)
	defer grace.Stop()
	select {
	case <-done:
		job.result <- jobResult{err: fmt.Errorf("%w after %s", ErrQueueTimeout, q.timeout)}
		return true
	case <-grace.C:
	}

	// The job would not stop even after being killed. The queue can no longer
	// prove that engine work is serialized, so it quarantines itself instead of
	// reusing the slot: two writers is the failure it exists to prevent.
	err := fmt.Errorf("%w: job did not stop within %s of the %s deadline", ErrQueueStuck, q.stopGrace, q.timeout)
	q.quarantine(err)
	job.result <- jobResult{err: err}
	return false
}

// quarantine marks the queue stuck and wakes every waiter. The first caller
// wins; later calls are no-ops. The installed onStuck hook, if any, runs after
// the state is published so a hook can observe it.
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

// SetOnStuck installs fn, which the worker calls once after the queue
// quarantines itself, with the quarantine error. It is the owner's hook to shut
// down and let the process restart, the only safe recovery once the queue can
// no longer guarantee a single writer. fn must return promptly and must not
// call back into the queue. Install it before submitting work.
func (q *Queue) SetOnStuck(fn func(error)) {
	q.mu.Lock()
	q.onStuck = fn
	q.mu.Unlock()
}

// Submit enqueues run and waits for its result. It fails fast with ErrQueueFull
// when the backlog is full, ErrQueueClosed after Close, and ErrQueueStuck once
// the queue has quarantined itself. It returns the context error when ctx is
// canceled before the result is delivered, ErrQueueTimeout when the job
// outlives the queue's timeout, and ErrQueueStuck when the queue gave up on the
// job that caused that timeout.
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

// Close stops accepting new jobs, drains the backlog, and waits for the worker
// to exit. A hung job is bounded by the queue's timeout and stop grace, so
// Close cannot block forever on one. If the queue quarantined itself, the
// worker is already gone and Close returns at once, even though the job that
// would not stop may still be running in the background: the queue cannot
// cancel a goroutine, which is precisely why it refuses further work instead.
// It is safe to call more than once.
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
