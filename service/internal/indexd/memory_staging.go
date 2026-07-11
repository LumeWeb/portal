package indexd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// MemoryStagingBackend is an in-memory StagingBackend. It is primarily intended
// for development, testing, and single-node deployments where durability of
// staged data across restarts is not required.
type MemoryStagingBackend struct {
	mu   sync.RWMutex
	data map[string][]byte
	seq  int
}

// NewMemoryStagingBackend creates a new in-memory staging backend.
func NewMemoryStagingBackend() *MemoryStagingBackend {
	return &MemoryStagingBackend{data: make(map[string][]byte)}
}

func (m *MemoryStagingBackend) Put(_ context.Context, reader io.Reader) (string, error) {
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read staging data: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("staging/%d", m.seq)
	m.seq++
	m.data[key] = buf
	return key, nil
}

func (m *MemoryStagingBackend) Get(_ context.Context, stagingKey string, offset, length int64) (io.ReadCloser, error) {
	m.mu.RLock()
	data, ok := m.data[stagingKey]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("staging key not found: %s", stagingKey)
	}
	if offset > 0 {
		if int64(len(data)) < offset {
			return nil, fmt.Errorf("offset out of range")
		}
		data = data[offset:]
	}
	if length >= 0 && int64(len(data)) > length {
		data = data[:length]
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *MemoryStagingBackend) Delete(_ context.Context, stagingKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, stagingKey)
	return nil
}

func (m *MemoryStagingBackend) Size(_ context.Context, stagingKey string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[stagingKey]
	if !ok {
		return 0, fmt.Errorf("staging key not found: %s", stagingKey)
	}
	return int64(len(data)), nil
}
