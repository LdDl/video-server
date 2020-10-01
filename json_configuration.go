package videoserver

// ConfigurationArgs Configuration parameters for application as JSON-file
type ConfigurationArgs struct {
	Server          ServerConfiguration `json:"server"`
	Streams         []StreamArg         `json:"streams"`
	HlsMsPerSegment int64               `json:"hls_ms_per_segment"`
	HlsDirectory    string              `json:"hls_directory"`
	HlsWindowSize   uint                `json:"hls_window_size"`
	HlsCapacity     uint                `json:"hls_window_capacity"`
}

// StreamArg Infromation about stream's source
type StreamArg struct {
	GUID        string   `json:"guid"`
	URL         string   `json:"url"`
	StreamTypes []string `json:"stream_types"`
}

// ServerConfiguration Configuration parameters for server
type ServerConfiguration struct {
	HTTPAddr string `json:"http_addr"`
	HTTPPort int    `json:"http_port"`
}
