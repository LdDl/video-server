package videoserver

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Minimal RTSP client that goes as far as DESCRIBE and never sends PLAY.
// It is enough to learn whether the endpoint is up, the path exists, credentials are accepted
// and the SDP has a video track, while the camera does not start sending any media.

const (
	rtspDefaultPort  = "554"
	rtspProbeMethod  = "DESCRIBE"
	rtspProbeAgent   = "video-server-health-probe"
	rtspMaxBodyBytes = 64 * 1024
)

// rtspResponse is a parsed RTSP reply
type rtspResponse struct {
	status  int
	headers map[string]string
	body    string
}

// probeRTSP checks that the RTSP source answers DESCRIBE with an SDP that has a video track
func probeRTSP(ctx context.Context, rawURL string, timeout time.Duration) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("bad url: %w", err)
	}
	host := parsed.Host
	if parsed.Port() == "" {
		host = net.JoinHostPort(parsed.Hostname(), rtspDefaultPort)
	}
	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	// Credentials travel in the Authorization header, not in the request line
	requestURL := *parsed
	requestURL.User = nil
	target := requestURL.String()

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)

	resp, err := rtspExchange(conn, reader, 1, target, "")
	if err != nil {
		return err
	}
	if resp.status == 401 && username != "" {
		auth, err := rtspAuthorization(resp.headers["www-authenticate"], username, password, target)
		if err != nil {
			return err
		}
		resp, err = rtspExchange(conn, reader, 2, target, auth)
		if err != nil {
			return err
		}
	}
	if resp.status != 200 {
		return fmt.Errorf("%s returned %d", rtspProbeMethod, resp.status)
	}
	if !strings.Contains(resp.body, "m=video") {
		return fmt.Errorf("no video track in SDP")
	}
	return nil
}

// rtspExchange sends one DESCRIBE and reads the reply
func rtspExchange(conn net.Conn, reader *bufio.Reader, cseq int, target, authorization string) (*rtspResponse, error) {
	var request strings.Builder
	fmt.Fprintf(&request, "%s %s RTSP/1.0\r\n", rtspProbeMethod, target)
	fmt.Fprintf(&request, "CSeq: %d\r\n", cseq)
	fmt.Fprintf(&request, "User-Agent: %s\r\n", rtspProbeAgent)
	request.WriteString("Accept: application/sdp\r\n")
	if authorization != "" {
		fmt.Fprintf(&request, "Authorization: %s\r\n", authorization)
	}
	request.WriteString("\r\n")
	if _, err := io.WriteString(conn, request.String()); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	return readRTSPResponse(reader)
}

// readRTSPResponse parses a status line, headers and an optional Content-Length body
func readRTSPResponse(reader *bufio.Reader) (*rtspResponse, error) {
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(statusLine), " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "RTSP/") {
		return nil, fmt.Errorf("not an RTSP response: %q", strings.TrimSpace(statusLine))
	}
	status, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("bad status code in %q", strings.TrimSpace(statusLine))
	}
	resp := &rtspResponse{status: status, headers: make(map[string]string)}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read headers: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			resp.headers[strings.ToLower(strings.TrimSpace(line[:idx]))] = strings.TrimSpace(line[idx+1:])
		}
	}
	if lengthText, ok := resp.headers["content-length"]; ok {
		length, err := strconv.Atoi(lengthText)
		if err != nil || length < 0 || length > rtspMaxBodyBytes {
			return nil, fmt.Errorf("bad content length %q", lengthText)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		resp.body = string(body)
	}
	return resp, nil
}

// rtspAuthorization builds an Authorization header for the challenge. Digest is preferred, Basic is the fallback
func rtspAuthorization(challenge, username, password, target string) (string, error) {
	if challenge == "" {
		return "", fmt.Errorf("401 without WWW-Authenticate")
	}
	scheme, params, _ := strings.Cut(challenge, " ")
	switch strings.ToLower(scheme) {
	case "digest":
		fields := parseAuthParams(params)
		realm, nonce := fields["realm"], fields["nonce"]
		if nonce == "" {
			return "", fmt.Errorf("digest challenge without nonce")
		}
		ha1 := md5Hex(username + ":" + realm + ":" + password)
		ha2 := md5Hex(rtspProbeMethod + ":" + target)
		response := md5Hex(ha1 + ":" + nonce + ":" + ha2)
		return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`, username, realm, nonce, target, response), nil
	case "basic":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password)), nil
	default:
		return "", fmt.Errorf("unsupported auth scheme %q", scheme)
	}
}

// parseAuthParams splits `k="v", k2=v2` into a map
func parseAuthParams(params string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(params, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
