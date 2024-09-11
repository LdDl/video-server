package videoserver

import (
	"fmt"

	"github.com/LdDl/video-server/configuration"
	"github.com/LdDl/video-server/storage"
	"github.com/gin-contrib/cors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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
	minioClient    *minio.Client
}

// APIConfiguration is just copy of configuration.APIConfiguration but with some not exported fields
type APIConfiguration struct {
	Enabled bool         `json:"-"`
	Host    string       `json:"host"`
	Port    int32        `json:"port"`
	Mode    string       `json:"-"`
	Verbose VerboseLevel `json:"-"`
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
			Verbose: NewVerboseLevelFrom(cfg.APICfg.Verbose),
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
	minioEnabled := false
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
		tmp.Streams.Streams[validUUID].verboseLevel = NewVerboseLevelFrom(rtspStream.Verbose)
		if rtspStream.Archive.Enabled && cfg.ArchiveCfg.Enabled {
			if rtspStream.Archive.MsPerSegment == 0 {
				return nil, fmt.Errorf("bad ms per segment archive stream")
			}
			storageType := storage.NewStorageTypeFrom(rtspStream.Archive.TypeArchive)
			var archiveStorage streamArhive
			switch storageType {
			case storage.STORAGE_FILESYSTEM:
				fsStorage, err := storage.NewFileSystemProvider(rtspStream.Archive.Directory)
				if err != nil {
					return nil, errors.Wrap(err, "Can't create filesystem provider")
				}
				archiveStorage = streamArhive{
					store:        fsStorage,
					dir:          rtspStream.Archive.Directory,
					bucket:       rtspStream.Archive.Directory,
					bucketPath:   rtspStream.Archive.Directory,
					msPerSegment: rtspStream.Archive.MsPerSegment,
				}
			case storage.STORAGE_MINIO:
				if !minioEnabled {
					client, err := minio.New(fmt.Sprintf("%s:%d", cfg.ArchiveCfg.Minio.Host, cfg.ArchiveCfg.Minio.Port), &minio.Options{
						Creds:  credentials.NewStaticV4(cfg.ArchiveCfg.Minio.User, cfg.ArchiveCfg.Minio.Password, ""),
						Secure: false,
					})
					if err != nil {
						return nil, errors.Wrap(err, "Can't connect to MinIO instance")
					}
					tmp.minioClient = client
					minioEnabled = true
				}
				minioStorage, err := storage.NewMinioProvider(tmp.minioClient, rtspStream.Archive.MinioBucket, rtspStream.Archive.MinioPath)
				if err != nil {
					return nil, errors.Wrap(err, "Can't create MinIO provider")
				}
				archiveStorage = streamArhive{
					store:        minioStorage,
					dir:          rtspStream.Archive.Directory,
					bucket:       rtspStream.Archive.MinioBucket,
					bucketPath:   rtspStream.Archive.MinioPath,
					msPerSegment: rtspStream.Archive.MsPerSegment,
				}
			default:
				return nil, fmt.Errorf("unsupported archive type")
			}
			err = tmp.SetStreamArchive(validUUID, &archiveStorage)
			if err != nil {
				return nil, errors.Wrap(err, "can't set archive for given stream")
			}
		}
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

func (app *Application) cast(streamID uuid.UUID, pck av.Packet, hlsEnabled, archiveEnabled bool) error {
	return app.Streams.cast(streamID, pck, hlsEnabled, archiveEnabled)
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

func (app *Application) startHlsCast(streamID uuid.UUID, stopCast chan bool) error {
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.Streams[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	go app.startHls(streamID, stream.hlsChanel, stopCast)
	return nil
}

func (app *Application) startMP4Cast(streamID uuid.UUID, stopCast chan bool) error {
	app.Streams.Lock()
	defer app.Streams.Unlock()
	stream, ok := app.Streams.Streams[streamID]
	if !ok {
		return ErrStreamNotFound
	}
	go app.startMP4(streamID, stream.mp4Chanel, stopCast)
	return nil
}

func (app *Application) getStreamsIDs() []uuid.UUID {
	return app.Streams.getKeys()
}

func (app *Application) SetStreamArchive(streamID uuid.UUID, archiveStorage *streamArhive) error {
	return app.Streams.setArchiveStream(streamID, archiveStorage)
}

func (app *Application) getStreamArchive(streamID uuid.UUID) *streamArhive {
	return app.Streams.getArchiveStream(streamID)
}
