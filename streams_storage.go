package videoserver

import (
	"sync"

	"github.com/google/uuid"
)

// StreamsStorage Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsStorage struct {
	sync.Mutex
	Streams map[uuid.UUID]*StreamConfiguration
}

// NewStreamsStorageDefault prepares new allocated storage
func NewStreamsStorageDefault() *StreamsStorage {
	return &StreamsStorage{Streams: make(map[uuid.UUID]*StreamConfiguration)}
}

// getKeys returns all storage streams' keys as slice
func (sm *StreamsStorage) getKeys() []uuid.UUID {
	sm.Lock()
	keys := make([]uuid.UUID, 0, len(sm.Streams))
	for k := range sm.Streams {
		keys = append(keys, k)
	}
	sm.Unlock()
	return keys
}
