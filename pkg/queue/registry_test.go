package queue

import (
	"context"
	"testing"
	"time"
)

func TestTaskRegistryPublishesOwnerTaskChanges(t *testing.T) {
	registry := NewTaskRegistry()
	updates, unsubscribe := registry.Subscribe(42)
	defer unsubscribe()

	registry.Publish(42)
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive task update")
	}
}

func TestQueueCancelsRegisteredActiveWorker(t *testing.T) {
	q := &queue{
		activeCancels:  make(map[int]context.CancelFunc),
		activeCanceled: make(map[int]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.registerActiveWorker(7, cancel)
	q.cancelActiveWorker(7)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("active worker context was not canceled")
	}
}
