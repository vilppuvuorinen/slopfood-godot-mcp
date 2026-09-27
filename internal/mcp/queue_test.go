package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueReturnsJobResult(t *testing.T) {
	q := NewQueue(4)
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
	q := NewQueue(4)
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
	q := NewQueue(64)
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
	q := NewQueue(64)
	defer q.Close()

	const jobs = 100
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
	q := NewQueue(1)
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
	q := NewQueue(4)
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
	q := NewQueue(4)
	q.Close()
	q.Close()

	if _, err := q.Submit(context.Background(), func(context.Context) (any, error) {
		return nil, nil
	}); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("err = %v, want ErrQueueClosed", err)
	}
}

func TestQueueCloseDrainsBacklog(t *testing.T) {
	q := NewQueue(4)

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
