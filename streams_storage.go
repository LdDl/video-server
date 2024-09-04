package videoserver

import (
	"fmt"
	"sync"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/google/uuid"
)

// StreamsStorage Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsStorage struct {
	sync.RWMutex
	Streams map[uuid.UUID]*StreamConfiguration `json:"rtsp_streams"`
}

// NewStreamsStorageDefault prepares new allocated storage
func NewStreamsStorageDefault() StreamsStorage {
	return StreamsStorage{Streams: make(map[uuid.UUID]*StreamConfiguration)}
}

func (sm *StreamsStorage) GetStream(id uuid.UUID) (string, []StreamType) {
	sm.Lock()
	defer sm.Unlock()

	return sm.Streams[id].URL, sm.Streams[id].SupportedOutputTypes
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

func (streams *StreamsStorage) streamExists(streamID uuid.UUID) bool {
	streams.RLock()
	_, ok := streams.Streams[streamID]
	streams.RUnlock()
	return ok
}

func (streams *StreamsStorage) existsWithType(streamID uuid.UUID, streamType StreamType) bool {
	// @error: here is wrong?
	streams.Lock()
	defer streams.Unlock()
	curStream, ok := streams.Streams[streamID]
	if !ok {
		return false
	}
	supportedTypes := curStream.SupportedOutputTypes
	typeEnabled := typeExists(streamType, supportedTypes)
	return ok && typeEnabled
}

func (streams *StreamsStorage) addCodec(streamID uuid.UUID, codecs []av.CodecData) {
	streams.Lock()
	streams.Streams[streamID].Codecs = codecs
	streams.Unlock()
}

func (streams *StreamsStorage) getCodec(streamID uuid.UUID) ([]av.CodecData, error) {
	curStream, ok := streams.Streams[streamID]
	if !ok {
		return nil, ErrStreamNotFound
	}
	codecs := make([]av.CodecData, len(curStream.Codecs))
	for i, iface := range curStream.Codecs {
		switch codecType := iface.(type) {
		case aacparser.CodecData, h264parser.CodecData:
			codecs[i] = codecType
		default:
			return nil, fmt.Errorf("unknown codec type: %T", iface)
		}
	}
	return codecs, nil
}

func (streams *StreamsStorage) updateStreamStatus(streamID uuid.UUID, status bool) error {
	streams.Lock()
	curStream, ok := streams.Streams[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	curStream.Status = status
	streams.Streams[streamID] = curStream
	streams.Unlock()
	return nil
}

func (streams *StreamsStorage) addClient(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	// @error: here is wrong?
	streams.Lock()
	defer streams.Unlock()
	curStream, ok := streams.Streams[streamID]
	if !ok {
		return uuid.UUID{}, nil, ErrStreamNotFound
	}
	clientID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	ch := make(chan av.Packet, 100)
	curStream.Clients[clientID] = viewer{c: ch}
	return clientID, ch, nil
}

func (streams *StreamsStorage) deleteClient(streamID, clientID uuid.UUID) {
	streams.Lock()
	delete(streams.Streams[streamID].Clients, clientID)
	streams.Unlock()
}

func (streams *StreamsStorage) cast(streamID uuid.UUID, pck av.Packet, hlsEnabled bool) error {
	streams.Lock()
	curStream, ok := streams.Streams[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	if hlsEnabled {
		curStream.hlsChanel <- pck
	}
	archive := streams.getArchiveStream(streamID)
	if archive != nil {
		curStream.mp4Chanel <- pck
	}
	for _, v := range curStream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
	streams.Unlock()
	return nil
}

func (streams *StreamsStorage) setArchiveStream(streamID uuid.UUID, dir string, msPerSegment int64) error {
	if dir == "" {
		return fmt.Errorf("bad directory archive stream")
	}
	if msPerSegment == 0 {
		return fmt.Errorf("bad ms per segment archive stream")
	}
	newArhive := streamArhive{
		dir:          dir,
		msPerSegment: msPerSegment,
	}
	streams.Lock()
	streams.Streams[streamID].archive = &newArhive
	streams.Unlock()
	return nil
}

func (streams *StreamsStorage) getArchiveStream(streamID uuid.UUID) *streamArhive {
	streams.Lock()
	archive := streams.Streams[streamID].archive
	streams.Unlock()
	return archive
}
