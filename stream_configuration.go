package videoserver

import (
	"context"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
)

// StreamConfiguration is a configuration parameters for specific stream
type StreamConfiguration struct {
	URL    string `json:"url"`
	Status bool   `json:"status"`
	// OnDemand is true when the upstream is pulled only while there is demand (viewers or recording)
	OnDemand bool `json:"on_demand"`
	// Online is the last known health of the source. For a streaming source it follows the live session,
	// for an idle on-demand source it is set by the health monitor
	Online bool `json:"online"`
	// Streaming is true while the upstream loop is running (dialing or receiving)
	Streaming bool `json:"streaming"`
	// Viewers is the number of MSE clients currently attached
	Viewers              int                  `json:"viewers"`
	LastHealthCheck      time.Time            `json:"last_health_check"`
	LastHealthError      string               `json:"last_health_error"`
	SupportedOutputTypes []StreamType         `json:"supported_output_types"`
	Codecs               []av.CodecData       `json:"codecs"`
	Clients              map[uuid.UUID]viewer `json:"-"`
	hlsChanel            chan av.Packet
	mp4Chanel            chan av.Packet
	verboseLevel         VerboseLevel
	archive              *StreamArchiveWrapper
	streamType           StreamType
	loop                 bool
	// recording is true when this stream writes archive segments, so its upstream must never be released
	recording bool

	// Upstream lifecycle. Guarded by the owning StreamsStorage mutex
	running        bool
	stopping       bool
	runCancel      context.CancelFunc
	runDone        chan struct{}
	idleTimer      *time.Timer
	hlsLastRequest time.Time
}

// isPermanent reports whether the upstream must be kept alive regardless of viewers
func (stream *StreamConfiguration) isPermanent() bool {
	return !stream.OnDemand || stream.recording
}

// hasDemand reports whether anything currently needs the upstream to be alive
func (stream *StreamConfiguration) hasDemand(idle time.Duration) bool {
	if stream.isPermanent() || len(stream.Clients) > 0 {
		return true
	}
	return !stream.hlsLastRequest.IsZero() && time.Since(stream.hlsLastRequest) < idle
}

// NewStreamConfiguration returns default configuration
func NewStreamConfiguration(streamURL string, supportedTypes []StreamType) *StreamConfiguration {
	return &StreamConfiguration{
		URL:                  streamURL,
		Clients:              make(map[uuid.UUID]viewer),
		hlsChanel:            make(chan av.Packet, 100),
		mp4Chanel:            make(chan av.Packet, 100),
		SupportedOutputTypes: supportedTypes,
	}
}
