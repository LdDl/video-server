package videoserver

import "github.com/LdDl/video-server/storage"

type StreamArchiveWrapper struct {
	store        storage.ArchiveStorage
	dir          string
	bucket       string
	bucketPath   string
	msPerSegment int64
}
