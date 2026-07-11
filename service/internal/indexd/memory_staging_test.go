package indexd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStagingBackend_PutGetDelete(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	// Put
	data := []byte("hello staging world")
	key, err := b.Put(ctx, bytes.NewReader(data))
	require.NoError(t, err)
	assert.NotEmpty(t, key)

	// Get — full read
	rc, err := b.Get(ctx, key, 0, -1)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, data, got)

	// Size
	size, err := b.Size(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), size)

	// Delete
	err = b.Delete(ctx, key)
	require.NoError(t, err)

	// Get after delete — should fail
	_, err = b.Get(ctx, key, 0, -1)
	assert.Error(t, err)

	// Size after delete — should fail
	_, err = b.Size(ctx, key)
	assert.Error(t, err)
}

func TestMemoryStagingBackend_RangeRead(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	data := []byte("0123456789ABCDEF")
	key, err := b.Put(ctx, bytes.NewReader(data))
	require.NoError(t, err)

	// Read from offset 4, length 4
	rc, err := b.Get(ctx, key, 4, 4)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, []byte("4567"), got)

	// Read from offset 4, length -1 (to end)
	rc, err = b.Get(ctx, key, 4, -1)
	require.NoError(t, err)
	got, _ = io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, data[4:], got)

	// Read from offset 0, length -1 (full)
	rc, err = b.Get(ctx, key, 0, -1)
	require.NoError(t, err)
	got, _ = io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, data, got)

	// Read with offset beyond data
	_, err = b.Get(ctx, key, int64(len(data)+1), -1)
	assert.Error(t, err)
}

func TestMemoryStagingBackend_MultiplePuts(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	keys := make([]string, 5)
	for i := 0; i < 5; i++ {
		data := []byte(strings.Repeat("x", i+1))
		key, err := b.Put(ctx, bytes.NewReader(data))
		require.NoError(t, err)
		keys[i] = key
		assert.NotEmpty(t, key)
	}

	// All keys should be unique
	seen := make(map[string]bool)
	for _, k := range keys {
		assert.False(t, seen[k], "duplicate key: %s", k)
		seen[k] = true
	}

	// Verify each key has correct size
	for i, k := range keys {
		size, err := b.Size(ctx, k)
		require.NoError(t, err)
		assert.Equal(t, int64(i+1), size)
	}
}

func TestMemoryStagingBackend_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	// Deleting a non-existent key should not error
	err := b.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestMemoryStagingBackend_GetNonExistent(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	_, err := b.Get(ctx, "nonexistent", 0, -1)
	assert.Error(t, err)

	_, err = b.Size(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestMemoryStagingBackend_Concurrent(t *testing.T) {
	ctx := context.Background()
	b := NewMemoryStagingBackend()

	// Concurrent puts and gets should not race
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			data := []byte{byte(i)}
			_, err := b.Put(ctx, bytes.NewReader(data))
			require.NoError(t, err)
		}
	}()

	// Read while writing
	for i := 0; i < 50; i++ {
		_, _ = b.Size(ctx, "staging/0")
	}

	<-done
}
