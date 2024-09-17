package videoserver

import "github.com/LdDl/video-server/storage"

type StreamArchiveWrapper struct {
	store         storage.ArchiveStorage
	filesystemDir string
	bucket        string
	bucketPath    string
	msPerSegment  int64
}
