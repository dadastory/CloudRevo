package user

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dadastory/CloudRevo/application/constants"
	"github.com/dadastory/CloudRevo/application/dependency"
	"github.com/dadastory/CloudRevo/ent"
	"github.com/dadastory/CloudRevo/inventory"
	"github.com/dadastory/CloudRevo/pkg/boolset"
	"github.com/dadastory/CloudRevo/pkg/cache"
	"github.com/dadastory/CloudRevo/pkg/conf"
	"github.com/dadastory/CloudRevo/pkg/hashid"
	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/gin-gonic/gin"
)

func TestSearchShareGroupsFindsMatchBeyondFormerGroupListLimit(t *testing.T) {
	dep := newInfoGroupsTestDependency(t)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if _, err := dep.GroupClient().Upsert(ctx, &ent.Group{Name: fmt.Sprintf("early-group-%03d", i), Permissions: &boolset.BooleanSet{}}); err != nil {
			t.Fatalf("create early group %d: %v", i, err)
		}
	}
	late, err := dep.GroupClient().Upsert(ctx, &ent.Group{Name: "later searchable group", Permissions: &boolset.BooleanSet{}})
	if err != nil {
		t.Fatalf("create later group: %v", err)
	}

	groups, err := (&SearchShareGroupService{Keyword: "searchable"}).Search(newInfoGroupsGinContext(t, dep))
	if err != nil {
		t.Fatalf("search groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != late.ID {
		t.Fatalf("server search must return the later matching group, got %#v", groups)
	}
}

func TestResolveShareGroupsUsesHashIDsAndReturnsSavedAudience(t *testing.T) {
	dep := newInfoGroupsTestDependency(t)
	ctx := context.Background()
	saved, err := dep.GroupClient().Upsert(ctx, &ent.Group{Name: "saved audience group", Permissions: &boolset.BooleanSet{}})
	if err != nil {
		t.Fatalf("create saved group: %v", err)
	}

	groups, err := (&ResolveShareGroupsService{IDs: []string{hashid.EncodeGroupID(dep.HashIDEncoder(), saved.ID)}}).Resolve(newInfoGroupsGinContext(t, dep))
	if err != nil {
		t.Fatalf("resolve saved groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != saved.ID || groups[0].Name != saved.Name {
		t.Fatalf("resolved saved group must include its identity, got %#v", groups)
	}
}

func newInfoGroupsGinContext(t *testing.T, dep dependency.Dep) *gin.Context {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.ContextWithFallback = true
	var captured *gin.Context
	router.GET("/", func(c *gin.Context) { captured = c })
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), dependency.DepCtx{}, dep))
	router.ServeHTTP(httptest.NewRecorder(), req)
	if captured == nil {
		t.Fatal("capture Gin context")
	}
	return captured
}

func newInfoGroupsTestDependency(t *testing.T) dependency.Dep {
	t.Helper()
	logger := logging.NewConsoleLogger(logging.LevelError)
	kv := cache.NewMemoStore("", logger)
	config := infoGroupsTestConfig{database: conf.Database{Type: conf.SQLiteDB, DBFile: filepath.Join(t.TempDir(), "groups.db")}, system: conf.System{Mode: conf.MasterMode, SessionSecret: "test-session", LogLevel: "error"}, cors: conf.Cors{AllowOrigins: []string{"UNSET"}}}
	raw, err := inventory.NewRawEntClient(logger, config)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	client, err := inventory.InitializeDBClient(logger, raw, kv, constants.BackendVersion)
	if err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return dependency.NewDependency(dependency.WithConfigProvider(config), dependency.WithDbClient(client), dependency.WithKV(kv))
}

type infoGroupsTestConfig struct {
	database conf.Database
	system   conf.System
	cors     conf.Cors
}

func (c infoGroupsTestConfig) Database() *conf.Database        { return &c.database }
func (c infoGroupsTestConfig) System() *conf.System            { return &c.system }
func (c infoGroupsTestConfig) SSL() *conf.SSL                  { return &conf.SSL{} }
func (c infoGroupsTestConfig) Unix() *conf.Unix                { return &conf.Unix{} }
func (c infoGroupsTestConfig) Slave() *conf.Slave              { return &conf.Slave{} }
func (c infoGroupsTestConfig) Redis() *conf.Redis              { return &conf.Redis{} }
func (c infoGroupsTestConfig) Cors() *conf.Cors                { return &c.cors }
func (c infoGroupsTestConfig) OptionOverwrite() map[string]any { return map[string]any{} }
