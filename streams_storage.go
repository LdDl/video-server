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

func NewStreamsStorageDefault() *StreamsStorage {
	return &StreamsStorage{Streams: make(map[uuid.UUID]*StreamConfiguration)}
}
