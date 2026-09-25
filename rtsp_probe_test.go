package videoserver

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

const testSDPVideo = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=cam\r\nm=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\n"
const testSDPAudio = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=cam\r\nm=audio 0 RTP/AVP 97\r\n"

// fakeRTSPRequest is what the fake server hands to the scenario callback
type fakeRTSPRequest struct {
	method  string
	target  string
	headers map[string]string
}

// startFakeRTSP serves one connection at a time. The scenario returns the raw reply for each request;
// an empty reply means "stay silent"
func startFakeRTSP(t *testing.T, scenario func(req fakeRTSPRequest) string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					parts := strings.Fields(line)
					if len(parts) < 2 {
						return
					}
					req := fakeRTSPRequest{method: parts[0], target: parts[1], headers: map[string]string{}}
					for {
						header, err := reader.ReadString('\n')
						if err != nil {
							return
						}
						header = strings.TrimRight(header, "\r\n")
						if header == "" {
							break
						}
						key, value, _ := strings.Cut(header, ":")
						req.headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
					}
					reply := scenario(req)
					if reply == "" {
						// Silent server: hold the connection open until the client gives up
						time.Sleep(5 * time.Second)
						return
					}
					if _, err := c.Write([]byte(reply)); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return listener.Addr().String()
}

func rtspReply(status int, cseq string, sdp string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RTSP/1.0 %d X\r\nCSeq: %s\r\n", status, cseq)
	if sdp != "" {
		fmt.Fprintf(&b, "Content-Type: application/sdp\r\nContent-Length: %d\r\n", len(sdp))
	}
	b.WriteString("\r\n")
	b.WriteString(sdp)
	return b.String()
}

func TestProbeRTSP_OK(t *testing.T) {
	var seen []string
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		seen = append(seen, req.method)
		return rtspReply(200, req.headers["cseq"], testSDPVideo)
	})
	err := probeRTSP(context.Background(), "rtsp://"+addr+"/main", time.Second)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(seen) != 1 || seen[0] != "DESCRIBE" {
		t.Fatalf("expected exactly one DESCRIBE, got %v", seen)
	}
}

func TestProbeRTSP_DigestAuth(t *testing.T) {
	const realm, nonce = "cam-realm", "0123abcd"
	var authorized bool
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		auth := req.headers["authorization"]
		if auth == "" {
			return "RTSP/1.0 401 Unauthorized\r\nCSeq: " + req.headers["cseq"] + "\r\nWWW-Authenticate: Digest realm=\"" + realm + "\", nonce=\"" + nonce + "\"\r\n\r\n"
		}
		if !strings.Contains(req.target, "@") && strings.HasPrefix(auth, "Digest ") {
			fields := parseAuthParams(strings.TrimPrefix(auth, "Digest "))
			expected := md5Hex(md5Hex("admin:"+realm+":secret") + ":" + nonce + ":" + md5Hex("DESCRIBE:"+req.target))
			if fields["response"] == expected && fields["username"] == "admin" && fields["uri"] == req.target {
				authorized = true
				return rtspReply(200, req.headers["cseq"], testSDPVideo)
			}
		}
		return rtspReply(403, req.headers["cseq"], "")
	})
	err := probeRTSP(context.Background(), "rtsp://admin:secret@"+addr+"/main", time.Second)
	if err != nil {
		t.Fatalf("expected success with digest auth, got %v", err)
	}
	if !authorized {
		t.Fatal("server never saw a valid digest response")
	}
}

func TestProbeRTSP_BasicAuth(t *testing.T) {
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		if req.headers["authorization"] == "" {
			return "RTSP/1.0 401 Unauthorized\r\nCSeq: " + req.headers["cseq"] + "\r\nWWW-Authenticate: Basic realm=\"cam\"\r\n\r\n"
		}
		if req.headers["authorization"] == "Basic YWRtaW46c2VjcmV0" { // admin:secret
			return rtspReply(200, req.headers["cseq"], testSDPVideo)
		}
		return rtspReply(403, req.headers["cseq"], "")
	})
	if err := probeRTSP(context.Background(), "rtsp://admin:secret@"+addr+"/main", time.Second); err != nil {
		t.Fatalf("expected success with basic auth, got %v", err)
	}
}

func TestProbeRTSP_WrongCredentials(t *testing.T) {
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		return "RTSP/1.0 401 Unauthorized\r\nCSeq: " + req.headers["cseq"] + "\r\nWWW-Authenticate: Digest realm=\"cam\", nonce=\"n\"\r\n\r\n"
	})
	err := probeRTSP(context.Background(), "rtsp://admin:wrong@"+addr+"/main", time.Second)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestProbeRTSP_NoVideoTrack(t *testing.T) {
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		return rtspReply(200, req.headers["cseq"], testSDPAudio)
	})
	err := probeRTSP(context.Background(), "rtsp://"+addr+"/main", time.Second)
	if err == nil || !strings.Contains(err.Error(), "no video") {
		t.Fatalf("expected no-video error, got %v", err)
	}
}

func TestProbeRTSP_NotFound(t *testing.T) {
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		return rtspReply(404, req.headers["cseq"], "")
	})
	err := probeRTSP(context.Background(), "rtsp://"+addr+"/missing", time.Second)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestProbeRTSP_Timeout(t *testing.T) {
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string { return "" })
	start := time.Now()
	err := probeRTSP(context.Background(), "rtsp://"+addr+"/main", 300*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("probe did not respect timeout, took %v", elapsed)
	}
}

func TestProbeRTSP_ConnectionRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	err = probeRTSP(context.Background(), "rtsp://"+addr+"/main", time.Second)
	if err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatalf("expected connect error, got %v", err)
	}
}

func TestProbeRTSP_DefaultPort(t *testing.T) {
	// Only checks URL handling: a hostless port defaults to 554, which nothing listens on here
	err := probeRTSP(context.Background(), "rtsp://127.0.0.1/main", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected an error when nothing listens on 554")
	}
}

func TestProbeHost(t *testing.T) {
	cases := map[string]string{
		"rtsp://admin:admin@10.76.200.31:554/main": "10.76.200.31:554",
		"rtsp://10.160.1.47:2224/TOKEN/abc":        "10.160.1.47:2224",
		"rtsp://cam.local/live":                    "cam.local",
	}
	for raw, want := range cases {
		if got := probeHost(raw, STREAM_TYPE_RTSP); got != want {
			t.Errorf("probeHost(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := probeHost("/data/file.mp4", STREAM_TYPE_LOCAL_FILE); got != "local" {
		t.Errorf("local file host = %q", got)
	}
}

func TestProbeIdleStreams_PerHostLimit(t *testing.T) {
	var mu sync.Mutex
	inflight, peak := 0, 0
	addr := startFakeRTSP(t, func(req fakeRTSPRequest) string {
		mu.Lock()
		inflight++
		if inflight > peak {
			peak = inflight
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		return rtspReply(200, req.headers["cseq"], testSDPVideo)
	})
	app := &Application{Streams: NewStreamsStorageDefault(), OnDemand: OnDemandInfo{HealthCheck: true, HealthTimeout: time.Second}}
	ids := make([]uuid.UUID, 0, 12)
	for i := 0; i < 12; i++ {
		id := uuid.New()
		ids = append(ids, id)
		cfg := NewStreamConfiguration(fmt.Sprintf("rtsp://%s/cam%d", addr, i), []StreamType{STREAM_TYPE_MSE})
		cfg.streamType = STREAM_TYPE_RTSP
		cfg.OnDemand = true
		app.Streams.store[id] = cfg
	}
	app.probeIdleStreams(context.Background())
	if peak > healthProbePerHostConcurrency {
		t.Fatalf("per-host concurrency exceeded: peak %d, limit %d", peak, healthProbePerHostConcurrency)
	}
	for _, id := range ids {
		app.Streams.RLock()
		online := app.Streams.store[id].Online
		app.Streams.RUnlock()
		if !online {
			t.Fatalf("stream %s must be online after a successful probe", id)
		}
	}
}
