package storage

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

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
	err := m.client.MakeBucket(context.Background(),
		bucket,
		minio.MakeBucketOptions{
			ObjectLocking: true,
		})
	// Ignore "bucket already exists" error
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code != "BucketAlreadyOwnedByYou" && errResp.Code != "BucketAlreadyExists" {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

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

	err = m.client.SetBucketLifecycle(context.Background(), bucket, config)
	if err != nil {
		return fmt.Errorf("failed to set bucket lifecycle: %w", err)
	}
	return nil
}

// UploadFile loads file to MinIO. Do not provide FileName field in ArchiveUnit object if you want to use Payload bytes; otherwise file will be loaded from filesystem by FileName field
func (m *MinioProvider) UploadFile(ctx context.Context, object ArchiveUnit) (string, error) {
	fname := fmt.Sprintf("%s/%s", m.Path, object.SegmentName)
	bucket := m.DefaultBucket
	if object.Bucket != "" {
		bucket = object.Bucket
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

// ListFiles lists archive segments in MinIO matching the stream prefix and time range
func (m *MinioProvider) ListFiles(ctx context.Context, bucket string, streamPrefix string, startTime, endTime time.Time) ([]ArchiveSegmentInfo, error) {
	var segments []ArchiveSegmentInfo

	if bucket == "" {
		bucket = m.DefaultBucket
	}

	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	// List objects with prefix
	prefix := m.Path
	if streamPrefix != "" {
		prefix = fmt.Sprintf("%s/%s", m.Path, streamPrefix)
	}

	objectCh := m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}

		name := object.Key
		if !strings.HasSuffix(name, ".mp4") {
			continue
		}

		// Extract just the filename from the full path
		parts := strings.Split(name, "/")
		fileName := parts[len(parts)-1]

		// Parse timestamp from filename: {streamID}_{unixTimestamp}.mp4
		baseName := strings.TrimSuffix(fileName, ".mp4")
		nameParts := strings.Split(baseName, "_")
		if len(nameParts) < 2 {
			continue
		}

		timestampStr := nameParts[len(nameParts)-1]
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			continue
		}

		// Filter by time range
		if timestamp < startUnix || timestamp > endUnix {
			continue
		}

		segments = append(segments, ArchiveSegmentInfo{
			SegmentName: fileName,
			StartTime:   time.Unix(timestamp, 0),
			FilePath:    name,
			Size:        object.Size,
		})
	}

	// Sort by start time
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})

	return segments, nil
}

// GetFilePath returns a presigned URL for the segment (valid for 1 hour)
func (m *MinioProvider) GetFilePath(ctx context.Context, bucket, segmentName string) (string, error) {
	if bucket == "" {
		bucket = m.DefaultBucket
	}

	objectName := fmt.Sprintf("%s/%s", m.Path, segmentName)

	// Generate presigned URL valid for 1 hour
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, bucket, objectName, time.Hour, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}
