package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Long enough that no test job reaches it by accident.
const testQueueTimeout = time.Minute

// Generous so a cooperative job is never mistaken for a stuck one.
const testQueueStopGrace = time.Second

func TestQueueWrapRunsHandlerOnWorker(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	defer q.Close()

	handler := q.Wrap(func(context.Context, json.RawMessage) (any, error) {
		return 7, nil
	})
	value, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("wrapped handler: %v", err)
	}
	if value != 7 {
		t.Errorf("value = %v, want 7", value)
	}
}

func TestQueueWrapNilReturnsHandlerUnchanged(t *testing.T) {
	var q *Queue
	handler := ToolHandler(func(context.Context, json.RawMessage) (any, error) {
		return 7, nil
	})
	value, err := q.Wrap(handler)(context.Background(), nil)
	if err != nil {
		t.Fatalf("wrapped handler: %v", err)
	}
	if value != 7 {
		t.Errorf("value = %v, want 7", value)
	}
}

func TestQueueReturnsJobResult(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	defer q.Close()

	value, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if value != 42 {
		t.Errorf("value = %v, want 42", value)
	}
}

func TestQueuePropagatesJobError(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	defer q.Close()

	want := errors.New("boom")
	_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestQueueSerializesJobs(t *testing.T) {
	q := NewQueue(64, testQueueTimeout)
	defer q.Close()

	var active, maxActive int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = q.Submit(context.Background(), func(context.Context) (any, error) {
				n := atomic.AddInt32(&active, 1)
				for {
					highest := atomic.LoadInt32(&maxActive)
					if n <= highest || atomic.CompareAndSwapInt32(&maxActive, highest, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil, nil
			})
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("max concurrent jobs = %d, want 1", got)
	}
}

func TestQueueConcurrentSubmissions(t *testing.T) {
	const jobs = 100
	q := NewQueue(jobs, testQueueTimeout)
	defer q.Close()

	var sum int64
	var wg sync.WaitGroup
	for i := 1; i <= jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
				atomic.AddInt64(&sum, 1)
				return nil, nil
			})
			if err != nil {
				t.Errorf("Submit: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&sum); got != jobs {
		t.Errorf("ran %d jobs, want %d", got, jobs)
	}
}

func TestQueueOverloadReturnsErrQueueFull(t *testing.T) {
	q := NewQueue(1, testQueueTimeout)
	defer q.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		})
		firstDone <- err
	}()
	<-started

	secondDone := make(chan struct{})
	go func() {
		_, _ = q.Submit(context.Background(), func(context.Context) (any, error) {
			return nil, nil
		})
		close(secondDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(q.jobs) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("second job never reached the backlog")
		}
		time.Sleep(time.Millisecond)
	}

	if _, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return nil, nil
	}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Errorf("first job: %v", err)
	}
	<-secondDone
}

func TestQueueSubmitRespectsContextCancellation(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	defer q.Close()

	release := make(chan struct{})
	started := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		})
		firstDone <- err
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() {
		_, err := q.Submit(ctx, func(context.Context) (any, error) {
			return nil, nil
		})
		waiting <- err
	}()
	cancel()

	if err := <-waiting; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Errorf("first job: %v", err)
	}
}

func TestQueueCloseRejectsSubmit(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	q.Close()
	q.Close()

	if _, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return nil, nil
	}); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("err = %v, want ErrQueueClosed", err)
	}
}

func TestQueueCloseDrainsBacklog(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)

	var ran int64
	q.Submit(context.Background(), func(context.Context) (any, error) { //nolint:errcheck // asserted via the counter below.
		atomic.AddInt64(&ran, 1)
		return nil, nil
	})
	q.Close()

	if got := atomic.LoadInt64(&ran); got != 1 {
		t.Errorf("ran %d jobs, want 1", got)
	}
}

func TestQueueTimeoutCancelsThenReleasesSlot(t *testing.T) {
	const timeout = 20 * time.Millisecond
	q := NewQueue(1, timeout)
	q.stopGrace = testQueueStopGrace
	defer q.Close()

	// The job honors its context, so the timeout's cancel lets the worker reuse the slot.
	if _, err := q.Submit(context.Background(), func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}); !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("err = %v, want ErrQueueTimeout", err)
	}

	ran := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(ran)
			return nil, nil
		})
		done <- err
	}()

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("next job never ran after the timed-out job stopped")
	}
	if err := <-done; err != nil {
		t.Fatalf("next job: %v", err)
	}
}

// While a job that ignores its context is still running after its timeout, the
// worker must not start the job queued behind it.
func TestQueueTimeoutDoesNotOverlapNextJob(t *testing.T) {
	const timeout = 20 * time.Millisecond
	q := NewQueue(2, timeout)
	q.stopGrace = time.Minute // keep the grace out of the way
	defer q.Close()

	var firstStopped, secondStarted atomic.Bool
	release := make(chan struct{})
	started := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(started)
			<-release
			firstStopped.Store(true)
			return nil, nil
		})
		firstDone <- err
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			if !firstStopped.Load() {
				t.Error("second job started before the timed-out first job stopped")
			}
			secondStarted.Store(true)
			return nil, nil
		})
		secondDone <- err
	}()

	time.Sleep(4 * timeout)
	if secondStarted.Load() {
		t.Fatal("second job started while the timed-out first job was still running")
	}

	close(release)
	if err := <-firstDone; !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("first job err = %v, want ErrQueueTimeout", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second job err = %v", err)
	}
	if !secondStarted.Load() {
		t.Fatal("second job never ran after the first job stopped")
	}
}

func TestQueueTimeoutCancelsJobContext(t *testing.T) {
	const timeout = 50 * time.Millisecond
	q := NewQueue(1, timeout)
	q.stopGrace = testQueueStopGrace
	defer q.Close()

	jobCtxErr := make(chan error, 1)
	if _, err := q.Submit(context.Background(), func(ctx context.Context) (any, error) {
		<-ctx.Done()
		jobCtxErr <- ctx.Err()
		return nil, ctx.Err()
	}); !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("err = %v, want ErrQueueTimeout", err)
	}

	select {
	case err := <-jobCtxErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("job context error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job context was not canceled when the timeout fired")
	}
}

// A job that ignores its context past the stop grace makes the queue refuse all
// current and future work instead of starting a second writer.
func TestQueueQuarantinesWhenJobWillNotStop(t *testing.T) {
	const timeout = 20 * time.Millisecond
	q := NewQueue(2, timeout)
	q.stopGrace = 20 * time.Millisecond

	stuckHook := make(chan error, 1)
	q.SetOnStuck(func(err error) { stuckHook <- err })

	var ranSecond atomic.Bool
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})

	firstDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		})
		firstDone <- err
	}()
	<-started

	// Queue a second job behind the first so it is waiting at quarantine time.
	secondDone := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			ranSecond.Store(true)
			return nil, nil
		})
		secondDone <- err
	}()

	if err := <-firstDone; !errors.Is(err, ErrQueueStuck) {
		t.Fatalf("first job err = %v, want ErrQueueStuck", err)
	}
	if err := <-secondDone; !errors.Is(err, ErrQueueStuck) {
		t.Fatalf("second job err = %v, want ErrQueueStuck", err)
	}
	if ranSecond.Load() {
		t.Fatal("a queued job ran after the queue was stuck")
	}

	select {
	case err := <-stuckHook:
		if !errors.Is(err, ErrQueueStuck) {
			t.Fatalf("onStuck err = %v, want ErrQueueStuck", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onStuck hook never fired")
	}

	if _, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return nil, nil
	}); !errors.Is(err, ErrQueueStuck) {
		t.Fatalf("err = %v, want ErrQueueStuck", err)
	}
	q.Close()
}

func TestQueueCloseWaitsOutHungJob(t *testing.T) {
	const timeout = 20 * time.Millisecond
	q := NewQueue(1, timeout)
	q.stopGrace = 20 * time.Millisecond

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	submitErr := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			close(started)
			<-release
			return nil, nil
		})
		submitErr <- err
	}()
	<-started

	closed := make(chan struct{})
	go func() {
		q.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked on a hung job instead of the timeout and grace")
	}
	if err := <-submitErr; !errors.Is(err, ErrQueueStuck) {
		t.Fatalf("err = %v, want ErrQueueStuck", err)
	}
}
