package videoserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Upstream lifecycle.
//
// A stream's upstream (RTSP session or local file reader) is alive while there is demand for it:
//   - always, when the stream is not on-demand or when it is being recorded (permanent streams);
//   - while at least one MSE viewer is attached;
//   - while HLS files were requested less than the idle timeout ago.
//
// One stream never has more than one upstream loop. The loop is started by ensureRunning and
// stopped by cancelling its context; `running` stays true until the loop goroutine has actually
// exited, so a viewer that arrives during teardown waits for it and then starts a fresh loop
// instead of opening a second connection to the camera.

// StartStreams starts upstream loops for permanent streams. On-demand streams wait for their first viewer
func (app *Application) StartStreams() {
	streamsIDs := app.Streams.GetAllStreamsIDS()
	for i := range streamsIDs {
		app.StartStream(streamsIDs[i])
	}
}

// StartStream starts the upstream loop for the given stream unless it is on-demand and nobody needs it yet
func (app *Application) StartStream(streamID uuid.UUID) {
	app.Streams.Lock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		app.Streams.Unlock()
		return
	}
	demand := stream.hasDemand(app.OnDemand.IdleTimeout)
	app.Streams.Unlock()
	if demand {
		app.ensureRunning(streamID)
	}
}

// ensureRunning starts the upstream loop when it is not running. Must be called without the storage lock held
func (app *Application) ensureRunning(streamID uuid.UUID) {
	app.Streams.Lock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		app.Streams.Unlock()
		return
	}
	if stream.idleTimer != nil {
		stream.idleTimer.Stop()
		stream.idleTimer = nil
	}
	if stream.running {
		if stream.stopping {
			// Teardown is in progress: wait for it and re-evaluate demand afterwards
			done := stream.runDone
			app.Streams.Unlock()
			go func() {
				<-done
				app.StartStream(streamID)
			}()
			return
		}
		app.Streams.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	stream.running = true
	stream.stopping = false
	stream.runCancel = cancel
	stream.runDone = done
	stream.Streaming = true
	verbose := stream.verboseLevel
	onDemand := stream.OnDemand
	app.Streams.Unlock()

	if verbose > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_ACQUIRE).Str("stream_id", streamID.String()).Bool("on_demand", onDemand).Msg("Starting upstream")
	}
	go func() {
		defer func() {
			app.Streams.Lock()
			if current, ok := app.Streams.store[streamID]; ok && current.runDone == done {
				current.running = false
				current.stopping = false
				current.runCancel = nil
				current.runDone = nil
				current.Streaming = false
				current.Status = false
				if current.OnDemand {
					// Codecs may change between sessions (camera reconfigured meanwhile).
					// The next viewer waits for fresh ones instead of getting a stale init segment
					current.Codecs = nil
				}
			}
			app.Streams.Unlock()
			cancel()
			close(done)
			if verbose > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DONE).Str("stream_id", streamID.String()).Msg("Upstream stopped")
			}
		}()
		err := app.RunStream(ctx, streamID)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_RUN).Str("stream_id", streamID.String()).Msg("Error on stream runner")
		}
	}()
}

// acquireStream is called after a viewer has been attached to the stream
func (app *Application) acquireStream(streamID uuid.UUID) {
	app.ensureRunning(streamID)
}

// releaseStream is called after a viewer has been detached from the stream.
// It schedules an idle stop for on-demand streams that nobody needs anymore
func (app *Application) releaseStream(streamID uuid.UUID) {
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.store[streamID]
	if !ok || stream.isPermanent() || !stream.running {
		return
	}
	if len(stream.Clients) > 0 {
		return
	}
	if stream.verboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_RELEASE).Str("stream_id", streamID.String()).Dur("idle_timeout", app.OnDemand.IdleTimeout).Msg("No viewers left, scheduling idle stop")
	}
	app.scheduleIdleStopLocked(stream, streamID, app.OnDemand.IdleTimeout)
}

// scheduleIdleStopLocked (re)arms the idle timer. Storage lock must be held
func (app *Application) scheduleIdleStopLocked(stream *StreamConfiguration, streamID uuid.UUID, after time.Duration) {
	if stream.idleTimer != nil {
		stream.idleTimer.Stop()
	}
	stream.idleTimer = time.AfterFunc(after, func() {
		app.idleStop(streamID)
	})
}

// idleStop is the idle timer callback: stops the upstream if demand is still absent
func (app *Application) idleStop(streamID uuid.UUID) {
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		return
	}
	stream.idleTimer = nil
	if stream.hasDemand(app.OnDemand.IdleTimeout) {
		if len(stream.Clients) == 0 && !stream.isPermanent() {
			// Only HLS activity keeps it alive: check again when that activity would expire
			remaining := app.OnDemand.IdleTimeout - time.Since(stream.hlsLastRequest)
			if remaining < 0 {
				remaining = 0
			}
			app.scheduleIdleStopLocked(stream, streamID, remaining)
		}
		return
	}
	if !stream.running || stream.stopping {
		return
	}
	if stream.verboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_IDLE_STOP).Str("stream_id", streamID.String()).Msg("Idle timeout expired, stopping upstream")
	}
	stream.stopping = true
	stream.runCancel()
}

// stopStream cancels the upstream loop and waits until it has exited. Safe to call for a stream that is not running
func (app *Application) stopStream(streamID uuid.UUID) {
	app.Streams.Lock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		app.Streams.Unlock()
		return
	}
	if stream.idleTimer != nil {
		stream.idleTimer.Stop()
		stream.idleTimer = nil
	}
	done := stream.runDone
	if stream.running && !stream.stopping {
		if stream.verboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_CANCEL).Str("stream_id", streamID.String()).Msg("Stopping upstream")
		}
		stream.stopping = true
		stream.runCancel()
	}
	app.Streams.Unlock()
	if done != nil {
		<-done
	}
}

// touchHLS registers HLS activity for the stream and makes sure its upstream is alive
func (app *Application) touchHLS(streamID uuid.UUID) {
	app.Streams.Lock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		app.Streams.Unlock()
		return
	}
	stream.hlsLastRequest = time.Now()
	if stream.isPermanent() {
		app.Streams.Unlock()
		return
	}
	if len(stream.Clients) == 0 {
		app.scheduleIdleStopLocked(stream, streamID, app.OnDemand.IdleTimeout)
	}
	alive := stream.running && !stream.stopping
	app.Streams.Unlock()
	if !alive {
		app.ensureRunning(streamID)
	}
}

// isStreamRunning reports whether the upstream loop is alive and not being torn down
func (app *Application) isStreamRunning(streamID uuid.UUID) bool {
	app.Streams.RLock()
	defer app.Streams.RUnlock()
	stream, ok := app.Streams.store[streamID]
	if !ok {
		return false
	}
	return stream.running && !stream.stopping
}

// sleepCtx sleeps for the given duration or until the context is done. Returns false when cancelled
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
