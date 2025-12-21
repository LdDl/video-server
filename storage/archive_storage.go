package storage

import (
	"context"
	"time"
)

type ArchiveUnit struct {
	Bucket      string
	SegmentName string
	FileName    string
}

// ArchiveSegmentInfo contains metadata about an archive segment
type ArchiveSegmentInfo struct {
	SegmentName string
	StartTime   time.Time
	FilePath    string
	Size        int64
}

type ArchiveStorage interface {
	Type() StorageType
	MakeBucket(string) error
	UploadFile(context.Context, ArchiveUnit) (string, error)
	// ListFiles returns segments in the given time range for a stream
	ListFiles(ctx context.Context, directory string, streamPrefix string, startTime, endTime time.Time) ([]ArchiveSegmentInfo, error)
	// GetFilePath returns the full path to serve a segment
	GetFilePath(ctx context.Context, directory, segmentName string) (string, error)
}
