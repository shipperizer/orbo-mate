package pool

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPool_SubmitAndRun(t *testing.T) {
	p := NewPool(3)
	p.Start()
	defer p.Stop()

	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0

	for i := 0; i < 10; i++ {
		wg.Add(1)
		p.Submit(func(ctx context.Context) {
			defer wg.Done()
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	wg.Wait()

	if count != 10 {
		t.Errorf("Expected count to be 10, got %d", count)
	}
}

func TestPool_Stop(t *testing.T) {
	p := NewPool(2)
	p.Start()

	var wg sync.WaitGroup
	wg.Add(1)
	p.Submit(func(ctx context.Context) {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
	})

	p.Stop()
	wg.Wait()
}
