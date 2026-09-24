package configuration

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5

	defaultOnDemandIdleMs          = 30000
	defaultOnDemandHealthInterval  = 30000
	defaultOnDemandHealthTimeoutMs = 3000
)

func postProcessDefaults(cfg *Configuration) {
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
	if cfg.OnDemandCfg.IdleMs <= 0 {
		cfg.OnDemandCfg.IdleMs = defaultOnDemandIdleMs
	}
	if cfg.OnDemandCfg.HealthIntervalMs <= 0 {
		cfg.OnDemandCfg.HealthIntervalMs = defaultOnDemandHealthInterval
	}
	if cfg.OnDemandCfg.HealthTimeoutMs <= 0 {
		cfg.OnDemandCfg.HealthTimeoutMs = defaultOnDemandHealthTimeoutMs
	}
	for i := range cfg.RTSPStreams {
		stream := cfg.RTSPStreams[i]
		archiveCfg := stream.Archive

		// Skip defaults if recording not enabled for this stream
		if !archiveCfg.Recording {
			continue
		}

		// Default common settings for archive
		if archiveCfg.MsPerSegment <= 0 {
			if cfg.ArchiveCfg.MsPerSegment > 0 {
				cfg.RTSPStreams[i].Archive.MsPerSegment = cfg.ArchiveCfg.MsPerSegment
			} else {
				cfg.RTSPStreams[i].Archive.MsPerSegment = 30
			}
		}

		// Default filesystem settigs
		if archiveCfg.Directory == "" {
			if cfg.ArchiveCfg.Directory != "" {
				cfg.RTSPStreams[i].Archive.Directory = cfg.ArchiveCfg.Directory
			} else {
				cfg.RTSPStreams[i].Archive.Directory = "./mp4"
			}
		}

		// Default minio settings
		if archiveCfg.MinioBucket == "" {
			cfg.RTSPStreams[i].Archive.MinioBucket = cfg.ArchiveCfg.Minio.DefaultBucket
		}
		if archiveCfg.MinioPath == "" {
			cfg.RTSPStreams[i].Archive.MinioPath = cfg.ArchiveCfg.Minio.DefaultPath
		}
	}

}
