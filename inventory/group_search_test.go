package inventory

import (
	"context"
	"fmt"
	"testing"

	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/pkg/boolset"
	"github.com/dadastory/CloudRevo/pkg/cache"
	"github.com/dadastory/CloudRevo/pkg/conf"
	"github.com/dadastory/CloudRevo/pkg/logging"
)

func TestGroupClientSearchFindsLaterMatchingGroupWithoutListingAllGroups(t *testing.T) {
	ctx := context.Background()
	client := newGroupSearchTestClient(t)
	groupClient := NewGroupClient(client, conf.SQLiteDB, cache.NewMemoStore("", logging.NewConsoleLogger(logging.LevelError)))

	for i := 0; i < 100; i++ {
		if _, err := client.Group.Create().SetName(fmt.Sprintf("early-%03d", i)).SetPermissions(&boolset.BooleanSet{}).Save(ctx); err != nil {
			t.Fatalf("create early group %d: %v", i, err)
		}
	}
	late, err := client.Group.Create().SetName("later matching group").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	if err != nil {
		t.Fatalf("create later matching group: %v", err)
	}

	groups, err := groupClient.Search(ctx, "matching", 20)
	if err != nil {
		t.Fatalf("search groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != late.ID {
		t.Fatalf("search must return the group after the former first-100 window, got %#v", groups)
	}
}

func TestGroupClientGetByIDsResolvesPersistedAudienceNames(t *testing.T) {
	ctx := context.Background()
	client := newGroupSearchTestClient(t)
	groupClient := NewGroupClient(client, conf.SQLiteDB, cache.NewMemoStore("", logging.NewConsoleLogger(logging.LevelError)))

	first, err := client.Group.Create().SetName("first saved audience").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	if err != nil {
		t.Fatalf("create first group: %v", err)
	}
	second, err := client.Group.Create().SetName("second saved audience").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	if err != nil {
		t.Fatalf("create second group: %v", err)
	}

	groups, err := groupClient.GetByIDs(ctx, []int{second.ID, first.ID})
	if err != nil {
		t.Fatalf("resolve group ids: %v", err)
	}
	if len(groups) != 2 || groups[0].ID != first.ID || groups[1].ID != second.ID {
		t.Fatalf("resolved groups must be deterministic and complete, got %#v", groups)
	}
}

func newGroupSearchTestClient(t *testing.T) *ent.Client {
	t.Helper()
	client, err := ent.Open("sqlite3", fmt.Sprintf("file:group-search-%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}
