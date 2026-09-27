package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testQueueTimeout is long enough that no test job reaches it by accident;
// tests that exercise the timeout use their own short value.
const testQueueTimeout = time.Minute

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
	// Size the backlog for every job so concurrent submissions cannot trip the
	// overload path this test is not about; overload has its own test.
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

func TestQueueTimeoutReleasesSlotForNextJob(t *testing.T) {
	const timeout = 50 * time.Millisecond
	q := NewQueue(1, timeout)
	defer q.Close()

	release := make(chan struct{})
	hungDone := make(chan struct{})
	hungErr := make(chan error, 1)
	go func() {
		_, err := q.Submit(context.Background(), func(context.Context) (any, error) {
			defer close(hungDone)
			<-release
			return nil, nil
		})
		hungErr <- err
	}()

	if err := <-hungErr; !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("err = %v, want ErrQueueTimeout", err)
	}
	select {
	case <-hungDone:
		t.Fatal("hung job returned before the test released it")
	default:
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
		t.Fatal("next job never ran; the timed-out job kept its slot")
	}
	if err := <-done; err != nil {
		t.Fatalf("next job: %v", err)
	}

	close(release)
	<-hungDone
}

func TestQueueTimeoutCancelsJobContext(t *testing.T) {
	const timeout = 50 * time.Millisecond
	q := NewQueue(1, timeout)
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

func TestQueueCloseWaitsOutHungJob(t *testing.T) {
	const timeout = 50 * time.Millisecond
	q := NewQueue(1, timeout)

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
		t.Fatal("Close blocked on a hung job instead of the timeout")
	}
	if err := <-submitErr; !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("err = %v, want ErrQueueTimeout", err)
	}
}
