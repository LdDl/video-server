package videoserver

import (
	"github.com/LdDl/vdk/av"
	"github.com/google/uuid"
)

// StreamConfiguration Configuration parameters for stream
type StreamConfiguration struct {
	URL                  string   `json:"url"`
	Status               bool     `json:"status"`
	SupportedStreamTypes []string `json:"supported_stream_types"`
	Codecs               []av.CodecData
	Clients              map[uuid.UUID]viewer
	hlsChanel            chan av.Packet
}

func NewStreamConfiguration(streamURL string, supportedTypes []string) *StreamConfiguration {
	return &StreamConfiguration{
		URL:                  streamURL,
		Clients:              make(map[uuid.UUID]viewer),
		hlsChanel:            make(chan av.Packet, 100),
		SupportedStreamTypes: supportedTypes,
	}
}
