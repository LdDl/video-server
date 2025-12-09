package videoserver

import (
	"io"
	"os"
	"testing"
	"time"
)

// TestMP4DemuxerOpen tests that the demuxer can open a valid MP4 file
func TestMP4DemuxerOpen(t *testing.T) {
	testFile := os.Getenv("TEST_MP4_FILE")
	if testFile == "" {
		testFile = "./cmd/video_server/BigBuckBunny_320x180.mp4"
	}

	// Skip if test file doesn't exist
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test MP4 file not found: %s (set TEST_MP4_FILE env var)", testFile)
	}

	demuxer, err := NewMP4Demuxer(testFile)
	if err != nil {
		t.Fatalf("Failed to open MP4 file: %v", err)
	}
	defer demuxer.Close()

	// Check that we have streams
	streams := demuxer.Streams()
	if len(streams) == 0 {
		t.Fatal("No streams found in MP4 file")
	}

	t.Logf("Found %d streams", len(streams))
	for i, s := range streams {
		t.Logf("  Stream %d: type=%v", i, s.Type())
	}
}

// TestMP4DemuxerReadPackets tests that packets can be read from the file
func TestMP4DemuxerReadPackets(t *testing.T) {
	testFile := os.Getenv("TEST_MP4_FILE")
	if testFile == "" {
		testFile = "./cmd/video_server/BigBuckBunny_320x180.mp4"
	}

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test MP4 file not found: %s", testFile)
	}

	demuxer, err := NewMP4Demuxer(testFile)
	if err != nil {
		t.Fatalf("Failed to open MP4 file: %v", err)
	}
	defer demuxer.Close()

	// Read first 100 packets
	packetCount := 0
	keyframeCount := 0
	var lastTime time.Duration
	var firstKeyframeTime time.Duration
	gotFirstKeyframe := false

	for i := 0; i < 100; i++ {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", i, err)
		}

		packetCount++
		if pkt.IsKeyFrame {
			keyframeCount++
			if !gotFirstKeyframe {
				firstKeyframeTime = pkt.Time
				gotFirstKeyframe = true
			}
		}

		// Timestamps should be monotonically increasing (for same track)
		if pkt.Time < lastTime {
			// This is okay if it's a different track (audio vs video interleaved)
			// Log it but don't fail
			t.Logf("Non-monotonic timestamp at packet %d: %v < %v (may be different track)", i, pkt.Time, lastTime)
		}
		lastTime = pkt.Time
	}

	t.Logf("Read %d packets, %d keyframes", packetCount, keyframeCount)
	t.Logf("First keyframe at %v", firstKeyframeTime)

	if packetCount == 0 {
		t.Fatal("No packets read from file")
	}
	if keyframeCount == 0 {
		t.Fatal("No keyframes found in file")
	}
}

// TestMP4DemuxerSeekToStart tests the loop/seek functionality
func TestMP4DemuxerSeekToStart(t *testing.T) {
	testFile := os.Getenv("TEST_MP4_FILE")
	if testFile == "" {
		testFile = "./cmd/video_server/BigBuckBunny_320x180.mp4"
	}

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test MP4 file not found: %s", testFile)
	}

	demuxer, err := NewMP4Demuxer(testFile)
	if err != nil {
		t.Fatalf("Failed to open MP4 file: %v", err)
	}
	defer demuxer.Close()

	// Read first 10 packets and store their data
	firstPass := make([][]byte, 0, 10)
	for i := 0; i < 10; i++ {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet %d: %v", i, err)
		}
		// Store copy of data
		dataCopy := make([]byte, len(pkt.Data))
		copy(dataCopy, pkt.Data)
		firstPass = append(firstPass, dataCopy)
	}

	// Seek to start
	demuxer.SeekToStart()

	// Read again and compare
	for i := 0; i < len(firstPass); i++ {
		pkt, err := demuxer.ReadPacket()
		if err != nil {
			t.Fatalf("Failed to read packet %d after seek: %v", i, err)
		}

		if len(pkt.Data) != len(firstPass[i]) {
			t.Errorf("Packet %d size mismatch after seek: got %d, want %d", i, len(pkt.Data), len(firstPass[i]))
		}

		// Compare first few bytes as a sanity check
		compareLen := min(len(pkt.Data), len(firstPass[i]), 16)
		for j := 0; j < compareLen; j++ {
			if pkt.Data[j] != firstPass[i][j] {
				t.Errorf("Packet %d data mismatch at byte %d after seek", i, j)
				break
			}
		}
	}

	t.Log("SeekToStart works correctly")
}

// TestLoopTimestampContinuity simulates looping and verifies timestamps are continuous
func TestLoopTimestampContinuity(t *testing.T) {
	testFile := os.Getenv("TEST_MP4_FILE")
	if testFile == "" {
		testFile = "./cmd/video_server/BigBuckBunny_320x180.mp4"
	}

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test MP4 file not found: %s", testFile)
	}

	demuxer, err := NewMP4Demuxer(testFile)
	if err != nil {
		t.Fatalf("Failed to open MP4 file: %v", err)
	}
	defer demuxer.Close()

	// Simulate what stream_file.go does
	var timeOffset time.Duration
	var lastAdjustedTime time.Duration
	var allTimestamps []time.Duration

	for loop := 0; loop < 2; loop++ {
		demuxer.SeekToStart()

		// Read first 20 packets per loop
		for i := 0; i < 20; i++ {
			pkt, err := demuxer.ReadPacket()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Loop %d, packet %d: failed to read: %v", loop, i, err)
			}

			// Apply offset like stream_file.go does
			adjustedTime := pkt.Time + timeOffset
			allTimestamps = append(allTimestamps, adjustedTime)
			lastAdjustedTime = adjustedTime
		}

		// Update offset for next loop (like stream_file.go)
		timeOffset = lastAdjustedTime + 100*time.Millisecond
		t.Logf("Loop %d complete, next offset: %v", loop, timeOffset)
	}

	// Verify timestamps are monotonically increasing
	for i := 1; i < len(allTimestamps); i++ {
		if allTimestamps[i] < allTimestamps[i-1] {
			t.Errorf("Timestamp discontinuity at index %d: %v < %v", i, allTimestamps[i], allTimestamps[i-1])
		}
	}

	t.Logf("Total timestamps: %d, range: %v to %v", len(allTimestamps), allTimestamps[0], allTimestamps[len(allTimestamps)-1])
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
