package videoserver

import (
	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func (app *Application) startHlsCast(streamID uuid.UUID, stopCast chan StopSignal) error {
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	go func(id uuid.UUID, hlsChanel chan av.Packet, stop chan StopSignal) {
		err := app.startHls(id, hlsChanel, stop)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_START_CAST).Str("stream_id", id.String()).Msg("Error on HLS cast start")
		}
	}(streamID, stream.hlsChanel, stopCast)
	return nil
}
