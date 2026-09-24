package videoserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LdDl/video-server/configuration"
	"github.com/google/uuid"
)

const testVideoFile = "cmd/video_server/BigBuckBunny_320x180.mp4"

func boolPtr(v bool) *bool { return &v }

// newLifecycleApp builds an application with one looping local-file stream
func newLifecycleApp(t *testing.T, onDemand *bool, globalOnDemand bool, idle time.Duration, outputTypes []string) (*Application, uuid.UUID) {
	t.Helper()
	if _, err := os.Stat(testVideoFile); err != nil {
		t.Skipf("test video %s is not available: %v", testVideoFile, err)
	}
	streamID := uuid.New()
	cfg := &configuration.Configuration{
		HLSCfg: configuration.HLSConfiguration{
			Directory:    filepath.Join(t.TempDir(), "hls"),
			MsPerSegment: 500,
			WindowSize:   2,
			Capacity:     4,
		},
		OnDemandCfg: configuration.OnDemandConfiguration{
			Enabled: globalOnDemand,
			IdleMs:  idle.Milliseconds(),
		},
		LocalFiles: []configuration.LocalFileConfiguration{{
			GUID:        streamID.String(),
			File:        testVideoFile,
			OutputTypes: outputTypes,
			Loop:        true,
			OnDemand:    onDemand,
		}},
	}
	app, err := NewApplication(cfg)
	if err != nil {
		t.Fatalf("NewApplication: %v", err)
	}
	t.Cleanup(func() { app.stopStream(streamID) })
	return app, streamID
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (app *Application) snapshot(streamID uuid.UUID) (running, stopping, streaming, status bool, viewers int) {
	app.Streams.RLock()
	defer app.Streams.RUnlock()
	s := app.Streams.store[streamID]
	return s.running, s.stopping, s.Streaming, s.Status, s.Viewers
}

func TestOnDemand_StartsOnFirstViewerAndStopsWhenIdle(t *testing.T) {
	idle := 200 * time.Millisecond
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"mse"})

	app.StartStreams()
	time.Sleep(50 * time.Millisecond)
	if running, _, _, _, _ := app.snapshot(streamID); running {
		t.Fatal("on-demand stream must not start without viewers")
	}

	clientID, _, err := app.Streams.AddViewer(streamID)
	if err != nil {
		t.Fatal(err)
	}
	app.acquireStream(streamID)
	waitFor(t, 5*time.Second, "upstream to come up", func() bool {
		_, _, _, status, _ := app.snapshot(streamID)
		return status
	})
	if _, _, streaming, _, viewers := app.snapshot(streamID); !streaming || viewers != 1 {
		t.Fatalf("expected streaming=true viewers=1, got streaming=%v viewers=%d", streaming, viewers)
	}

	app.Streams.DeleteViewer(streamID, clientID)
	app.releaseStream(streamID)
	// Must survive the idle window untouched
	time.Sleep(idle / 2)
	if running, _, _, _, _ := app.snapshot(streamID); !running {
		t.Fatal("upstream stopped before the idle timeout expired")
	}
	waitFor(t, 5*time.Second, "upstream to stop after idle", func() bool {
		running, _, streaming, _, _ := app.snapshot(streamID)
		return !running && !streaming
	})
	codecs, _ := app.Streams.GetCodecsDataForStream(streamID)
	if len(codecs) != 0 {
		t.Fatalf("codecs must be cleared after an on-demand stop, got %d", len(codecs))
	}
}

func TestOnDemand_ViewerReturningWithinIdleKeepsUpstream(t *testing.T) {
	idle := 300 * time.Millisecond
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"mse"})

	first, _, _ := app.Streams.AddViewer(streamID)
	app.acquireStream(streamID)
	waitFor(t, 5*time.Second, "upstream to come up", func() bool {
		_, _, _, status, _ := app.snapshot(streamID)
		return status
	})
	app.Streams.RLock()
	firstDone := app.Streams.store[streamID].runDone
	app.Streams.RUnlock()

	app.Streams.DeleteViewer(streamID, first)
	app.releaseStream(streamID)
	time.Sleep(idle / 3)

	second, _, _ := app.Streams.AddViewer(streamID)
	app.acquireStream(streamID)
	time.Sleep(idle * 2)

	app.Streams.RLock()
	s := app.Streams.store[streamID]
	sameLoop := s.runDone == firstDone
	running, timer := s.running, s.idleTimer
	app.Streams.RUnlock()
	if !running || !sameLoop {
		t.Fatalf("upstream must be the same loop, running=%v sameLoop=%v", running, sameLoop)
	}
	if timer != nil {
		t.Fatal("idle timer must be disarmed while a viewer is attached")
	}
	app.Streams.DeleteViewer(streamID, second)
	app.releaseStream(streamID)
}

func TestOnDemand_ReacquireDuringTeardownStartsExactlyOneLoop(t *testing.T) {
	idle := time.Millisecond
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"mse"})

	// Hammer the 0->1->0->1 transition so that some acquires land while a previous loop is still tearing down
	var last uuid.UUID
	for i := 0; i < 15; i++ {
		id, _, err := app.Streams.AddViewer(streamID)
		if err != nil {
			t.Fatal(err)
		}
		app.acquireStream(streamID)
		time.Sleep(3 * time.Millisecond)
		app.Streams.DeleteViewer(streamID, id)
		app.releaseStream(streamID)
		time.Sleep(3 * time.Millisecond)
		last = id
	}
	_ = last
	// Final state: one viewer attached, exactly one loop alive
	id, _, _ := app.Streams.AddViewer(streamID)
	app.acquireStream(streamID)
	waitFor(t, 10*time.Second, "single loop to settle", func() bool {
		running, stopping, streaming, status, _ := app.snapshot(streamID)
		return running && !stopping && streaming && status
	})
	app.Streams.DeleteViewer(streamID, id)
	app.releaseStream(streamID)
	waitFor(t, 10*time.Second, "loop to stop", func() bool {
		running, _, _, _, _ := app.snapshot(streamID)
		return !running
	})
}

func TestPermanentStream_StartsImmediately(t *testing.T) {
	app, streamID := newLifecycleApp(t, boolPtr(false), true, 100*time.Millisecond, []string{"mse"})
	app.StartStreams()
	waitFor(t, 5*time.Second, "permanent upstream to come up", func() bool {
		_, _, _, status, _ := app.snapshot(streamID)
		return status
	})
	// Release must be a no-op for permanent streams
	app.releaseStream(streamID)
	time.Sleep(300 * time.Millisecond)
	if running, _, _, _, _ := app.snapshot(streamID); !running {
		t.Fatal("permanent stream must never be stopped by release")
	}
}

func TestOnDemand_PerStreamOverrideWinsOverGlobal(t *testing.T) {
	app, streamID := newLifecycleApp(t, boolPtr(true), false, 100*time.Millisecond, []string{"mse"})
	app.StartStreams()
	time.Sleep(50 * time.Millisecond)
	app.Streams.RLock()
	onDemand, running := app.Streams.store[streamID].OnDemand, app.Streams.store[streamID].running
	app.Streams.RUnlock()
	if !onDemand || running {
		t.Fatalf("expected on_demand=true and idle, got on_demand=%v running=%v", onDemand, running)
	}
}

func TestOnDemand_HLSTouchStartsAndIdleStops(t *testing.T) {
	idle := 400 * time.Millisecond
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"hls"})

	app.touchHLS(streamID)
	waitFor(t, 5*time.Second, "upstream to come up on HLS touch", func() bool {
		_, _, _, status, _ := app.snapshot(streamID)
		return status
	})
	playlist := filepath.Join(app.HLS.Directory, streamID.String()+".m3u8")
	waitFor(t, 10*time.Second, "playlist to appear", func() bool {
		_, err := os.Stat(playlist)
		return err == nil
	})
	// Keep touching for a while: the stream must stay alive
	for i := 0; i < 4; i++ {
		time.Sleep(idle / 2)
		app.touchHLS(streamID)
	}
	if running, _, _, _, _ := app.snapshot(streamID); !running {
		t.Fatal("HLS activity must keep the upstream alive")
	}
	waitFor(t, 5*time.Second, "upstream to stop once HLS requests cease", func() bool {
		running, _, _, _, _ := app.snapshot(streamID)
		return !running
	})
}

func TestStopStream_WaitsForLoopExit(t *testing.T) {
	app, streamID := newLifecycleApp(t, boolPtr(false), false, 100*time.Millisecond, []string{"mse"})
	app.StartStreams()
	waitFor(t, 5*time.Second, "upstream to come up", func() bool {
		_, _, _, status, _ := app.snapshot(streamID)
		return status
	})
	app.stopStream(streamID)
	if running, _, streaming, status, _ := app.snapshot(streamID); running || streaming || status {
		t.Fatalf("stopStream must return only after the loop exited, got running=%v streaming=%v status=%v", running, streaming, status)
	}
}

func TestStatusJSON_ListsStreamsAndMilliseconds(t *testing.T) {
	app, streamID := newLifecycleApp(t, nil, true, 1500*time.Millisecond, []string{"mse"})
	app.OnDemand.HealthCheck = true
	app.OnDemand.HealthInterval = 30 * time.Second
	app.OnDemand.HealthTimeout = 3 * time.Second
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Streams map[string]struct {
			URL       string `json:"url"`
			OnDemand  bool   `json:"on_demand"`
			Online    bool   `json:"online"`
			Streaming bool   `json:"streaming"`
			Viewers   int    `json:"viewers"`
			LastError string `json:"last_health_error"`
		} `json:"streams"`
		OnDemand struct {
			Enabled          bool  `json:"enabled"`
			IdleMs           int64 `json:"idle_ms"`
			HealthCheck      bool  `json:"health_check"`
			HealthIntervalMs int64 `json:"health_interval_ms"`
			HealthTimeoutMs  int64 `json:"health_timeout_ms"`
		} `json:"on_demand"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	stream, ok := parsed.Streams[streamID.String()]
	if !ok {
		t.Fatalf("stream %s missing from status: %s", streamID, raw)
	}
	if !stream.OnDemand || stream.Streaming || stream.Viewers != 0 || stream.URL != testVideoFile {
		t.Fatalf("unexpected stream status: %+v", stream)
	}
	if parsed.OnDemand.IdleMs != 1500 || parsed.OnDemand.HealthIntervalMs != 30000 || parsed.OnDemand.HealthTimeoutMs != 3000 || !parsed.OnDemand.HealthCheck {
		t.Fatalf("durations must be milliseconds: %+v", parsed.OnDemand)
	}
}
