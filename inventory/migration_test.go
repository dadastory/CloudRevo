package inventory

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/ent/node"
	"github.com/dadastory/CloudRevo/ent/setting"
	"github.com/dadastory/CloudRevo/inventory/types"
	"github.com/dadastory/CloudRevo/pkg/boolset"
	"github.com/dadastory/CloudRevo/pkg/cache"
	"github.com/dadastory/CloudRevo/pkg/logging"
)

func TestMigrateMasterNodeDefaultsToGopeed(t *testing.T) {
	t.Setenv(EnvGopeedAPIToken, "compose-token")
	ctx := context.Background()
	client, err := ent.Open("sqlite3", "file:gopeed-master-node?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	if err := migrateMasterNode(logging.NewConsoleLogger(logging.LevelError), client, ctx); err != nil {
		t.Fatalf("migrate master node: %v", err)
	}
	master, err := client.Node.Query().Where(node.TypeEQ(node.TypeMaster)).Only(ctx)
	if err != nil {
		t.Fatalf("load master node: %v", err)
	}
	if master.Settings.Provider != types.DownloaderProviderGopeed || master.Settings.GopeedSetting == nil {
		t.Fatalf("master downloader = %#v, want Gopeed", master.Settings)
	}
	if master.Settings.GopeedSetting.Server != "http://gopeed:9999" ||
		master.Settings.GopeedSetting.Token != "compose-token" ||
		master.Settings.GopeedSetting.DownloadPath != "/app/Downloads" ||
		master.Settings.GopeedSetting.TempPath != "/cloudrevo/data/temp/gopeed" {
		t.Fatalf("unexpected Gopeed configuration: %#v", master.Settings.GopeedSetting)
	}
}

func TestMigrateDefaultShareMarkersBackfillsLegacyDefaultProperty(t *testing.T) {
	ctx := context.Background()
	client, err := ent.Open("sqlite3", "file:default-share-backfill?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	group, err := client.Group.Create().SetName("test").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	owner, err := client.User.Create().SetEmail("owner@example.test").SetNick("owner").SetGroupID(group.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	file, err := client.File.Create().SetOwnerID(owner.ID).SetName("legacy-default").SetType(0).Save(ctx)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	legacy, err := client.Share.Create().SetUserID(owner.ID).SetFileID(file.ID).SetProps(&types.ShareProps{Default: true}).Save(ctx)
	if err != nil {
		t.Fatalf("create legacy share: %v", err)
	}

	if err := migrateDefaultShareMarkers(ctx, client); err != nil {
		t.Fatalf("backfill default markers: %v", err)
	}
	updated, err := client.Share.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("load migrated share: %v", err)
	}
	if !updated.IsDefault {
		t.Fatal("legacy props.default must be backfilled into is_default")
	}
}

func TestInitializeDBClientUpgradesExistingDatabaseWithDefaultShareMarker(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "existing.db")
	client, err := ent.Open("sqlite3", databasePath+"?_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create current schema: %v", err)
	}
	group, err := client.Group.Create().SetName("test").SetPermissions(&boolset.BooleanSet{}).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	owner, err := client.User.Create().SetEmail("upgrade-owner@example.test").SetNick("owner").SetGroupID(group.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	file, err := client.File.Create().SetOwnerID(owner.ID).SetName("legacy-default").SetType(0).Save(ctx)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	legacy, err := client.Share.Create().SetUserID(owner.ID).SetFileID(file.ID).SetProps(&types.ShareProps{Default: true}).Save(ctx)
	if err != nil {
		t.Fatalf("create legacy share: %v", err)
	}
	if _, err := client.Setting.Create().SetName(DBVersionPrefix + "4.14.0").SetValue("installed").Save(ctx); err != nil {
		t.Fatalf("write old migration marker: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close current client: %v", err)
	}

	legacyDB, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	if _, err := legacyDB.Exec("DROP INDEX share_is_default"); err != nil {
		t.Fatalf("drop default index: %v", err)
	}
	if _, err := legacyDB.Exec("ALTER TABLE shares DROP COLUMN is_default"); err != nil {
		t.Fatalf("remove marker from legacy schema: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy sqlite: %v", err)
	}

	logger := logging.NewConsoleLogger(logging.LevelError)
	raw, err := ent.Open("sqlite3", databasePath+"?_fk=1")
	if err != nil {
		t.Fatalf("reopen legacy schema: %v", err)
	}
	upgraded, err := InitializeDBClient(logger, raw, cache.NewMemoStore("", logger), constants.BackendVersion)
	if err != nil {
		t.Fatalf("upgrade existing database: %v", err)
	}
	defer upgraded.Close()
	share, err := upgraded.Share.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("load upgraded legacy share: %v", err)
	}
	if !share.IsDefault {
		t.Fatal("upgrade must backfill legacy props.default into is_default")
	}
	if _, err := upgraded.Setting.Query().Where(setting.NameEQ(DBVersionPrefix + constants.BackendVersion)).Only(ctx); err != nil {
		t.Fatalf("upgrade must record the new version marker: %v", err)
	}
}
