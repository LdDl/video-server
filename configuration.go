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

// NewConfiguration Constructor for ConfigurationArgs
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
	if conf.HlsDirectory == "" {
		conf.HlsDirectory = defaultHlsDir
	}
	if conf.HlsMsPerSegment == 0 {
		conf.HlsMsPerSegment = defaultHlsMsPerSegment
	}
	if conf.HlsCapacity == 0 {
		conf.HlsCapacity = defaultHlsCapacity
	}
	if conf.HlsWindowSize == 0 {
		conf.HlsWindowSize = defaultHlsWindowSize
	}
	if conf.HlsWindowSize > conf.HlsCapacity {
		conf.HlsWindowSize = conf.HlsCapacity
	}
	return &conf, nil
}
