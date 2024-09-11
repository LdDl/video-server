package storage

import (
	"context"
	"fmt"
	"os"
)

var ErrNotImplementedYet = fmt.Errorf("not implemented yet")

type FileSystemProvider struct {
	Path string
}

func NewFileSystemProvider(path string) (ArchiveStorage, error) {
	return &FileSystemProvider{
		Path: path,
	}, nil
}

func (storage *FileSystemProvider) Type() StorageType {
	return STORAGE_FILESYSTEM
}

func (storage *FileSystemProvider) MakeBucket(bucket string) error {
	return os.MkdirAll(bucket, os.ModePerm)
}

func (storage *FileSystemProvider) UploadFile(ctx context.Context, object ArchiveUnit) (string, error) {
	return "", ErrNotImplementedYet
}
