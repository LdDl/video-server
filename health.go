package videoserver

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// healthProbeConcurrency caps how many sources are probed at the same time in one sweep
const healthProbeConcurrency = 8

// StartHealthMonitor periodically probes idle on-demand streams so that their `online` flag stays
// meaningful while no video is being pulled. Streams with a live upstream are skipped: the session
// itself is the health signal for them. Returns immediately when health checks are disabled
func (app *Application) StartHealthMonitor(ctx context.Context) {
	if !app.OnDemand.HealthCheck {
		return
	}
	log.Info().Str("scope", SCOPE_HEALTH).Str("event", EVENT_HEALTH_START).Dur("interval", app.OnDemand.HealthInterval).Dur("timeout", app.OnDemand.HealthTimeout).Msg("Starting health monitor for idle on-demand streams")
	ticker := time.NewTicker(app.OnDemand.HealthInterval)
	defer ticker.Stop()
	app.probeIdleStreams(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.probeIdleStreams(ctx)
		}
	}
}

// probeTarget is a snapshot of what the probe needs, taken under the storage lock
type probeTarget struct {
	id         uuid.UUID
	url        string
	streamType StreamType
	verbose    VerboseLevel
}

// probeIdleStreams runs one sweep over idle on-demand streams
func (app *Application) probeIdleStreams(ctx context.Context) {
	targets := app.idleOnDemandTargets()
	if len(targets) == 0 {
		return
	}
	sem := make(chan struct{}, healthProbeConcurrency)
	var wg sync.WaitGroup
	for _, target := range targets {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(t probeTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			app.probeOne(ctx, t)
		}(target)
	}
	wg.Wait()
}

// idleOnDemandTargets lists on-demand streams whose upstream is not running right now
func (app *Application) idleOnDemandTargets() []probeTarget {
	app.Streams.RLock()
	defer app.Streams.RUnlock()
	targets := make([]probeTarget, 0, len(app.Streams.store))
	for id, stream := range app.Streams.store {
		if !stream.OnDemand || stream.running {
			continue
		}
		targets = append(targets, probeTarget{id: id, url: stream.URL, streamType: stream.streamType, verbose: stream.verboseLevel})
	}
	return targets
}

// probeOne probes a single source and records the result.
// A result is dropped when the upstream has started meanwhile: the live session owns the status then
func (app *Application) probeOne(ctx context.Context, target probeTarget) {
	var err error
	switch target.streamType {
	case STREAM_TYPE_RTSP:
		err = probeRTSP(ctx, target.url, app.OnDemand.HealthTimeout)
	case STREAM_TYPE_LOCAL_FILE:
		_, err = os.Stat(target.url)
	default:
		return
	}
	if app.isStreamRunning(target.id) {
		return
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	if target.verbose > VERBOSE_SIMPLE || (err != nil && target.verbose > VERBOSE_NONE) {
		log.Info().Err(err).Str("scope", SCOPE_HEALTH).Str("event", EVENT_HEALTH_PROBE).Str("stream_id", target.id.String()).Bool("online", err == nil).Msg("Health probe finished")
	}
	_ = app.Streams.UpdateStreamHealth(target.id, err == nil, errText)
}
