package videoserver

import (
	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func (app *Application) startMP4Cast(archive *StreamArchiveWrapper, streamID uuid.UUID, stopCast chan StopSignal, streamVerboseLevel VerboseLevel) error {
	if archive == nil {
		return ErrNullArchive
	}
	if RWMutexLocked(&app.Streams.RWMutex) {
		log.Warn().Str("fn", "startMP4Cast").Str("stream_id", streamID.String()).Msg("Locked already")
	}
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	channel := stream.mp4Chanel
	go func(arch *StreamArchiveWrapper, id uuid.UUID, mp4Chanel chan av.Packet, stop chan StopSignal, verbose VerboseLevel) {
		err := app.startMP4(arch, id, mp4Chanel, stop, verbose)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_START_CAST).Str("stream_id", id.String()).Msg("Error on MP4 cast start")
		}
	}(archive, streamID, channel, stopCast, streamVerboseLevel)
	return nil
}
