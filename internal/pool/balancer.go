package pool

import (
	"sync/atomic"
)

// Balancer implements round-robin load balancing across workers
type Balancer struct {
	workers []*Worker
	current atomic.Uint64
}

// NewBalancer creates a new round-robin balancer
func NewBalancer(workers []*Worker) *Balancer {
	return &Balancer{
		workers: workers,
	}
}

// Next returns the next available worker using round-robin, skipping dead workers.
// Returns nil if no workers are alive.
func (b *Balancer) Next() *Worker {
	n := len(b.workers)
	if n == 0 {
		return nil
	}

	// Try up to n times to find an alive worker
	for i := 0; i < n; i++ {
		idx := b.current.Add(1) - 1
		worker := b.workers[idx%uint64(n)]
		if worker.IsAlive() {
			return worker
		}
	}

	return nil // All workers are dead
}

// ActiveCount returns the number of alive workers
func (b *Balancer) ActiveCount() int {
	count := 0
	for _, w := range b.workers {
		if w.IsAlive() {
			count++
		}
	}
	return count
}
