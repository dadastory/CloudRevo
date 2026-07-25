package queue

import "sync"

type (
	// TaskRegistry is used in slave node to track in-memory stateful tasks.
	TaskRegistry interface {
		// NextID returns the next available Task ID.
		NextID() int
		// Get returns the Task by ID.
		Get(id int) (Task, bool)
		// Set sets the Task by ID.
		Set(id int, t Task)
		// Delete deletes the Task by ID.
		Delete(id int)
		// Subscribe receives coalesced updates for one task owner.
		Subscribe(ownerID int) (<-chan struct{}, func())
		Publish(ownerID int)
	}

	taskRegistry struct {
		tasks       map[int]Task
		current     int
		subscribers map[int]map[chan struct{}]struct{}
		mu          sync.Mutex
	}
)

// NewTaskRegistry creates a new TaskRegistry.
func NewTaskRegistry() TaskRegistry {
	return &taskRegistry{
		tasks:       make(map[int]Task),
		subscribers: make(map[int]map[chan struct{}]struct{}),
	}
}

func (r *taskRegistry) NextID() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.current++
	return r.current
}

func (r *taskRegistry) Get(id int) (Task, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	return t, ok
}

func (r *taskRegistry) Set(id int, t Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks[id] = t
}

func (r *taskRegistry) Delete(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tasks, id)
}

func (r *taskRegistry) Subscribe(ownerID int) (<-chan struct{}, func()) {
	r.mu.Lock()
	updates := make(chan struct{}, 1)
	if r.subscribers[ownerID] == nil {
		r.subscribers[ownerID] = make(map[chan struct{}]struct{})
	}
	r.subscribers[ownerID][updates] = struct{}{}
	r.mu.Unlock()
	return updates, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if subscribers := r.subscribers[ownerID]; subscribers != nil {
			delete(subscribers, updates)
			if len(subscribers) == 0 {
				delete(r.subscribers, ownerID)
			}
		}
	}
}

func (r *taskRegistry) Publish(ownerID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for subscriber := range r.subscribers[ownerID] {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
