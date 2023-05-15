package videoserver

import (
	"sync"

	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
)

// StreamsStorage Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsStorage struct {
	sync.Mutex
	Streams map[uuid.UUID]*StreamConfiguration `json:"rtsp_streams"`
}

// NewStreamsStorageDefault prepares new allocated storage
func NewStreamsStorageDefault() StreamsStorage {
	return StreamsStorage{Streams: make(map[uuid.UUID]*StreamConfiguration)}
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

func (sm *StreamsStorage) cast(streamID uuid.UUID, pck av.Packet, hlsEnabled bool) (err error) {
	sm.Lock()
	curStream, ok := sm.Streams[streamID]
	if !ok {
		err = ErrStreamNotFound
	}
	if hlsEnabled {
		curStream.hlsChanel <- pck
	}
	for _, v := range curStream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
	sm.Unlock()
	return err
}

func (sm *StreamsStorage) exists(streamID uuid.UUID) bool {
	sm.Lock()
	_, ok := sm.Streams[streamID]
	sm.Unlock()
	return ok
}

func (sm *StreamsStorage) existsWithType(streamID uuid.UUID, streamType StreamType) bool {
	sm.Lock()
	stream, ok := sm.Streams[streamID]
	if !ok {
		return false
	}
	supportedTypes := stream.SupportedOutputTypes
	typeEnabled := typeExists(streamType, supportedTypes)
	sm.Unlock()
	return ok && typeEnabled
}

func (sm *StreamsStorage) addCodec(streamID uuid.UUID, codecs []av.CodecData) {
	sm.Lock()
	sm.Streams[streamID].Codecs = codecs
	sm.Unlock()
}

func (sm *StreamsStorage) getCodec(streamID uuid.UUID) (codecs []av.CodecData, err error) {
	sm.Lock()
	curStream, ok := sm.Streams[streamID]
	if !ok {
		err = ErrStreamNotFound
	}
	codecs = curStream.Codecs
	sm.Unlock()
	return codecs, err
}

func (sm *StreamsStorage) updateStreamStatus(streamID uuid.UUID, status bool) (err error) {
	sm.Lock()
	t, ok := sm.Streams[streamID]
	if !ok {
		err = ErrStreamNotFound
	}
	t.Status = status
	sm.Streams[streamID] = t
	sm.Unlock()
	return err
}

func (sm *StreamsStorage) clientAdd(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	sm.Lock()
	clientID, err := uuid.NewUUID()
	ch := make(chan av.Packet, 100)
	curStream, ok := sm.Streams[streamID]
	if !ok {
		err = ErrStreamNotFound
	}
	curStream.Clients[clientID] = viewer{c: ch}
	sm.Unlock()
	return clientID, ch, err
}

func (sm *StreamsStorage) clientDelete(streamID, clientID uuid.UUID) {
	sm.Lock()
	delete(sm.Streams[streamID].Clients, clientID)
	sm.Unlock()
}
