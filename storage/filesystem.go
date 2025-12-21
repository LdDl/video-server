package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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

type candidate struct {
	entry     os.DirEntry
	name      string
	timestamp int64
}

// ListFiles lists archive segments in the directory matching the stream prefix and time range
// Segment filenames are expected in format: {streamID}_{unixTimestamp}.mp4
// It finds the segment containing startTime (largest timestamp <= startTime) and includes
// all segments from there up to endTime.
func (storage *FileSystemProvider) ListFiles(ctx context.Context, directory string, streamPrefix string, startTime, endTime time.Time) ([]ArchiveSegmentInfo, error) {
	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", directory, err)
	}

	// First pass: collect all matching segments and find the one containing start time
	var candidates []candidate
	var containingTimestamp int64 = -1

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".mp4") {
			continue
		}

		// Check if filename starts with stream prefix
		if streamPrefix != "" && !strings.HasPrefix(name, streamPrefix) {
			continue
		}

		// Parse timestamp from filename: {streamID}_{unixTimestamp}.mp4
		baseName := strings.TrimSuffix(name, ".mp4")
		parts := strings.Split(baseName, "_")
		if len(parts) < 2 {
			continue
		}

		timestampStr := parts[len(parts)-1]
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			continue
		}

		candidates = append(candidates, candidate{entry: entry, name: name, timestamp: timestamp})

		// Find largest timestamp that is <= startUnix (segment containing start time)
		if timestamp <= startUnix && timestamp > containingTimestamp {
			containingTimestamp = timestamp
		}
	}

	// Determine effective start: use containing segment if found, otherwise use startUnix
	effectiveStart := startUnix
	if containingTimestamp > 0 {
		effectiveStart = containingTimestamp
	}

	// Second pass: filter segments from effectiveStart to endUnix
	var segments []ArchiveSegmentInfo
	for _, cnd := range candidates {
		if cnd.timestamp < effectiveStart || cnd.timestamp > endUnix {
			continue
		}

		info, err := cnd.entry.Info()
		if err != nil {
			continue
		}

		segments = append(segments, ArchiveSegmentInfo{
			SegmentName: cnd.name,
			StartTime:   time.Unix(cnd.timestamp, 0),
			FilePath:    filepath.Join(directory, cnd.name),
			Size:        info.Size(),
		})
	}

	// Sort by start time
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})

	return segments, nil
}

// GetFilePath returns the full path to a segment file
func (storage *FileSystemProvider) GetFilePath(ctx context.Context, directory, segmentName string) (string, error) {
	fullPath := filepath.Join(directory, segmentName)

	// Verify file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("segment %s not found", segmentName)
		}
		return "", err
	}

	return fullPath, nil
}
