package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDriverSet(t *testing.T) {
	asserts := assert.New(t)
	store := newTestMemoStore()

	asserts.NoError(store.Set("123", "321", -1))
}

func TestDriverGet(t *testing.T) {
	asserts := assert.New(t)
	store := newTestMemoStore()
	asserts.NoError(store.Set("123", "321", -1))

	value, ok := store.Get("123")
	asserts.True(ok)
	asserts.Equal("321", value)

	value, ok = store.Get("not_exist")
	asserts.False(ok)
}

func TestDriverDelete(t *testing.T) {
	asserts := assert.New(t)
	store := newTestMemoStore()
	asserts.NoError(store.Set("123", "321", -1))
	err := store.Delete("", "123")
	asserts.NoError(err)
	_, exist := store.Get("123")
	asserts.False(exist)
}

func TestDriverGets(t *testing.T) {
	asserts := assert.New(t)
	store := newTestMemoStore()
	asserts.NoError(store.Set("test_1", "1", -1))

	values, missed := store.Gets([]string{"1", "2"}, "test_")
	asserts.Equal(map[string]any{"1": "1"}, values)
	asserts.Equal([]string{"2"}, missed)
}

func TestDriverSets(t *testing.T) {
	asserts := assert.New(t)
	store := newTestMemoStore()

	err := store.Sets(map[string]any{"3": "3", "4": "4"}, "test_")
	asserts.NoError(err)
	value1, _ := store.Get("test_3")
	value2, _ := store.Get("test_4")
	asserts.Equal("3", value1)
	asserts.Equal("4", value2)
}

func TestNewMemoStoreImplementsDriver(t *testing.T) {
	var store Driver = newTestMemoStore()
	assert.NotNil(t, store)
}
