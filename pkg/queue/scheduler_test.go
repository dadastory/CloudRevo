package queue

import (
	"context"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/ent/task"
	"github.com/dadastory/CloudRevo/inventory/types"
)

type schedulerTestTask struct{ *InMemoryTask }

func (t *schedulerTestTask) Do(context.Context) (task.Status, error) {
	return task.StatusCompleted, nil
}

func TestFifoSchedulerRemovesWaitingTaskByID(t *testing.T) {
	scheduler := NewFifoScheduler(0, nil)
	first := &schedulerTestTask{InMemoryTask: &InMemoryTask{DBTask: &DBTask{Task: &ent.Task{ID: 1, PublicState: &types.TaskPublicState{}}}}}
	second := &schedulerTestTask{InMemoryTask: &InMemoryTask{DBTask: &DBTask{Task: &ent.Task{ID: 2, PublicState: &types.TaskPublicState{}}}}}
	if err := scheduler.Queue(first); err != nil {
		t.Fatalf("queue first task: %v", err)
	}
	if err := scheduler.Queue(second); err != nil {
		t.Fatalf("queue second task: %v", err)
	}

	removed, err := scheduler.Remove(second.ID())
	if err != nil || !removed {
		t.Fatalf("Remove() = (%v, %v), want (true, nil)", removed, err)
	}
	queued, err := scheduler.Request()
	if err != nil {
		t.Fatalf("request remaining task: %v", err)
	}
	if queued.ID() != first.ID() {
		t.Fatalf("remaining task = %d, want %d", queued.ID(), first.ID())
	}
}
