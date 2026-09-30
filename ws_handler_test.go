package videoserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// startWSTestServer exposes the MSE handler over a real HTTP server
func startWSTestServer(t *testing.T, app *Application) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wshandler(app, &upgrader, w, r, VERBOSE_NONE)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// dialViewer connects as an MSE client and keeps draining the socket until it is closed
func dialViewer(t *testing.T, wsURL string, streamID uuid.UUID) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws/live?stream_id="+streamID.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return conn
}

func (app *Application) viewersOf(streamID uuid.UUID) int {
	app.Streams.RLock()
	defer app.Streams.RUnlock()
	return len(app.Streams.store[streamID].Clients)
}

// TestWSHandler_ViewerIsRemovedOnDisconnect guards the cleanup path of the MSE handler.
// A viewer that survives its websocket pins the upstream forever, because releaseStream
// sees a non-empty client list and never arms the idle timer
func TestWSHandler_ViewerIsRemovedOnDisconnect(t *testing.T) {
	idle := 300 * time.Millisecond
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"mse"})
	wsURL := startWSTestServer(t, app)

	conn := dialViewer(t, wsURL, streamID)
	waitFor(t, 10*time.Second, "viewer to be registered", func() bool {
		return app.viewersOf(streamID) == 1
	})
	waitFor(t, 10*time.Second, "upstream to come up", func() bool {
		_, _, streaming, _, _ := app.snapshot(streamID)
		return streaming
	})

	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	waitFor(t, 10*time.Second, "viewer to be removed", func() bool {
		return app.viewersOf(streamID) == 0
	})
	waitFor(t, 10*time.Second, "upstream to stop once nobody watches", func() bool {
		running, _, _, _, _ := app.snapshot(streamID)
		return !running
	})
}

// TestWSHandler_ViewersAreIndependent makes sure one client leaving does not drop the others
func TestWSHandler_ViewersAreIndependent(t *testing.T) {
	idle := 5 * time.Second
	app, streamID := newLifecycleApp(t, nil, true, idle, []string{"mse"})
	wsURL := startWSTestServer(t, app)

	first := dialViewer(t, wsURL, streamID)
	second := dialViewer(t, wsURL, streamID)
	waitFor(t, 10*time.Second, "both viewers to be registered", func() bool {
		return app.viewersOf(streamID) == 2
	})

	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	waitFor(t, 10*time.Second, "first viewer to be removed", func() bool {
		return app.viewersOf(streamID) == 1
	})
	if running, _, _, _, _ := app.snapshot(streamID); !running {
		t.Fatal("upstream must stay alive while the second viewer watches")
	}

	if err := second.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	waitFor(t, 10*time.Second, "second viewer to be removed", func() bool {
		return app.viewersOf(streamID) == 0
	})
}
