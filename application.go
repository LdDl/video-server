package videoserver

import (
	"fmt"

	"github.com/LdDl/video-server/configuration"
	"github.com/LdDl/video-server/storage"
	"github.com/gin-contrib/cors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"

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
	Host    string       `json:"host"`
	Port    int32        `json:"port"`
	Mode    string       `json:"-"`
	Verbose VerboseLevel `json:"-"`
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
			Host:    cfg.VideoServerCfg.Host,
			Port:    cfg.VideoServerCfg.Port,
			Verbose: NewVerboseLevelFrom(cfg.VideoServerCfg.Verbose),
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
	for rs := range cfg.RTSPStreams {
		rtspStream := cfg.RTSPStreams[rs]
		validUUID, err := uuid.Parse(rtspStream.GUID)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_CONFIGURATION).Str("stream_id", rtspStream.GUID).Msg("Not valid UUID")
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

		tmp.Streams.store[validUUID] = NewStreamConfiguration(rtspStream.URL, outputTypes)
		tmp.Streams.store[validUUID].verboseLevel = NewVerboseLevelFrom(rtspStream.Verbose)
		if rtspStream.Archive.Enabled && cfg.ArchiveCfg.Enabled {
			if rtspStream.Archive.MsPerSegment == 0 {
				return nil, fmt.Errorf("bad ms per segment archive stream")
			}
			storageType := storage.NewStorageTypeFrom(rtspStream.Archive.TypeArchive)
			var archiveStorage StreamArchiveWrapper
			switch storageType {
			case storage.STORAGE_FILESYSTEM:
				fsStorage, err := storage.NewFileSystemProvider(rtspStream.Archive.Directory)
				if err != nil {
					return nil, errors.Wrap(err, "Can't create filesystem provider")
				}
				archiveStorage = StreamArchiveWrapper{
					store:         fsStorage,
					filesystemDir: rtspStream.Archive.Directory,
					bucket:        rtspStream.Archive.Directory,
					bucketPath:    rtspStream.Archive.Directory,
					msPerSegment:  rtspStream.Archive.MsPerSegment,
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
				archiveStorage = StreamArchiveWrapper{
					store:         minioStorage,
					filesystemDir: rtspStream.Archive.Directory,
					bucket:        rtspStream.Archive.MinioBucket,
					bucketPath:    rtspStream.Archive.MinioPath,
					msPerSegment:  rtspStream.Archive.MsPerSegment,
				}
			default:
				return nil, fmt.Errorf("unsupported archive type")
			}
			err = tmp.Streams.UpdateArchiveStorageForStream(validUUID, &archiveStorage)
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
	// See https://github.com/gofiber/fiber/security/advisories/GHSA-fmg4-x8pw-hjhg
	if app.CorsConfig.AllowCredentials {
		for _, v := range app.CorsConfig.AllowOrigins {
			if v == "*" {
				log.Warn().Str("scope", SCOPE_APP).Str("event", EVENT_APP_CORS_CONFIG).Msg("[CORS] Insecure setup, 'AllowCredentials' is set to true, and 'AllowOrigins' is set to a wildcard. Settings 'AllowCredentials' to be 'false'. See https://github.com/gofiber/fiber/security/advisories/GHSA-fmg4-x8pw-hjhg")
				app.CorsConfig.AllowCredentials = false
			}
		}
	}
}
