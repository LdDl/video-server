package storage

import (
	"context"
	"io"
)

type ArchiveUnit struct {
	Payload     io.Reader
	Bucket      string
	SegmentName string
}

type ArchiveStorage interface {
	Type() StorageType
	MakeBucket(string) error
	UploadFile(context.Context, ArchiveUnit) (string, error)
}
