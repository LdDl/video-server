package videoserver

import (
	"github.com/LdDl/video-server/configuration"
	"github.com/gin-contrib/cors"
	"github.com/pkg/errors"

	"github.com/deepch/vdk/av"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Application is a configuration parameters for application
type Application struct {
	APICfg         APIConfiguration   `json:"api"`
	VideoServerCfg VideoConfiguration `json:"video"`
	Streams        StreamsStorage     `json:"streams"`
	HLS            HLSInfo            `json:"hls"`
	CorsConfig     *cors.Config       `json:"-"`
}

// APIConfiguration is just copy of configuration.APIConfiguration but with some not exported fields
type APIConfiguration struct {
	Enabled bool   `json:"-"`
	Host    string `json:"host"`
	Port    int32  `json:"port"`
	Mode    string `json:"-"`
}

// VideoConfiguration is just copy of configuration.VideoConfiguration but with some not exported fields
type VideoConfiguration struct {
	Host string `json:"host"`
	Port int32  `json:"port"`
	Mode string `json:"-"`
}

// HLSInfo is an information about HLS parameters for server
type HLSInfo struct {
	MsPerSegment int64  `json:"hls_ms_per_segment"`
	Directory    string `json:"-"`
	WindowSize   uint   `json:"hls_window_size"`
	Capacity     uint   `json:"hls_window_capacity"`
}

// ServerInfo is an information about server
type ServerInfo struct {
	HTTPAddr      string `json:"http_addr"`
	VideoHTTPPort int32  `json:"http_port"`
	APIHTTPPort   int32  `json:"-"`
}

// NewApplication Prepare configuration for application
func NewApplication(cfg *configuration.Configuration) (*Application, error) {
	tmp := Application{
		APICfg: APIConfiguration{
			Enabled: cfg.APICfg.Enabled,
			Host:    cfg.APICfg.Host,
			Port:    cfg.APICfg.Port,
			Mode:    cfg.APICfg.Mode,
		},
		VideoServerCfg: VideoConfiguration{
			Host: cfg.VideoServerCfg.Host,
			Port: cfg.VideoServerCfg.Port,
		},
		Streams: NewStreamsStorageDefault(),
		HLS: HLSInfo{
			MsPerSegment: cfg.HLSCfg.MsPerSegment,
			Directory:    cfg.HLSCfg.Directory,
			WindowSize:   cfg.HLSCfg.WindowSize,
			Capacity:     cfg.HLSCfg.Capacity,
		},
	}
	if cfg.CorsConfig.Enabled {
		tmp.setCors(cfg.CorsConfig)
	}
	for _, rtspStream := range cfg.RTSPStreams {
		validUUID, err := uuid.Parse(rtspStream.GUID)
		if err != nil {
			log.Error().Err(err).Str("scope", "configuration").Str("stream_id", rtspStream.GUID).Msg("Not valid UUID")
			continue
		}
		outputTypes := make([]StreamType, 0, len(rtspStream.OutputTypes))
		for _, v := range rtspStream.OutputTypes {
			typ, ok := streamTypeExists(v)
			if !ok {
				return nil, errors.Wrapf(ErrStreamTypeNotExists, "Type: '%s'", v)
			}
			if _, ok := supportedOutputStreamTypes[typ]; !ok {
				return nil, errors.Wrapf(ErrStreamTypeNotSupported, "Type: '%s'", v)
			}
			outputTypes = append(outputTypes, typ)
		}

		tmp.Streams.Streams[validUUID] = NewStreamConfiguration(rtspStream.URL, outputTypes)
		if rtspStream.Archive.Enabled {
			dir := rtspStream.Archive.Directory
			msPerSegment := rtspStream.Archive.MsPerSegment
			if dir == "" {
				dir = cfg.ArchiveCfg.Directory
			}
			if msPerSegment == 0 {
				msPerSegment = cfg.ArchiveCfg.MsPerSegment
			}
			tmp.SetStreamArchive(validUUID, dir, msPerSegment)
		}
		tmp.Streams.Streams[validUUID].verboseLevel = NewVerboseLevelFrom(rtspStream.Verbose)
	}
	return &tmp, nil
}

func (app *Application) setCors(cfg configuration.CORSConfiguration) {
	newCors := cors.DefaultConfig()
	app.CorsConfig = &newCors
	app.CorsConfig.AllowOrigins = cfg.AllowOrigins
	if len(cfg.AllowMethods) != 0 {
		app.CorsConfig.AllowMethods = cfg.AllowMethods
	}
	if len(cfg.AllowHeaders) != 0 {
		app.CorsConfig.AllowHeaders = cfg.AllowHeaders
	}
	app.CorsConfig.ExposeHeaders = cfg.ExposeHeaders
	app.CorsConfig.AllowCredentials = cfg.AllowCredentials
}

func (app *Application) cast(streamID uuid.UUID, pck av.Packet, hlsEnabled bool) error {
	return app.Streams.cast(streamID, pck, hlsEnabled)
}

func (app *Application) streamExists(streamID uuid.UUID) bool {
	return app.Streams.streamExists(streamID)
}

func (app *Application) existsWithType(streamID uuid.UUID, streamType StreamType) bool {
	return app.Streams.existsWithType(streamID, streamType)
}

func (app *Application) addCodec(streamID uuid.UUID, codecs []av.CodecData) {
	app.Streams.addCodec(streamID, codecs)
}

func (app *Application) getCodec(streamID uuid.UUID) ([]av.CodecData, error) {
	return app.Streams.getCodec(streamID)
}

func (app *Application) updateStreamStatus(streamID uuid.UUID, status bool) error {
	return app.Streams.updateStreamStatus(streamID, status)
}

func (app *Application) addClient(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	return app.Streams.addClient(streamID)
}

func (app *Application) clientDelete(streamID, clientID uuid.UUID) {
	app.Streams.deleteClient(streamID, clientID)
}

func (app *Application) startHlsCast(streamID uuid.UUID, stopCast chan bool) {
	defer app.Streams.Unlock()
	app.Streams.Lock()
	go app.startHls(streamID, app.Streams.Streams[streamID].hlsChanel, stopCast)
}

func (app *Application) startMP4Cast(streamID uuid.UUID, stopCast chan bool) {
	defer app.Streams.Unlock()
	app.Streams.Lock()
	go app.startMP4(streamID, app.Streams.Streams[streamID].mp4Chanel, stopCast)
}

func (app *Application) getStreamsIDs() []uuid.UUID {
	return app.Streams.getKeys()
}

func (app *Application) SetStreamArchive(streamID uuid.UUID, dir string, msPerSegment int64) error {
	return app.Streams.setArchiveStream(streamID, dir, msPerSegment)
}

func (app *Application) getStreamArchive(streamID uuid.UUID) *streamArhive {
	return app.Streams.getArchiveStream(streamID)
}
