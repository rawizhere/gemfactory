package worker

import (
	"context"
	"sync"
)

// Worker defines the contract for any task intended to run as a background service.
type Worker interface {
	Start(ctx context.Context, wg *sync.WaitGroup)
}

// Manager orchestrates the lifecycle and execution of multiple background workers.
type Manager struct {
	workers []Worker
	wg      *sync.WaitGroup
}

// NewManager initializes a new Worker Manager instance.
func NewManager() *Manager {
	return &Manager{
		workers: make([]Worker, 0),
		wg:      &sync.WaitGroup{},
	}
}

// Add registers a new background worker with the manager.
func (m *Manager) Add(worker Worker) {
	m.workers = append(m.workers, worker)
}

// Start launches all registered workers in their own goroutines.
func (m *Manager) Start(ctx context.Context) {
	for _, w := range m.workers {
		m.wg.Add(1)
		go w.Start(ctx, m.wg)
	}
}
