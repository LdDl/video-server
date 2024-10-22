package configuration

import (
	"github.com/BurntSushi/toml"
)

func PrepareConfigurationTOML(fname string) (*Configuration, error) {
	cfg := Configuration{}

	var confToml map[string]map[string]any
	_, err := toml.DecodeFile(fname, &confToml)
	if err != nil {
		return nil, err
	}

	if confToml["api"] != nil {
		cfg.APICfg = APIConfiguration{}
		for k, v := range confToml["api"] {
			switch k {
			case "enabled":
				cfg.APICfg.Enabled = v.(bool)
			case "host":
				cfg.APICfg.Host = v.(string)
			case "port":
				cfg.APICfg.Port = int32(v.(int64))
			case "mode":
				cfg.APICfg.Mode = v.(string)
			case "verbose":
				cfg.APICfg.Verbose = v.(string)
			default:
				continue
			}
		}
	}

	if confToml["video"] != nil {
		cfg.VideoServerCfg = VideoConfiguration{}
		for k, v := range confToml["video"] {
			switch k {
			case "host":
				cfg.VideoServerCfg.Host = v.(string)
			case "port":
				cfg.VideoServerCfg.Port = int32(v.(int64))
			case "mode":
				cfg.VideoServerCfg.Mode = v.(string)
			case "verbose":
				cfg.VideoServerCfg.Verbose = v.(string)
			default:
				continue
			}
		}
	}

	if confToml["hls"] != nil {
		cfg.HLSCfg = HLSConfiguration{}
		for k, v := range confToml["hls"] {
			switch k {
			case "ms_per_segment":
				cfg.HLSCfg.MsPerSegment = v.(int64)
			case "directory":
				cfg.HLSCfg.Directory = v.(string)
			case "window_size":
				cfg.HLSCfg.WindowSize = uint(v.(int64))
			case "window_capacity":
				cfg.HLSCfg.Capacity = uint(v.(int64))
			default:
				continue
			}
		}
	}

	if confToml["archive"] != nil {
		cfg.ArchiveCfg = ArchiveConfiguration{}
		for k, v := range confToml["archive"] {
			switch k {
			case "enabled":
				cfg.ArchiveCfg.Enabled = v.(bool)
			case "directory":
				cfg.ArchiveCfg.Directory = v.(string)
			case "ms_per_file":
				cfg.ArchiveCfg.MsPerSegment = v.(int64)
			case "minio_settings":
				cfg.ArchiveCfg.Minio = MinioSettings{}
				settings := v.(map[string]any)
				for sk, sv := range settings {
					switch sk {
					case "host":
						cfg.ArchiveCfg.Minio.Host = sv.(string)
					case "port":
						cfg.ArchiveCfg.Minio.Port = int32(sv.(int64))
					case "user":
						cfg.ArchiveCfg.Minio.User = sv.(string)
					case "password":
						cfg.ArchiveCfg.Minio.Password = sv.(string)
					case "default_bucket":
						cfg.ArchiveCfg.Minio.DefaultBucket = sv.(string)
					case "default_path":
						cfg.ArchiveCfg.Minio.DefaultPath = sv.(string)
					default:
						continue
					}
				}
			default:
				continue
			}
		}
	}

	if confToml["cors"] != nil {
		cfg.CorsConfig = CORSConfiguration{}
		for k, v := range confToml["cors"] {
			switch k {
			case "enabled":
				cfg.CorsConfig.Enabled = v.(bool)
			case "allow_origins":
				allowOrigins := make([]string, len(v.([]interface{})))
				for i, allowOrigin := range v.([]interface{}) {
					allowOrigins[i] = allowOrigin.(string)
				}
				cfg.CorsConfig.AllowOrigins = allowOrigins
			case "allow_methods":
				allowMethods := make([]string, len(v.([]interface{})))
				for i, allowMethod := range v.([]interface{}) {
					allowMethods[i] = allowMethod.(string)
				}
				cfg.CorsConfig.AllowMethods = allowMethods
			case "allow_headers":
				allowHeaders := make([]string, len(v.([]interface{})))
				for i, allowHeader := range v.([]interface{}) {
					allowHeaders[i] = allowHeader.(string)
				}
				cfg.CorsConfig.AllowHeaders = allowHeaders
			case "expose_headers":
				exposeHeaders := make([]string, len(v.([]interface{})))
				for i, exposeHeader := range v.([]interface{}) {
					exposeHeaders[i] = exposeHeader.(string)
				}
				cfg.CorsConfig.ExposeHeaders = exposeHeaders
			default:
				continue
			}
		}
	}

	if confToml["rtsp_streams"] != nil {
		cfg.RTSPStreams = make([]SingleStreamConfiguration, 0, len(confToml["rtsp_streams"]))
		for _, v := range confToml["rtsp_streams"] {
			singleStream := SingleStreamConfiguration{}
			stream := v.(map[string]any)
			for sk, sv := range stream {
				switch sk {
				case "guid":
					singleStream.GUID = sv.(string)
				case "type":
					singleStream.Type = sv.(string)
				case "url":
					singleStream.URL = sv.(string)
				case "output_types":
					types := make([]string, len(sv.([]interface{})))
					for i, stype := range sv.([]interface{}) {
						types[i] = stype.(string)
					}
					singleStream.OutputTypes = types
				case "verbose":
					singleStream.Verbose = sv.(string)
				case "archive":
					archive := sv.(map[string]any)
					for ak, av := range archive {
						switch ak {
						case "enabled":
							singleStream.Archive.Enabled = av.(bool)
						case "ms_per_file":
							singleStream.Archive.MsPerSegment = av.(int64)
						case "type":
							singleStream.Archive.TypeArchive = av.(string)
						case "directory":
							singleStream.Archive.Directory = av.(string)
						case "minio_bucket":
							singleStream.Archive.MinioBucket = av.(string)
						case "minio_path":
							singleStream.Archive.MinioPath = av.(string)
						default:
							continue
						}
					}
				default:
					continue
				}
			}
			// Default common settings for archive
			if singleStream.Archive.MsPerSegment <= 0 {
				if cfg.ArchiveCfg.MsPerSegment > 0 {
					singleStream.Archive.MsPerSegment = cfg.ArchiveCfg.MsPerSegment
				} else {
					singleStream.Archive.MsPerSegment = 30
				}
			}

			// Default filesystem settigs
			if singleStream.Archive.Directory == "" {
				if cfg.ArchiveCfg.Directory != "" {
					singleStream.Archive.Directory = cfg.ArchiveCfg.Directory
				} else {
					singleStream.Archive.Directory = "./mp4"
				}
			}

			// Default minio settings
			if singleStream.Archive.MinioBucket == "" {
				singleStream.Archive.MinioBucket = cfg.ArchiveCfg.Minio.DefaultBucket
			}
			if singleStream.Archive.MinioPath == "" {
				singleStream.Archive.MinioPath = cfg.ArchiveCfg.Minio.DefaultPath
			}
			cfg.RTSPStreams = append(cfg.RTSPStreams, singleStream)
		}
	}

	return &cfg, nil
}
