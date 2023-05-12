package videoserver

import "strings"

type StreamType uint16

const (
	STREAM_TYPE_UNDEFINED = StreamType(iota)
	STREAM_TYPE_RTSP
	STREAM_TYPE_HLS
	STREAM_TYPE_MSE
)

func (iotaIdx StreamType) String() string {
	return [...]string{"undefined", "rtsp", "hls", "mse"}[iotaIdx]
}

var (
	supportedInputStreamTypes = map[StreamType]struct{}{
		STREAM_TYPE_RTSP: {},
	}
	supportedOutputStreamTypes = map[StreamType]struct{}{
		STREAM_TYPE_HLS: {},
		STREAM_TYPE_MSE: {},
	}
	supportedStreamTypes = map[string]StreamType{
		"rtsp": STREAM_TYPE_RTSP,
		"hls":  STREAM_TYPE_HLS,
		"mse":  STREAM_TYPE_MSE,
	}
)

func streamTypeExists(typeName string) (StreamType, bool) {
	v, ok := supportedStreamTypes[strings.ToLower(typeName)]
	return v, ok
}
