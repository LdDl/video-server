package videoserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const (
	restartStreamDuration = 5 * time.Second
)

// RunStream runs single video stream loop until the context is cancelled
func (app *Application) RunStream(ctx context.Context, streamID uuid.UUID) error {
	url, supportedTypes := app.Streams.GetStreamInfo(streamID)
	if url == "" {
		return ErrStreamNotFound
	}
	hlsEnabled := typeExists(STREAM_TYPE_HLS, supportedTypes)
	archiveEnabled, err := app.Streams.IsArchiveEnabledForStream(streamID)
	if err != nil {
		return errors.Wrap(err, "Can't enable archive")
	}
	streamVerboseLevel := app.Streams.GetVerboseLevelForStream(streamID)
	app.startLoop(ctx, streamID, url, hlsEnabled, archiveEnabled, streamVerboseLevel)
	return nil
}

// startLoop starts stream loop with dialing to certain RTSP
func (app *Application) startLoop(ctx context.Context, streamID uuid.UUID, url string, hlsEnabled, archiveEnabled bool, streamVerboseLevel VerboseLevel) {
	app.Streams.Lock()
	stream, exists := app.Streams.store[streamID]
	app.Streams.Unlock()
	if !exists {
		log.Error().Str("scope", SCOPE_STREAMING).Str("stream_id", streamID.String()).Msg("Stream not found")
		return
	}

	for {
		select {
		case <-ctx.Done():
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DONE).Str("stream_id", streamID.String()).Str("stream_type", stream.streamType.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Stream is done")
			}
			return
		default:
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_START).Str("stream_id", streamID.String()).Str("stream_type", stream.streamType.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Stream must be establishment")
			}

			var err error
			switch stream.streamType {
			case STREAM_TYPE_RTSP:
				err = app.runStream(ctx, streamID, url, hlsEnabled, archiveEnabled, streamVerboseLevel)
			case STREAM_TYPE_LOCAL_FILE:
				err = app.runLocalFileStream(ctx, streamID, url, stream.loop, hlsEnabled, archiveEnabled, streamVerboseLevel)
			default:
				log.Error().Str("scope", SCOPE_STREAMING).Str("stream_id", streamID.String()).Str("stream_type", stream.streamType.String()).Msg("Unknown stream type")
				return
			}
			if ctx.Err() != nil {
				// Deliberate stop, not a source failure: leave the health state as it was
				continue
			}
			if err != nil {
				if streamVerboseLevel > VERBOSE_NONE {
					log.Error().Err(err).Str("scope", SCOPE_STREAMING).Str("stream_id", streamID.String()).Str("stream_type", stream.streamType.String()).Msg("Stream error")
				}
				_ = app.Streams.UpdateStreamHealth(streamID, false, err.Error())
			}
			if !sleepCtx(ctx, 2*time.Second) {
				continue
			}
		}
		if !sleepCtx(ctx, restartStreamDuration) {
			continue
		}
	}
}

// typeExists checks if a type exists in a types list
func typeExists(typ StreamType, types []StreamType) bool {
	for i := range types {
		if types[i] == typ {
			return true
		}
	}
	return false
}
