package conf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dadastory/CloudRevo/pkg/logging"
	"github.com/go-ini/ini"
	"github.com/stretchr/testify/assert"
)

func TestNewIniConfigProviderCreatesMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "conf.ini")

	provider, err := NewIniConfigProvider(path, logging.NewConsoleLogger(logging.LevelError))

	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.FileExists(t, path)
}

func TestNewIniConfigProviderRejectsMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.ini")
	assert.NoError(t, os.WriteFile(path, []byte("[Database]\nPassword233root"), 0o644))

	provider, err := NewIniConfigProvider(path, logging.NewConsoleLogger(logging.LevelError))

	assert.Nil(t, provider)
	assert.Error(t, err)
}

func TestNewIniConfigProviderMapsConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.ini")
	content := "[System]\nListen = :3000\n\n[Database]\nType = mysql\nUser = root\nPassword = root\nHost = 127.0.0.1\nName = cloudrevo\n"
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	provider, err := NewIniConfigProvider(path, logging.NewConsoleLogger(logging.LevelError))

	assert.NoError(t, err)
	assert.Equal(t, ":3000", provider.System().Listen)
	assert.Equal(t, MySqlDB, provider.Database().Type)
	assert.Equal(t, "cloudrevo", provider.Database().Name)
}

func TestMapSectionUsesCurrentSignature(t *testing.T) {
	cfg, err := ini.Load([]byte("[Database]\nType = mysql\nName = cloudrevo\n"))
	assert.NoError(t, err)

	database := *DatabaseConfig
	err = mapSection(cfg, "Database", &database)

	assert.NoError(t, err)
	assert.Equal(t, MySqlDB, database.Type)
	assert.Equal(t, "cloudrevo", database.Name)
}
