package queue

import (
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
