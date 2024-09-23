package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

type MinioProvider struct {
	client *minio.Client

	DefaultBucket string
	Path          string
}

func NewMinioProvider(client *minio.Client, bucket, path string) (ArchiveStorage, error) {
	return &MinioProvider{
		client:        client,
		DefaultBucket: bucket,
		Path:          path,
	}, nil
}

func (m *MinioProvider) Type() StorageType {
	return STORAGE_MINIO
}

func (m *MinioProvider) MakeBucket(bucket string) error {
	_ = m.client.MakeBucket(context.Background(),
		bucket,
		minio.MakeBucketOptions{
			ObjectLocking: true,
		})
	config := lifecycle.NewConfiguration()
	config.Rules = []lifecycle.Rule{
		{
			ID:     "expire-bucket",
			Status: "Enabled",
			Expiration: lifecycle.Expiration{
				Days: 2,
			},
		},
	}

	_ = m.client.SetBucketLifecycle(context.Background(), bucket, config)
	return nil
}

// UploadFile loads file to MinIO. Do not provide FileName field in ArchiveUnit object if you want to use Payload bytes; otherwise file will be loaded from filesystem by FileName field
func (m *MinioProvider) UploadFile(ctx context.Context, object ArchiveUnit) (string, error) {
	fname := fmt.Sprintf("%s/%s", m.Path, object.SegmentName)
	bucket := m.DefaultBucket
	if object.Bucket != "" {
		bucket = object.Bucket
	}
	if object.FileName == "" {
		buf := &bytes.Buffer{}
		size, err := io.Copy(buf, object.Payload)
		if err != nil {
			return "", err
		}
		_, err = m.client.PutObject(
			ctx,
			bucket,
			fname,
			buf,
			size,
			minio.PutObjectOptions{
				ContentType: "application/octet-stream",
			},
		)
		return object.SegmentName, err
	}
	_, err := m.client.FPutObject(
		ctx,
		bucket,
		fname,
		object.FileName,
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		},
	)
	return object.SegmentName, err
}
