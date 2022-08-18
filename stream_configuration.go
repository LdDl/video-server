package videoserver

import (
	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
)

// StreamConfiguration is a configuration parameters for specific stream
type StreamConfiguration struct {
	URL                  string               `json:"url"`
	Status               bool                 `json:"status"`
	SupportedStreamTypes []string             `json:"supported_stream_types"`
	Codecs               []av.CodecData       `json:"codecs"`
	Clients              map[uuid.UUID]viewer `json:"-"`
	hlsChanel            chan av.Packet
	verbose              bool
	verboseDetailed      bool
}

// NewStreamConfiguration returns default configuration
func NewStreamConfiguration(streamURL string, supportedTypes []string) *StreamConfiguration {
	return &StreamConfiguration{
		URL:                  streamURL,
		Clients:              make(map[uuid.UUID]viewer),
		hlsChanel:            make(chan av.Packet, 100),
		SupportedStreamTypes: supportedTypes,
	}
}
