package configuration

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5
)

func postProcessDefaults(cfg *Configuration) {
	// Backwards compatibility: if Enabled is true but Recording/Serving not set, enable both
	if cfg.ArchiveCfg.Enabled {
		if !cfg.ArchiveCfg.Recording {
			cfg.ArchiveCfg.Recording = true
		}
		if !cfg.ArchiveCfg.Serving {
			cfg.ArchiveCfg.Serving = true
		}
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

		// Backwards compatibility: if Enabled is true but Recording not set, enable Recording
		if archiveCfg.Enabled && !archiveCfg.Recording {
			cfg.RTSPStreams[i].Archive.Recording = true
		}

		// Skip defaults if recording not enabled for this stream
		if !cfg.RTSPStreams[i].Archive.Recording {
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

	// Process local files archive config
	for i := range cfg.LocalFiles {
		archiveCfg := cfg.LocalFiles[i].Archive

		// Backwards compatibility: if Enabled is true but Recording not set, enable Recording
		if archiveCfg.Enabled && !archiveCfg.Recording {
			cfg.LocalFiles[i].Archive.Recording = true
		}
	}
}
