package videoserver

import (
	"sync"

	"github.com/google/uuid"
)

// StreamsMap Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsMap struct {
	sync.Mutex
	Streams map[uuid.UUID]*StreamConfiguration
}
