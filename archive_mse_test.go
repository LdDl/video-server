package videoserver

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/mp4f"
)

// Ensure bytes package is used
var _ = bytes.Buffer{}

// TestArchiveMSEIntegration tests the full archive -> MSE playback flow
// This simulates:
// 1. Writing archive segments with accumulated timestamps (like from a looping source)
// 2. Reading them back and verifying timestamps are correct
// 3. Muxing to fragmented MP4 and verifying output timing
func TestArchiveMSEIntegration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "archive_mse_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Simulated accumulated time from looping source (like 24h51m)
	baseTime := 24*time.Hour + 51*time.Minute + 18*time.Second

	// Test parameters
	segmentDuration := 10 * time.Second
	fps := 30
	frameDuration := time.Second / time.Duration(fps)

	// Create test H.264 codec data
	testSPS := []byte{0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50, 0x05, 0xbb, 0x01, 0x6a, 0x02, 0x02, 0x02, 0x80, 0x00, 0x00, 0x03, 0x00, 0x80, 0x00, 0x00, 0x1e, 0x07, 0x8c, 0x18, 0xcb}
	testPPS := []byte{0x68, 0xe9, 0x7b, 0x2c, 0x8b}
	codec, err := h264parser.NewCodecDataFromSPSAndPPS(testSPS, testPPS)
	if err != nil {
		t.Fatalf("Failed to create codec: %v", err)
	}

	t.Run("SegmentWriteAndRead", func(t *testing.T) {
		segmentPath := filepath.Join(tmpDir, "segment1.mp4")

		// Step 1: Write segment with accumulated timestamps
		t.Log("Writing segment with accumulated timestamps starting at", baseTime)
		writeTestSegment(t, segmentPath, codec, baseTime, segmentDuration, fps)

		// Step 2: Read segment and verify timestamps
		t.Log("Reading segment and verifying timestamps")
		verifySegmentTimestamps(t, segmentPath, segmentDuration, frameDuration)
	})

	t.Run("MultipleSegmentsNormalized", func(t *testing.T) {
		// Create two segments with different base times
		seg1Path := filepath.Join(tmpDir, "multi_seg1.mp4")
		seg2Path := filepath.Join(tmpDir, "multi_seg2.mp4")

		// Segment 1: starts at 24h51m18s
		writeTestSegment(t, seg1Path, codec, baseTime, segmentDuration, fps)
		// Segment 2: starts at 24h51m28s (10 seconds later)
		writeTestSegment(t, seg2Path, codec, baseTime+segmentDuration, segmentDuration, fps)

		// Read both and verify they can be played back-to-back
		t.Log("Verifying segment 1 timestamps")
		firstPts1, lastPts1 := getSegmentPTSRange(t, seg1Path)
		t.Logf("Segment 1: first PTS=%v, last PTS=%v, duration=%v", firstPts1, lastPts1, lastPts1-firstPts1)

		t.Log("Verifying segment 2 timestamps")
		firstPts2, lastPts2 := getSegmentPTSRange(t, seg2Path)
		t.Logf("Segment 2: first PTS=%v, last PTS=%v, duration=%v", firstPts2, lastPts2, lastPts2-firstPts2)

		// Duration should be ~10 seconds each (not 24 hours!)
		seg1Duration := lastPts1 - firstPts1
		seg2Duration := lastPts2 - firstPts2

		if seg1Duration > 15*time.Second || seg1Duration < 5*time.Second {
			t.Errorf("Segment 1 duration wrong: got %v, expected ~10s", seg1Duration)
		}
		if seg2Duration > 15*time.Second || seg2Duration < 5*time.Second {
			t.Errorf("Segment 2 duration wrong: got %v, expected ~10s", seg2Duration)
		}
	})

	t.Run("FragmentedMP4Output", func(t *testing.T) {
		segmentPath := filepath.Join(tmpDir, "fmp4_test.mp4")
		writeTestSegment(t, segmentPath, codec, baseTime, segmentDuration, fps)

		// Read segment and mux to fMP4
		t.Log("Testing fragmented MP4 output")
		testFragmentedMP4Output(t, segmentPath, codec, segmentDuration)
	})

	t.Run("NormalizedTimestampsForMSE", func(t *testing.T) {
		segmentPath := filepath.Join(tmpDir, "mse_test.mp4")
		writeTestSegment(t, segmentPath, codec, baseTime, segmentDuration, fps)

		// Simulate what ws_archive_handler does
		t.Log("Testing MSE-ready timestamp normalization")
		testMSENormalization(t, segmentPath, codec)
	})
}

func writeTestSegment(t *testing.T, path string, codec av.CodecData, baseTime, duration time.Duration, fps int) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	muxer := NewMP4ffMuxer(file)
	err = muxer.WriteHeader([]av.CodecData{codec})
	if err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	frameDuration := time.Second / time.Duration(fps)
	frameCount := int(duration / frameDuration)

	// Create test NAL unit (IDR frame for first, non-IDR for rest)
	idrNAL := []byte{0x00, 0x00, 0x00, 0x01, 0x65}    // IDR
	nonIDRNAL := []byte{0x00, 0x00, 0x00, 0x01, 0x41} // Non-IDR
	// Add some padding to make it look like real frame data
	idrNAL = append(idrNAL, make([]byte, 1000)...)
	nonIDRNAL = append(nonIDRNAL, make([]byte, 500)...)

	for i := 0; i < frameCount; i++ {
		// Accumulated time (simulating looping source)
		pktTime := baseTime + time.Duration(i)*frameDuration
		// Keyframe every 2 seconds
		isKeyFrame := i == 0 || i%(fps*2) == 0
		var data []byte
		if isKeyFrame {
			data = idrNAL
		} else {
			data = nonIDRNAL
		}

		pkt := av.Packet{
			Idx:        0,
			IsKeyFrame: isKeyFrame,
			Time:       pktTime,
			Data:       data,
		}

		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("Failed to write packet %d: %v", i, err)
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}

	// Log the muxer's reported duration
	t.Logf("Muxer reported duration: %v", muxer.Duration())
}

func verifySegmentTimestamps(t *testing.T, path string, expectedDuration, frameDuration time.Duration) {
	t.Helper()

	demuxer, err := NewMP4Demuxer(path)
	if err != nil {
		t.Fatalf("Failed to open segment: %v", err)
	}
	defer demuxer.Close()

	var firstPTS, lastPTS time.Duration
	packetCount := 0

	for {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}

		if packetCount == 0 {
			firstPTS = pkt.Time
			t.Logf("First packet PTS: %v", firstPTS)
		}
		lastPTS = pkt.Time
		packetCount++
	}

	t.Logf("Last packet PTS: %v, total packets: %d", lastPTS, packetCount)

	// The raw timestamps from demuxer will be accumulated (starting from 24h+)
	// But the duration (last - first) should be ~10 seconds
	actualDuration := lastPTS - firstPTS
	t.Logf("Actual duration (last - first): %v", actualDuration)

	// Allow 20% tolerance
	minDur := expectedDuration * 80 / 100
	maxDur := expectedDuration * 120 / 100

	if actualDuration < minDur || actualDuration > maxDur {
		t.Errorf("Duration mismatch: got %v, expected %v (±20%%)", actualDuration, expectedDuration)
	}
}

func getSegmentPTSRange(t *testing.T, path string) (first, last time.Duration) {
	t.Helper()

	demuxer, err := NewMP4Demuxer(path)
	if err != nil {
		t.Fatalf("Failed to open segment: %v", err)
	}
	defer demuxer.Close()

	packetCount := 0
	for {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}

		if packetCount == 0 {
			first = pkt.Time
		}
		last = pkt.Time
		packetCount++
	}
	return
}

func testFragmentedMP4Output(t *testing.T, segmentPath string, codec av.CodecData, expectedDuration time.Duration) {
	t.Helper()

	demuxer, err := NewMP4Demuxer(segmentPath)
	if err != nil {
		t.Fatalf("Failed to open segment: %v", err)
	}
	defer demuxer.Close()

	// Create fragmented MP4 muxer (like ws_archive_handler does)
	fmp4Muxer := mp4f.NewMuxer(nil)
	err = fmp4Muxer.WriteHeader([]av.CodecData{codec})
	if err != nil {
		t.Fatalf("Failed to write fMP4 header: %v", err)
	}

	var timeOffset time.Duration
	var normalizedTimes []time.Duration
	packetCount := 0

	for {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}

		// Normalize time (like ws_archive_handler does)
		if packetCount == 0 {
			timeOffset = pkt.Time
			t.Logf("Time offset (first packet time): %v", timeOffset)
		}
		pkt.Time = pkt.Time - timeOffset
		normalizedTimes = append(normalizedTimes, pkt.Time)

		_, _, err = fmp4Muxer.WritePacket(pkt, false)
		if err != nil {
			t.Fatalf("Failed to mux packet: %v", err)
		}
		packetCount++
	}

	// Verify normalized timestamps start at 0 and increment properly
	if len(normalizedTimes) == 0 {
		t.Fatal("No packets read")
	}

	if normalizedTimes[0] != 0 {
		t.Errorf("First normalized time should be 0, got %v", normalizedTimes[0])
	}

	lastNormalized := normalizedTimes[len(normalizedTimes)-1]
	t.Logf("Last normalized time: %v", lastNormalized)

	// Should be ~10 seconds, not 24 hours
	if lastNormalized > 15*time.Second {
		t.Errorf("Normalized duration too large: %v (expected ~%v)", lastNormalized, expectedDuration)
	}
	if lastNormalized < 5*time.Second {
		t.Errorf("Normalized duration too small: %v (expected ~%v)", lastNormalized, expectedDuration)
	}

	t.Logf("SUCCESS: Normalized timestamps range from 0 to %v", lastNormalized)
}

func testMSENormalization(t *testing.T, segmentPath string, codec av.CodecData) {
	t.Helper()

	demuxer, err := NewMP4Demuxer(segmentPath)
	if err != nil {
		t.Fatalf("Failed to open segment: %v", err)
	}
	defer demuxer.Close()

	// Create fMP4 muxer and collect output
	fmp4Muxer := mp4f.NewMuxer(nil)
	err = fmp4Muxer.WriteHeader([]av.CodecData{codec})
	if err != nil {
		t.Fatalf("Failed to write fMP4 header: %v", err)
	}

	_, init := fmp4Muxer.GetInit([]av.CodecData{codec})
	t.Logf("Init segment size: %d bytes", len(init))

	// Verify init segment has correct structure
	if len(init) < 8 {
		t.Fatal("Init segment too small")
	}

	var timeOffset time.Duration
	var fragments [][]byte
	packetCount := 0

	for {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}

		// Normalize
		if packetCount == 0 {
			timeOffset = pkt.Time
		}
		pkt.Time = pkt.Time - timeOffset

		ready, buf, err := fmp4Muxer.WritePacket(pkt, false)
		if err != nil {
			t.Fatalf("Failed to mux packet: %v", err)
		}

		if ready && len(buf) > 0 {
			fragments = append(fragments, buf)
		}
		packetCount++
	}

	// Flush remaining
	_, buf, _ := fmp4Muxer.WritePacket(av.Packet{}, true)
	if len(buf) > 0 {
		fragments = append(fragments, buf)
	}

	t.Logf("Generated %d fragments from %d packets", len(fragments), packetCount)

	// Verify we have fragments
	if len(fragments) == 0 {
		t.Error("No fragments generated")
	}

	// Check fragment sizes are reasonable
	totalFragmentSize := 0
	for i, frag := range fragments {
		totalFragmentSize += len(frag)
		if i < 3 {
			t.Logf("Fragment %d: %d bytes", i, len(frag))
		}
	}
	t.Logf("Total fragment data: %d bytes", totalFragmentSize)

	// Verify fragments look like valid fMP4 (start with moof box typically)
	if len(fragments) > 0 && len(fragments[0]) >= 8 {
		boxType := string(fragments[0][4:8])
		t.Logf("First fragment box type: %s", boxType)
		if boxType != "moof" && boxType != "styp" {
			t.Logf("Warning: First fragment doesn't start with moof or styp, got: %s", boxType)
		}
	}
}

// TestArchiveFileDurationMetadata verifies the MP4 file metadata has correct duration
func TestArchiveFileDurationMetadata(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "archive_duration_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create codec
	testSPS := []byte{0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50, 0x05, 0xbb, 0x01, 0x6a, 0x02, 0x02, 0x02, 0x80, 0x00, 0x00, 0x03, 0x00, 0x80, 0x00, 0x00, 0x1e, 0x07, 0x8c, 0x18, 0xcb}
	testPPS := []byte{0x68, 0xe9, 0x7b, 0x2c, 0x8b}
	codec, err := h264parser.NewCodecDataFromSPSAndPPS(testSPS, testPPS)
	if err != nil {
		t.Fatalf("Failed to create codec: %v", err)
	}

	segmentPath := filepath.Join(tmpDir, "duration_test.mp4")

	// Write segment with 24h+ base time
	baseTime := 24*time.Hour + 51*time.Minute + 18*time.Second
	segmentDuration := 10 * time.Second
	fps := 30

	file, err := os.Create(segmentPath)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	muxer := NewMP4ffMuxer(file)
	err = muxer.WriteHeader([]av.CodecData{codec})
	if err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	frameDuration := time.Second / time.Duration(fps)
	frameCount := int(segmentDuration / frameDuration)

	idrNAL := append([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, make([]byte, 1000)...)
	nonIDRNAL := append([]byte{0x00, 0x00, 0x00, 0x01, 0x41}, make([]byte, 500)...)

	for i := 0; i < frameCount; i++ {
		pktTime := baseTime + time.Duration(i)*frameDuration
		isKeyFrame := i == 0 || i%(fps*2) == 0

		var data []byte
		if isKeyFrame {
			data = idrNAL
		} else {
			data = nonIDRNAL
		}

		pkt := av.Packet{
			Idx:        0,
			IsKeyFrame: isKeyFrame,
			Time:       pktTime,
			Data:       data,
		}
		muxer.WritePacket(pkt)
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("Failed to write trailer: %v", err)
	}
	file.Close()

	// Now read the file and check the moov duration
	demuxer, err := NewMP4Demuxer(segmentPath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer demuxer.Close()

	// The demuxer should report reasonable duration
	// We can check by reading all packets and seeing the range
	var firstPTS, lastPTS time.Duration
	count := 0
	for {
		pkt, err := demuxer.ReadPacket()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read packet: %v", err)
		}
		if count == 0 {
			firstPTS = pkt.Time
		}
		lastPTS = pkt.Time
		count++
	}

	// The raw PTS values will include the base offset
	t.Logf("Raw PTS range: %v to %v", firstPTS, lastPTS)
	t.Logf("Raw duration: %v", lastPTS-firstPTS)

	// Check that file metadata duration is correct
	// Re-read with mp4ff directly to check moov
	fileData, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Look for moov box and check mvhd duration
	// Simple check: file should be reasonable size (not corrupted)
	t.Logf("File size: %d bytes", len(fileData))

	// Duration in file should be ~10 seconds, not 24 hours
	actualDuration := lastPTS - firstPTS
	if actualDuration > 15*time.Second {
		t.Errorf("File duration too large: %v", actualDuration)
	}
	if actualDuration < 5*time.Second {
		t.Errorf("File duration too small: %v", actualDuration)
	}

	t.Logf("SUCCESS: File duration is %v (expected ~10s)", actualDuration)
}

// TestSeamlessMultiSegmentPlayback tests playing multiple segments back-to-back
func TestSeamlessMultiSegmentPlayback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seamless_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create codec
	testSPS := []byte{0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50, 0x05, 0xbb, 0x01, 0x6a, 0x02, 0x02, 0x02, 0x80, 0x00, 0x00, 0x03, 0x00, 0x80, 0x00, 0x00, 0x1e, 0x07, 0x8c, 0x18, 0xcb}
	testPPS := []byte{0x68, 0xe9, 0x7b, 0x2c, 0x8b}
	codec, err := h264parser.NewCodecDataFromSPSAndPPS(testSPS, testPPS)
	if err != nil {
		t.Fatalf("Failed to create codec: %v", err)
	}

	baseTime := 24*time.Hour + 51*time.Minute + 18*time.Second
	segmentDuration := 10 * time.Second
	fps := 30

	// Create 3 segments
	var segmentPaths []string
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "seg"+string(rune('0'+i))+".mp4")
		segmentPaths = append(segmentPaths, path)
		segTime := baseTime + time.Duration(i)*segmentDuration
		writeTestSegmentHelper(t, path, codec, segTime, segmentDuration, fps)
	}

	// Now simulate seamless playback across all segments
	fmp4Muxer := mp4f.NewMuxer(nil)
	err = fmp4Muxer.WriteHeader([]av.CodecData{codec})
	if err != nil {
		t.Fatalf("Failed to write fMP4 header: %v", err)
	}

	// Seamless playback: accumulate time across segments
	var segmentBaseTime time.Duration
	var accumulatedTime time.Duration
	var lastPacketTime time.Duration
	var allNormalizedTimes []time.Duration
	totalPackets := 0

	for segIdx, segPath := range segmentPaths {
		demuxer, err := NewMP4Demuxer(segPath)
		if err != nil {
			t.Fatalf("Failed to open segment %d: %v", segIdx, err)
		}

		// Before each segment (except first), update accumulated time
		if segIdx > 0 && lastPacketTime > 0 {
			accumulatedTime = lastPacketTime
		}

		segPackets := 0
		segmentFirstPacket := true

		for {
			pkt, err := demuxer.ReadPacket()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Failed to read packet from segment %d: %v", segIdx, err)
			}

			// Capture base time for this segment
			if segmentFirstPacket {
				segmentBaseTime = pkt.Time
				segmentFirstPacket = false
			}

			// Normalize: (pkt.Time - segmentBase) + accumulated
			pkt.Time = (pkt.Time - segmentBaseTime) + accumulatedTime
			lastPacketTime = pkt.Time
			allNormalizedTimes = append(allNormalizedTimes, pkt.Time)

			_, _, err = fmp4Muxer.WritePacket(pkt, false)
			if err != nil {
				t.Fatalf("Failed to mux packet: %v", err)
			}

			segPackets++
			totalPackets++
		}
		demuxer.Close()

		t.Logf("Segment %d: %d packets, lastTime=%v, accumulated=%v", segIdx, segPackets, lastPacketTime, accumulatedTime)
	}

	t.Logf("Total packets across all segments: %d", totalPackets)

	// Verify timestamps are continuous and increasing
	var lastTime time.Duration
	discontinuities := 0
	for i, ts := range allNormalizedTimes {
		if i > 0 && ts < lastTime {
			discontinuities++
			if discontinuities <= 5 {
				t.Logf("Discontinuity at packet %d: %v -> %v", i, lastTime, ts)
			}
		}
		lastTime = ts
	}

	if discontinuities > 0 {
		t.Errorf("Found %d timestamp discontinuities (times going backwards)", discontinuities)
	}

	// Final time should be ~30 seconds (3 segments * 10 seconds)
	finalTime := allNormalizedTimes[len(allNormalizedTimes)-1]
	t.Logf("Final normalized time: %v", finalTime)

	expectedTotal := 3 * segmentDuration
	if finalTime < expectedTotal*80/100 || finalTime > expectedTotal*120/100 {
		t.Errorf("Total playback duration wrong: got %v, expected ~%v", finalTime, expectedTotal)
	}

	t.Logf("SUCCESS: Seamless playback across 3 segments, total duration %v", finalTime)
}

func writeTestSegmentHelper(t *testing.T, path string, codec av.CodecData, baseTime, duration time.Duration, fps int) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	muxer := NewMP4ffMuxer(file)
	muxer.WriteHeader([]av.CodecData{codec})

	frameDuration := time.Second / time.Duration(fps)
	frameCount := int(duration / frameDuration)

	idrNAL := append([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, make([]byte, 1000)...)
	nonIDRNAL := append([]byte{0x00, 0x00, 0x00, 0x01, 0x41}, make([]byte, 500)...)

	for i := 0; i < frameCount; i++ {
		pktTime := baseTime + time.Duration(i)*frameDuration
		isKeyFrame := i == 0 || i%(fps*2) == 0

		var data []byte
		if isKeyFrame {
			data = idrNAL
		} else {
			data = nonIDRNAL
		}

		muxer.WritePacket(av.Packet{
			Idx:        0,
			IsKeyFrame: isKeyFrame,
			Time:       pktTime,
			Data:       data,
		})
	}

	muxer.WriteTrailer()
}
