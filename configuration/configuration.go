package configuration

import (
	"fmt"
)

// @todo: can we switch to TOML? Any benefits?

// Configuration represents user defined settings for video server
type Configuration struct {
	APICfg         APIConfiguration            `json:"api" toml:"api"`
	VideoServerCfg VideoConfiguration          `json:"video" toml:"video"`
	HLSCfg         HLSConfiguration            `json:"hls" toml:"hls"`
	ArchiveCfg     ArchiveConfiguration        `json:"archive" toml:"archive"`
	CorsConfig     CORSConfiguration           `json:"cors" toml:"cors"`
	RTSPStreams    []SingleStreamConfiguration `json:"rtsp_streams" toml:"rtsp_streams"`
}

// APIConfiguration is needed for configuring REST API part
type APIConfiguration struct {
	Enabled bool   `json:"enabled" toml:"enabled"`
	Host    string `json:"host" toml:"host"`
	Port    int32  `json:"port" toml:"port"`
	// 'release' or 'debug' for GIN
	Mode    string `json:"mode" toml:"mode"`
	Verbose string `json:"verbose" toml:"verbose"`
}

// VideoConfiguration is needed for configuring actual video server part
type VideoConfiguration struct {
	Host string `json:"host" toml:"host"`
	Port int32  `json:"port" toml:"port"`
	// 'release' or 'debug' for GIN
	Mode    string `json:"mode" toml:"mode"`
	Verbose string `json:"verbose" toml:"verbose"`
}

// HLSConfiguration is a HLS configuration for every stream with provided "hls" type in 'output_types' field of 'rtsp_streams' objects
type HLSConfiguration struct {
	MsPerSegment int64  `json:"ms_per_segment" toml:"ms_per_segment"`
	Directory    string `json:"directory" toml:"directory"`
	WindowSize   uint   `json:"window_size" toml:"window_size"`
	Capacity     uint   `json:"window_capacity" toml:"window_capacity"`
}

// ArchiveConfiguration is a archive configuration for every stream with enabled archive option
type ArchiveConfiguration struct {
	Enabled      bool          `json:"enabled" toml:"enabled"`
	MsPerSegment int64         `json:"ms_per_file" toml:"ms_per_file"`
	Directory    string        `json:"directory" toml:"directory"`
	Minio        MinioSettings `json:"minio_settings" toml:"minio_settings"`
}

// MinioSettings
type MinioSettings struct {
	Host          string `json:"host" toml:"host"`
	Port          int32  `json:"port" toml:"port"`
	User          string `json:"user" toml:"user"`
	Password      string `json:"password" toml:"password"`
	DefaultBucket string `json:"default_bucket" toml:"default_bucket"`
	DefaultPath   string `json:"default_path" toml:"default_path"`
}

func (ms *MinioSettings) String() string {
	return fmt.Sprintf("Host '%s' Port '%d' User '%s' Pass '%s' Bucket '%s' Path '%s'", ms.Host, ms.Port, ms.User, ms.Password, ms.DefaultBucket, ms.DefaultPath)
}

// CORSConfiguration is settings for CORS
type CORSConfiguration struct {
	Enabled          bool     `json:"enabled" toml:"enabled"`
	AllowOrigins     []string `json:"allow_origins" toml:"allow_origins"`
	AllowMethods     []string `json:"allow_methods" toml:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers" toml:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers" toml:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials" toml:"allow_credentials"`
}

// SingleStreamConfiguration is needed for configuring certain RTSP stream
type SingleStreamConfiguration struct {
	GUID        string                     `json:"guid" toml:"guid"`
	URL         string                     `json:"url" toml:"url"`
	Type        string                     `json:"type" toml:"type"`
	OutputTypes []string                   `json:"output_types" toml:"output_types"`
	Archive     StreamArchiveConfiguration `json:"archive" toml:"archive"`
	// Level of verbose. Pick 'v' or 'vvv' (or leave it empty)
	Verbose string `json:"verbose" toml:"verbose"`
}

// StreamArchiveConfiguration is a archive configuration for cpecific stream. I can overwrite parent archive options in needed
type StreamArchiveConfiguration struct {
	Enabled      bool   `json:"enabled" toml:"enabled"`
	MsPerSegment int64  `json:"ms_per_file" toml:"ms_per_file"`
	Directory    string `json:"directory" toml:"directory"`
	TypeArchive  string `json:"type" toml:"type"`
	MinioBucket  string `json:"minio_bucket" toml:"minio_bucket"`
	MinioPath    string `json:"minio_path" toml:"minio_path"`
}
