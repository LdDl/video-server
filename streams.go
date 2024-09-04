package videoserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	restartStreamDuration = 5 * time.Second
)

// StartStreams starts all video streams
func (app *Application) StartStreams() {
	streamsIDs := app.Streams.getKeys()
	for _, k := range streamsIDs {
		app.StartStream(k)
	}
}

// StartStream starts single video stream
func (app *Application) StartStream(k uuid.UUID) {
	go app.RunStream(context.Background(), k)
}

func (app *Application) RunStream(ctx context.Context, k uuid.UUID) {
	url, supportedTypes := app.Streams.GetStream(k)
	hlsEnabled := typeExists(STREAM_TYPE_HLS, supportedTypes)
	app.startLoop(ctx, k, url, hlsEnabled)
}

// startLoop starts stream loop with dialing to certain RTSP
func (app *Application) startLoop(ctx context.Context, streamID uuid.UUID, url string, hlsEnabled bool) {
	select {
	case <-ctx.Done():
		return
	default:
		log.Info().Str("scope", "streaming").Str("event", "stream_start").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Stream must be establishment")
		err := app.runStream(streamID, url, hlsEnabled)
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
