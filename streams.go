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

// StartStreams starts all video streams
func (app *Application) StartStreams() {
	streamsIDs := app.Streams.GetAllStreamsIDS()
	for i := range streamsIDs {
		app.StartStream(streamsIDs[i])
	}
}

// StartStream starts single video stream
func (app *Application) StartStream(streamID uuid.UUID) {
	go func(id uuid.UUID) {
		err := app.RunStream(context.Background(), id)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_RUN).Str("stream_id", id.String()).Msg("Error on stream runner")
		}
	}(streamID)
}

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
	for {
		select {
		case <-ctx.Done():
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DONE).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Stream is done")
			}
			return
		default:
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_START).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Stream must be establishment")
			}
			err := app.runStream(streamID, url, hlsEnabled, archiveEnabled, streamVerboseLevel)
			if err != nil {
				log.Error().Err(err).Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_RESTART).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Can't start stream")
			}
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_RESTART).Str("stream_id", streamID.String()).Str("stream_url", url).Dur("restart_duration", restartStreamDuration).Bool("hls_enabled", hlsEnabled).Bool("archive_enabled", archiveEnabled).Msg("Stream must be re-establishment")
			}
		}
		time.Sleep(restartStreamDuration)
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
