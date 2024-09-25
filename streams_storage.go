package videoserver

import (
	"fmt"
	"sync"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// StreamsStorage Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsStorage struct {
	sync.RWMutex
	store map[uuid.UUID]*StreamConfiguration
}

// NewStreamsStorageDefault prepares new allocated storage
func NewStreamsStorageDefault() StreamsStorage {
	return StreamsStorage{store: make(map[uuid.UUID]*StreamConfiguration)}
}

// GetStreamInfo returns stream URL and its supported output types
func (streams *StreamsStorage) GetStreamInfo(streamID uuid.UUID) (string, []StreamType) {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "GetStreamInfo").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return "", []StreamType{}
	}
	return stream.URL, stream.SupportedOutputTypes
}

// GetAllStreamsIDS returns all storage streams' keys as slice
func (streams *StreamsStorage) GetAllStreamsIDS() []uuid.UUID {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "GetAllStreamsIDS").Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	keys := make([]uuid.UUID, 0, len(streams.store))
	for k := range streams.store {
		keys = append(keys, k)
	}
	return keys
}

// StreamExists checks whenever given stream ID exists in storage
func (streams *StreamsStorage) StreamExists(streamID uuid.UUID) bool {
	if RWMutexRLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "StreamExists").Str("stream_id", streamID.String()).Msg("RLocked already")
	}
	streams.RLock()
	defer streams.RUnlock()
	_, ok := streams.store[streamID]
	return ok
}

// TypeExistsForStream checks whenever specific stream ID supports then given output stream type
func (streams *StreamsStorage) TypeExistsForStream(streamID uuid.UUID, streamType StreamType) bool {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "TypeExistsForStream").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return false
	}
	supportedTypes := stream.SupportedOutputTypes
	typeEnabled := typeExists(streamType, supportedTypes)
	return ok && typeEnabled
}

// AddCodecForStream appends new codecs data for the given stream
func (streams *StreamsStorage) AddCodecForStream(streamID uuid.UUID, codecs []av.CodecData) error {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "AddCodecForStream").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	stream.Codecs = codecs
	if stream.verboseLevel > VERBOSE_SIMPLE {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CODEC_ADD).Str("stream_id", streamID.String()).Any("codec_data", codecs).Msg("Add codec")
	}
	return nil
}

// GetCodecsDataForStream returns COPY of codecs data for the given stream
func (streams *StreamsStorage) GetCodecsDataForStream(streamID uuid.UUID) ([]av.CodecData, error) {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "GetCodecsDataForStream").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return nil, ErrStreamNotFound
	}
	codecs := make([]av.CodecData, len(stream.Codecs))
	for i, iface := range stream.Codecs {
		switch codecType := iface.(type) {
		case aacparser.CodecData, h264parser.CodecData:
			codecs[i] = codecType
		default:
			return nil, fmt.Errorf("unknown codec type: %T", iface)
		}
	}
	return codecs, nil
}

// UpdateStreamStatus sets new status value for the given stream
func (streams *StreamsStorage) UpdateStreamStatus(streamID uuid.UUID, status bool) error {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "UpdateStreamStatus").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	stream.Status = status
	if stream.verboseLevel > VERBOSE_SIMPLE {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_STATUS_UPDATE).Str("stream_id", streamID.String()).Bool("status", status).Msg("Status update")
	}
	return nil
}

// AddViewer adds client to the given stream. Return newly client ID, buffered channel for stream on success
func (streams *StreamsStorage) AddViewer(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "AddViewer").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return uuid.UUID{}, nil, ErrStreamNotFound
	}
	clientID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	if stream.verboseLevel > VERBOSE_SIMPLE {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CLIENT_ADD).Str("stream_id", streamID.String()).Str("client_id", clientID.String()).Msg("Add client")
	}
	ch := make(chan av.Packet, 100)
	stream.Clients[clientID] = viewer{c: ch}
	return clientID, ch, nil
}

// DeleteViewer removes given client from the stream
func (streams *StreamsStorage) DeleteViewer(streamID, clientID uuid.UUID) {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "DeleteViewer").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return
	}
	if stream.verboseLevel > VERBOSE_SIMPLE {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CLIENT_DELETE).Str("stream_id", streamID.String()).Str("client_id", clientID.String()).Msg("Delete client")
	}
	delete(stream.Clients, clientID)
}

// CastPacket cast AV Packet to viewers and possible to HLS/MP4 channels
func (streams *StreamsStorage) CastPacket(streamID uuid.UUID, pck av.Packet, hlsEnabled, archiveEnabled bool) error {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "CastPacket").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	stream, ok := streams.store[streamID]
	if !ok {
		streams.Unlock()
		return ErrStreamNotFound
	}
	streams.Unlock()
	if stream.verboseLevel > VERBOSE_ADD {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CAST_PACKET).Str("stream_id", streamID.String()).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", stream.archive != nil).Int("clients_num", len(stream.Clients)).Msg("Cast packet")
	}
	if hlsEnabled {
		if stream.verboseLevel > VERBOSE_ADD {
			log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CAST_PACKET).Str("stream_id", streamID.String()).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", stream.archive != nil).Int("clients_num", len(stream.Clients)).Msg("Cast packet to HLS")
		}
		stream.hlsChanel <- pck
	}
	if archiveEnabled {
		if stream.verboseLevel > VERBOSE_ADD {
			log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CAST_PACKET).Str("stream_id", streamID.String()).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", stream.archive != nil).Int("clients_num", len(stream.Clients)).Msg("Cast packet to MP4")
		}
		stream.mp4Chanel <- pck
	}
	if stream.verboseLevel > VERBOSE_ADD {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CAST_PACKET).Str("stream_id", streamID.String()).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", stream.archive != nil).Int("clients_num", len(stream.Clients)).Msg("Cast packet to viewers")
	}
	for _, v := range stream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
	if stream.verboseLevel > VERBOSE_ADD {
		log.Info().Str("scope", SCOPE_STREAM).Str("event", EVENT_STREAM_CAST_PACKET).Str("stream_id", streamID.String()).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", stream.archive != nil).Int("clients_num", len(stream.Clients)).Msg("Done casting")
	}
	return nil
}

// GetVerboseLevelForStream returst verbose level for the given stream
func (streams *StreamsStorage) GetVerboseLevelForStream(streamID uuid.UUID) VerboseLevel {
	if RWMutexRLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "GetVerboseLevelForStream").Str("stream_id", streamID.String()).Msg("RLocked already")
	}
	streams.RLock()
	defer streams.RUnlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return VERBOSE_NONE
	}
	return stream.verboseLevel
}

// IsArchiveEnabledForStream returns whenever archive has been enabled for stream
func (streams *StreamsStorage) IsArchiveEnabledForStream(streamID uuid.UUID) (bool, error) {
	if RWMutexRLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "IsArchiveEnabledForStream").Str("stream_id", streamID.String()).Msg("RLocked already")
	}
	streams.RLock()
	defer streams.RUnlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return false, ErrStreamNotFound
	}
	return stream.archive != nil, nil
}

// UpdateArchiveStorageForStream updates archive storage configuration (it override existing one!)
func (streams *StreamsStorage) UpdateArchiveStorageForStream(streamID uuid.UUID, archiveStorage *StreamArchiveWrapper) error {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "UpdateArchiveStorageForStream").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	stream.archive = archiveStorage
	return nil
}

// GetStreamArchiveStorage returns pointer to the archive storage for the given stream
func (streams *StreamsStorage) GetStreamArchiveStorage(streamID uuid.UUID) *StreamArchiveWrapper {
	if RWMutexLocked(&streams.RWMutex) {
		log.Warn().Str("fn", "GetStreamArchiveStorage").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	streams.Lock()
	defer streams.Unlock()
	stream, ok := streams.store[streamID]
	if !ok {
		return nil
	}
	return stream.archive
}
