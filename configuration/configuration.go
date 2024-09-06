package configuration

import (
	"encoding/json"
	"os"
)

// @todo: can we switch to TOML? Any benefits?

// Configuration represents user defined settings for video server
type Configuration struct {
	APICfg         APIConfiguration            `json:"api"`
	VideoServerCfg VideoConfiguration          `json:"video"`
	HLSCfg         HLSConfiguration            `json:"hls"`
	ArchiveCfg     ArchiveConfiguration        `json:"archive"`
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

// HLSConfiguration is a HLS configuration for every stream with provided "hls" type in 'output_types' field of 'rtsp_streams' objects
type HLSConfiguration struct {
	MsPerSegment int64  `json:"ms_per_segment"`
	Directory    string `json:"directory"`
	WindowSize   uint   `json:"window_size"`
	Capacity     uint   `json:"window_capacity"`
}

// ArchiveConfiguration is a archive configuration for every stream with enabled archive option
type ArchiveConfiguration struct {
	MsPerSegment int64  `json:"ms_per_file"`
	Directory    string `json:"directory"`
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
	GUID        string                     `json:"guid"`
	URL         string                     `json:"url"`
	Type        string                     `json:"type"`
	OutputTypes []string                   `json:"output_types"`
	Archive     StreamArchiveConfiguration `json:"archive"`
	// Level of verbose. Pick 'v' or 'vvv' (or leave it empty)
	Verbose string `json:"verbose"`
}

// StreamArchiveConfiguration is a archive configuration for cpecific stream. I can overwrite parent archive options in needed
type StreamArchiveConfiguration struct {
	Enabled      bool   `json:"enabled"`
	MsPerSegment int64  `json:"ms_per_file"`
	Directory    string `json:"directory"`
}

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5
)

// PrepareConfiguration
func PrepareConfiguration(fname string) (*Configuration, error) {
	configFile, err := os.ReadFile(fname)
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
	for i := range cfg.RTSPStreams {
		stream := cfg.RTSPStreams[i]
		archiveCfg := stream.Archive
		if !archiveCfg.Enabled {
			continue
		}
		if archiveCfg.Directory == "" {
			if cfg.ArchiveCfg.Directory != "" {
				cfg.RTSPStreams[i].Archive.Directory = cfg.ArchiveCfg.Directory
			} else {
				cfg.RTSPStreams[i].Archive.Directory = "./mp4"
			}
		}
		if archiveCfg.MsPerSegment == 0 {
			if cfg.ArchiveCfg.MsPerSegment > 0 {
				cfg.RTSPStreams[i].Archive.MsPerSegment = cfg.ArchiveCfg.MsPerSegment
			} else {
				cfg.RTSPStreams[i].Archive.MsPerSegment = 30
			}
		}

	}
	return cfg, nil
}
