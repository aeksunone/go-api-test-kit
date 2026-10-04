package store

import (
	"context"
	"errors"
	"slices"
	"sync"
)

var ErrNotFound = errors.New("task not found")

type Task struct {
	ID      int64  `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Title   string `json:"title"`
	Done    bool   `json:"done"`
}

type Tasks interface {
	List(context.Context, int64) ([]Task, error)
	Create(context.Context, int64, string) (Task, error)
	Get(context.Context, int64, int64) (Task, error)
	Update(context.Context, int64, int64, string, bool) (Task, error)
	Delete(context.Context, int64, int64) error
}

// Memory is a concurrency-safe reference store for fast HTTP tests and the demo.
type Memory struct {
	mu    sync.RWMutex
	next  int64
	tasks map[int64]Task
}

func NewMemory() *Memory { return &Memory{tasks: make(map[int64]Task)} }
func (m *Memory) List(_ context.Context, owner int64) ([]Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tasks := []Task{}
	for _, t := range m.tasks {
		if t.OwnerID == owner {
			tasks = append(tasks, t)
		}
	}
	slices.SortFunc(tasks, func(a, b Task) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return tasks, nil
}
func (m *Memory) Create(_ context.Context, owner int64, title string) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	t := Task{ID: m.next, OwnerID: owner, Title: title}
	m.tasks[t.ID] = t
	return t, nil
}
func (m *Memory) Get(_ context.Context, owner, id int64) (Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok || t.OwnerID != owner {
		return Task{}, ErrNotFound
	}
	return t, nil
}
func (m *Memory) Update(_ context.Context, owner, id int64, title string, done bool) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok || t.OwnerID != owner {
		return Task{}, ErrNotFound
	}
	t.Title = title
	t.Done = done
	m.tasks[id] = t
	return t, nil
}
func (m *Memory) Delete(_ context.Context, owner, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok || t.OwnerID != owner {
		return ErrNotFound
	}
	delete(m.tasks, id)
	return nil
}
