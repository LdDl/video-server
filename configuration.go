package videoserver

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
)

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5
)

// @todo: can we switch to TOML? Any benefits?

// ConfigurationArgs is a configuration parameters for application as JSON-file
type ConfigurationArgs struct {
	Server     ServerConfiguration `json:"server"`
	Streams    []StreamArg         `json:"streams"`
	HLSConfig  HLSConfiguration    `json:"hls"`
	CorsConfig CorsConfiguration   `json:"cors_config"`
}

// HLSConfiguration is a HLS configuration for every stream with provided "hls" type in 'stream_types' field
type HLSConfiguration struct {
	MsPerSegment int64  `json:"ms_per_segment"`
	Directory    string `json:"directory"`
	WindowSize   uint   `json:"window_size"`
	Capacity     uint   `json:"window_capacity"`
}

// CorsConfiguration is a configuration of CORS requests
type CorsConfiguration struct {
	UseCORS          bool     `json:"use_cors"`
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
}

// StreamArg is an information about stream's source
type StreamArg struct {
	GUID        string   `json:"guid"`
	URL         string   `json:"url"`
	StreamTypes []string `json:"stream_types"`
	Verbose     string   `json:"verbose"`
}

// ServerConfiguration is a configuration parameters for server
type ServerConfiguration struct {
	HTTPAddr      string `json:"http_addr"`
	VideoHTTPPort int    `json:"video_http_port"`
	APIHTTPPort   int    `json:"api_http_port"`
}

// NewConfiguration returns new application configuration
func NewConfiguration(fname string) (*ConfigurationArgs, error) {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, errors.Wrap(err, "Can't read file")
	}
	conf := ConfigurationArgs{}
	err = json.Unmarshal(data, &conf)
	if err != nil {
		return nil, errors.Wrap(err, "Can't unmarshal file's content")
	}
	if conf.HLSConfig.Directory == "" {
		conf.HLSConfig.Directory = defaultHlsDir
	}
	if conf.HLSConfig.MsPerSegment == 0 {
		conf.HLSConfig.MsPerSegment = defaultHlsMsPerSegment
	}
	if conf.HLSConfig.Capacity == 0 {
		conf.HLSConfig.Capacity = defaultHlsCapacity
	}
	if conf.HLSConfig.WindowSize == 0 {
		conf.HLSConfig.WindowSize = defaultHlsWindowSize
	}
	if conf.HLSConfig.WindowSize > conf.HLSConfig.Capacity {
		conf.HLSConfig.WindowSize = conf.HLSConfig.Capacity
	}
	return &conf, nil
}
