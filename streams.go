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
	streamsIDs := app.Streams.getKeys()
	for i := range streamsIDs {
		app.StartStream(streamsIDs[i])
	}
}

// StartStream starts single video stream
func (app *Application) StartStream(streamID uuid.UUID) {
	go func() {
		err := app.RunStream(context.Background(), streamID)
		if err != nil {
			log.Error().Err(err).Str("scope", "streaming").Str("event", "stream_run").Str("stream_id", streamID.String()).Msg("Error on stream runner")
		}
	}()
}

func (app *Application) RunStream(ctx context.Context, streamID uuid.UUID) error {
	url, supportedTypes := app.Streams.GetStream(streamID)
	hlsEnabled := typeExists(STREAM_TYPE_HLS, supportedTypes)
	archiveEnabled, err := app.Streams.archiveEnabled(streamID)
	if err != nil {
		return errors.Wrap(err, "Can't enable archive")
	}
	app.startLoop(ctx, streamID, url, hlsEnabled, archiveEnabled)
	return nil
}

// startLoop starts stream loop with dialing to certain RTSP
func (app *Application) startLoop(ctx context.Context, streamID uuid.UUID, url string, hlsEnabled, archiveEnabled bool) {
	select {
	case <-ctx.Done():
		log.Info().Str("scope", "streaming").Str("event", "stream_done").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Stream is done")
		return
	default:
		log.Info().Str("scope", "streaming").Str("event", "stream_start").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Stream must be establishment")
		err := app.runStream(streamID, url, hlsEnabled, archiveEnabled)
		if err != nil {
			log.Error().Err(err).Str("scope", "streaming").Str("event", "stream_restart").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Can't start stream")
		}
		log.Info().Str("scope", "streaming").Str("event", "stream_restart").Str("stream_id", streamID.String()).Str("stream_url", url).Any("restart_duration", restartStreamDuration).Msg("Stream must be re-establishment")
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
