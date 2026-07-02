package pool

import (
	"context"
	"sync"
)

// Task represents a unit of work to be processed by the pool.
type Task func(ctx context.Context)

// Pool represents a worker pool that runs tasks concurrently with a maximum limit.
type Pool struct {
	maxWorkers int
	taskChan   chan Task
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewPool creates a new worker pool with a specific limit of concurrent workers.
func NewPool(maxWorkers int) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	return &Pool{
		maxWorkers: maxWorkers,
		taskChan:   make(chan Task, maxWorkers*2), // buffered queue
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start spawns the workers and starts listening for tasks.
func (p *Pool) Start() {
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case task, ok := <-p.taskChan:
					if !ok {
						return
					}
					task(p.ctx)
				case <-p.ctx.Done():
					return
				}
			}
		}()
	}
}

// Submit submits a task to the pool.
func (p *Pool) Submit(task Task) {
	select {
	case p.taskChan <- task:
	case <-p.ctx.Done():
	}
}

// Stop gracefully stops the pool, waiting for active tasks to finish.
func (p *Pool) Stop() {
	close(p.taskChan)
	p.wg.Wait()
	p.cancel()
}
