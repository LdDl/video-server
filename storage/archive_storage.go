package storage

import (
	"context"
)

type ArchiveUnit struct {
	Bucket      string
	SegmentName string
	FileName    string
}

type ArchiveStorage interface {
	Type() StorageType
	MakeBucket(string) error
	UploadFile(context.Context, ArchiveUnit) (string, error)
}
