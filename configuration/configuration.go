package configuration

import (
	"encoding/json"
	"io/ioutil"
)

// @todo: can we switch to TOML? Any benefits?

// Configuration represents user defined settings for video server
type Configuration struct {
	APICfg         APIConfiguration            `json:"api"`
	VideoServerCfg VideoConfiguration          `json:"video"`
	HLSCfg         HLSConfiguration            `json:"hls"`
	CorsConfig     CORSConfiguration           `json:"cors"`
	RTSPStreams    []SingleStreamConfiguration `json:"rtsp_streams"`
}

// APIConfiguration is needed for configuring REST API part
type APIConfiguration struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int32  `json:"port"`
	// 'release' or 'debug' for GIN
	Mode string `json:"mode"`
}

// VideoConfiguration is needed for configuring actual video server part
type VideoConfiguration struct {
	Host string `json:"host"`
	Port int32  `json:"port"`
	// 'release' or 'debug' for GIN
	Mode string `json:"mode"`
}

// HLSConfiguration is a HLS configuration for every stream with provided "hls" type in 'stream_types' field of 'rtsp_streams' objects
type HLSConfiguration struct {
	MsPerSegment int64  `json:"ms_per_segment"`
	Directory    string `json:"directory"`
	WindowSize   uint   `json:"window_size"`
	Capacity     uint   `json:"window_capacity"`
}

// CORSConfiguration is settings for CORS
type CORSConfiguration struct {
	Enabled          bool     `json:"enabled"`
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
}

// SingleStreamConfiguration is needed for configuring certain RTSP stream
type SingleStreamConfiguration struct {
	GUID        string   `json:"guid"`
	URL         string   `json:"url"`
	StreamTypes []string `json:"stream_types"`
	// Level of verbose. Pick 'v' or 'vvv' (or leave it empty)
	Verbose string `json:"verbose"`
}

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5
)

// PrepareConfiguration
func PrepareConfiguration(fname string) (*Configuration, error) {
	configFile, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	cfg := &Configuration{}
	err = json.Unmarshal(configFile, &cfg)
	if err != nil {
		return nil, err
	}
	if cfg.HLSCfg.Directory == "" {
		cfg.HLSCfg.Directory = defaultHlsDir
	}
	if cfg.HLSCfg.MsPerSegment == 0 {
		cfg.HLSCfg.MsPerSegment = defaultHlsMsPerSegment
	}
	if cfg.HLSCfg.Capacity == 0 {
		cfg.HLSCfg.Capacity = defaultHlsCapacity
	}
	if cfg.HLSCfg.WindowSize == 0 {
		cfg.HLSCfg.WindowSize = defaultHlsWindowSize
	}
	if cfg.HLSCfg.WindowSize > cfg.HLSCfg.Capacity {
		cfg.HLSCfg.WindowSize = cfg.HLSCfg.Capacity
	}
	return cfg, nil
}
