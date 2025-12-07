package videoserver

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
)

// mockWriteSeeker implements io.WriteSeeker for testing with random access
type mockWriteSeeker struct {
	data   []byte
	offset int64
}

func newMockWriteSeeker() *mockWriteSeeker {
	return &mockWriteSeeker{
		data: make([]byte, 0),
	}
}

func (m *mockWriteSeeker) Write(p []byte) (n int, err error) {
	n = len(p)
	endOffset := m.offset + int64(n)

	// Extend buffer if needed
	if endOffset > int64(len(m.data)) {
		newData := make([]byte, endOffset)
		copy(newData, m.data)
		m.data = newData
	}

	// Copy data at current offset
	copy(m.data[m.offset:], p)
	m.offset = endOffset
	return n, nil
}

func (m *mockWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.offset = offset
	case io.SeekCurrent:
		m.offset += offset
	case io.SeekEnd:
		m.offset = int64(len(m.data)) + offset
	}
	if m.offset < 0 {
		m.offset = 0
	}
	return m.offset, nil
}

func (m *mockWriteSeeker) Bytes() []byte {
	return m.data
}

// Mock H264 codec data for testing
type mockH264Codec struct {
	sps    []byte
	pps    []byte
	width  int
	height int
}

func (m mockH264Codec) Type() av.CodecType { return av.H264 }
func (m mockH264Codec) SPS() []byte        { return m.sps }
func (m mockH264Codec) PPS() []byte        { return m.pps }
func (m mockH264Codec) Width() int         { return m.width }
func (m mockH264Codec) Height() int        { return m.height }

// Mock AAC codec data for testing
type mockAACCodec struct {
	config     []byte
	sampleRate int
	channels   int
}

func (m mockAACCodec) Type() av.CodecType              { return av.AAC }
func (m mockAACCodec) MPEG4AudioConfigBytes() []byte   { return m.config }
func (m mockAACCodec) SampleRate() int                 { return m.sampleRate }
func (m mockAACCodec) ChannelLayout() av.ChannelLayout { return av.ChannelLayout(m.channels) }
func (m mockAACCodec) SampleFormat() av.SampleFormat   { return av.S16 }

// Create minimal valid SPS for testing
func createTestSPS() []byte {
	// Minimal SPS NAL unit (simplified for testing)
	// This is a valid-ish SPS that parsers can handle
	return []byte{
		// NAL header: SPS
		0x67,
		// profile_idc, constraint flags, level_idc
		0x64, 0x00, 0x1f,
		0xac, 0xd9, 0x40, 0x44, 0x05, 0xbb, 0x01, 0x10,
		0x00, 0x00, 0x3e, 0x90, 0x00, 0x0b, 0xb8, 0x08,
		0xf1, 0x83, 0x19, 0x60,
	}
}

// Create minimal valid PPS for testing
func createTestPPS() []byte {
	return []byte{
		// NAL header: PPS
		0x68,
		0xeb, 0xe3, 0xcb, 0x22, 0xc0,
	}
}

// Create AAC config for testing (48kHz stereo)
func createTestAACConfig() []byte {
	// AAC-LC, 48kHz, stereo
	return []byte{0x11, 0x90}
}

func TestNewMP4ffMuxer(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	if muxer == nil {
		t.Fatal("NewMP4ffMuxer returned nil")
	}
	if muxer.writer != w {
		t.Error("Writer not set correctly")
	}
	if muxer.initialized {
		t.Error("Muxer should not be initialized yet")
	}
	if len(muxer.tracks) != 0 {
		t.Error("Tracks should be empty initially")
	}
}

func TestWriteHeader_NoCodecs(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	err := muxer.WriteHeader([]av.CodecData{})
	if err == nil {
		t.Error("Expected error for empty codecs")
	}
}

func TestWriteHeader_VideoOnly(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	codec := mockH264Codec{
		sps:    createTestSPS(),
		pps:    createTestPPS(),
		width:  1920,
		height: 1080,
	}

	// Need to use the actual codec type that implements the interface
	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(codec.sps, codec.pps)
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}

	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	if !muxer.initialized {
		t.Error("Muxer should be initialized after WriteHeader")
	}
	if len(muxer.tracks) != 1 {
		t.Errorf("Expected 1 track, got %d", len(muxer.tracks))
	}
	if muxer.videoTrack == nil {
		t.Error("Video track should be set")
	}
	if muxer.audioTrack != nil {
		t.Error("Audio track should not be set")
	}
	if muxer.videoTrack.timescale != 90000 {
		t.Errorf("Video timescale should be 90000, got %d", muxer.videoTrack.timescale)
	}
}

func TestWriteHeader_AudioOnly(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	aacCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(createTestAACConfig())
	if err != nil {
		t.Fatalf("Failed to create AAC codec: %v", err)
	}

	err = muxer.WriteHeader([]av.CodecData{aacCodec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	if len(muxer.tracks) != 1 {
		t.Errorf("Expected 1 track, got %d", len(muxer.tracks))
	}
	if muxer.audioTrack == nil {
		t.Error("Audio track should be set")
	}
	if muxer.videoTrack != nil {
		t.Error("Video track should not be set")
	}
}

func TestWriteHeader_VideoAndAudio(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	aacCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(createTestAACConfig())
	if err != nil {
		t.Fatalf("Failed to create AAC codec: %v", err)
	}

	err = muxer.WriteHeader([]av.CodecData{h264Codec, aacCodec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	if len(muxer.tracks) != 2 {
		t.Errorf("Expected 2 tracks, got %d", len(muxer.tracks))
	}
	if muxer.videoTrack == nil {
		t.Error("Video track should be set")
	}
	if muxer.audioTrack == nil {
		t.Error("Audio track should be set")
	}
	// Check packet indices
	if muxer.videoTrack.pktIdx != 0 {
		t.Errorf("Video pktIdx should be 0, got %d", muxer.videoTrack.pktIdx)
	}
	if muxer.audioTrack.pktIdx != 1 {
		t.Errorf("Audio pktIdx should be 1, got %d", muxer.audioTrack.pktIdx)
	}
}

func TestWritePacket_NotInitialized(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	// Keyframe NAL
	pkt := av.Packet{
		Idx:  0,
		Data: []byte{0x00, 0x00, 0x00, 0x01, 0x65},
		Time: time.Second,
	}

	err := muxer.WritePacket(pkt)
	if err == nil {
		t.Error("Expected error when writing to uninitialized muxer")
	}
}

func TestWritePacket_VideoPacket(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Create a video packet with AVCC format data
	// IDR slice
	nalData := []byte{0x65, 0x88, 0x84, 0x00, 0x33}
	avccData := make([]byte, 4+len(nalData))
	binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
	copy(avccData[4:], nalData)

	pkt := av.Packet{
		Idx:        0,
		Data:       avccData,
		Time:       time.Second,
		IsKeyFrame: true,
	}

	err = muxer.WritePacket(pkt)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}

	if len(muxer.videoTrack.samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(muxer.videoTrack.samples))
	}

	sample := muxer.videoTrack.samples[0]
	if !sample.isKeyFrame {
		t.Error("Sample should be marked as keyframe")
	}
	// 1 second at 90000 timescale = 90000
	if sample.pts != 90000 {
		t.Errorf("Expected PTS 90000, got %d", sample.pts)
	}
}

func TestWritePacket_AudioPacket(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	aacCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(createTestAACConfig())
	if err != nil {
		t.Fatalf("Failed to create AAC codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{aacCodec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// AAC frame data (dummy)
	pkt := av.Packet{
		Idx:  0,
		Data: []byte{0xff, 0xf1, 0x50, 0x80, 0x00, 0x1f, 0xfc},
		Time: time.Millisecond * 500,
	}

	err = muxer.WritePacket(pkt)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}

	if len(muxer.audioTrack.samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(muxer.audioTrack.samples))
	}
}

func TestWritePacket_UnknownTrack(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Packet with unknown index
	pkt := av.Packet{
		Idx:  99,
		Data: []byte{0x00, 0x00, 0x00, 0x01},
		Time: time.Second,
	}

	// Should not error, just skip
	err = muxer.WritePacket(pkt)
	if err != nil {
		t.Errorf("WritePacket should not error for unknown track: %v", err)
	}
}

func TestWritePacket_CompositionTime(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Packet with composition time offset (B-frames scenario)
	nalData := []byte{0x65, 0x88, 0x84}
	avccData := make([]byte, 4+len(nalData))
	binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
	copy(avccData[4:], nalData)

	// PTS = 1s, CTS offset = 50ms
	pkt := av.Packet{
		Idx:             0,
		Data:            avccData,
		Time:            time.Second,
		CompositionTime: time.Millisecond * 50,
		IsKeyFrame:      true,
	}

	err = muxer.WritePacket(pkt)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}

	sample := muxer.videoTrack.samples[0]
	// PTS = 90000 (1s at 90000 timescale)
	// CTS offset = 4500 (50ms at 90000 timescale)
	// DTS = PTS - CTS = 90000 - 4500 = 85500
	if sample.pts != 90000 {
		t.Errorf("Expected PTS 90000, got %d", sample.pts)
	}
	if sample.dts != 85500 {
		t.Errorf("Expected DTS 85500, got %d", sample.dts)
	}
	if sample.compositionTime != 4500 {
		t.Errorf("Expected compositionTime 4500, got %d", sample.compositionTime)
	}
}

func TestWritePacket_DTSUnderflowPrevention(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Packet where composition time is larger than PTS (edge case)
	// This should not cause unsigned underflow
	nalData := []byte{0x65}
	avccData := make([]byte, 4+len(nalData))
	binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
	copy(avccData[4:], nalData)

	// PTS = 10ms = 900 at 90000 timescale
	// CTS = 100ms = 9000 at 90000 timescale
	pkt := av.Packet{
		Idx:             0,
		Data:            avccData,
		Time:            time.Millisecond * 10,
		CompositionTime: time.Millisecond * 100,
		IsKeyFrame:      true,
	}

	err = muxer.WritePacket(pkt)
	if err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}

	sample := muxer.videoTrack.samples[0]
	// DTS should be 0, not a huge number from underflow
	if sample.dts != 0 {
		t.Errorf("Expected DTS 0 (underflow prevention), got %d", sample.dts)
	}
}

func TestWriteTrailer_NotInitialized(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	err := muxer.WriteTrailer()
	if err == nil {
		t.Error("Expected error when writing trailer without initialization")
	}
}

func TestWriteTrailer_CreatesValidMP4(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	aacCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(createTestAACConfig())
	if err != nil {
		t.Fatalf("Failed to create AAC codec: %v", err)
	}

	err = muxer.WriteHeader([]av.CodecData{h264Codec, aacCodec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Write some video packets
	for i := 0; i < 5; i++ {
		nalData := []byte{0x65, 0x88, byte(i)}
		avccData := make([]byte, 4+len(nalData))
		binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
		copy(avccData[4:], nalData)

		pkt := av.Packet{
			Idx:        0,
			Data:       avccData,
			Time:       time.Duration(i) * time.Second / 30,
			IsKeyFrame: i == 0,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket video failed: %v", err)
		}
	}

	// Write some audio packets (~21ms per AAC frame)
	for i := 0; i < 10; i++ {
		pkt := av.Packet{
			Idx:  1,
			Data: []byte{0xff, 0xf1, byte(i)},
			Time: time.Duration(i) * time.Millisecond * 21,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket audio failed: %v", err)
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}

	// Verify the output contains ftyp and moov boxes
	data := w.Bytes()
	if len(data) < 8 {
		t.Fatal("Output too short")
	}

	// Check ftyp box
	ftypSize := binary.BigEndian.Uint32(data[0:4])
	ftypType := string(data[4:8])
	if ftypType != "ftyp" {
		t.Errorf("Expected ftyp box first, got %s", ftypType)
	}
	if ftypSize < 16 {
		t.Errorf("ftyp box too small: %d", ftypSize)
	}

	// Parse the file to verify structure
	reader := bytes.NewReader(data)
	parsedFile, err := mp4.DecodeFile(reader)
	if err != nil {
		t.Fatalf("Failed to parse generated MP4: %v", err)
	}

	if parsedFile.Moov == nil {
		t.Fatal("Moov box not found")
	}

	if len(parsedFile.Moov.Traks) != 2 {
		t.Errorf("Expected 2 tracks, got %d", len(parsedFile.Moov.Traks))
	}
}

func TestWriteTrailer_EditList(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Write packets with composition time (creates non-zero firstSampleDTS)
	for i := 0; i < 3; i++ {
		nalData := []byte{0x65, byte(i)}
		avccData := make([]byte, 4+len(nalData))
		binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
		copy(avccData[4:], nalData)

		pkt := av.Packet{
			Idx:             0,
			Data:            avccData,
			Time:            time.Duration(i+1) * time.Second / 10,
			CompositionTime: time.Millisecond * 50,
			IsKeyFrame:      i == 0,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket failed: %v", err)
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}

	// Parse and verify edit list exists
	reader := bytes.NewReader(w.Bytes())
	parsedFile, err := mp4.DecodeFile(reader)
	if err != nil {
		t.Fatalf("Failed to parse MP4: %v", err)
	}

	trak := parsedFile.Moov.Traks[0]
	if trak.Edts == nil {
		t.Fatal("Edit box (edts) not found")
	}
	if len(trak.Edts.Elst) == 0 {
		t.Fatal("Edit list (elst) not found")
	}

	elst := trak.Edts.Elst[0]
	if len(elst.Entries) != 1 {
		t.Errorf("Expected 1 edit list entry, got %d", len(elst.Entries))
	}
}

func TestWriteTrailer_SyncSamples(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Write packets with specific keyframe pattern
	keyframes := []bool{true, false, false, true, false}
	for i, isKey := range keyframes {
		nalData := []byte{0x65, byte(i)}
		avccData := make([]byte, 4+len(nalData))
		binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
		copy(avccData[4:], nalData)

		pkt := av.Packet{
			Idx:        0,
			Data:       avccData,
			Time:       time.Duration(i) * time.Second / 30,
			IsKeyFrame: isKey,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket failed: %v", err)
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}

	reader := bytes.NewReader(w.Bytes())
	parsedFile, err := mp4.DecodeFile(reader)
	if err != nil {
		t.Fatalf("Failed to parse MP4: %v", err)
	}

	stss := parsedFile.Moov.Traks[0].Mdia.Minf.Stbl.Stss
	if stss == nil {
		t.Fatal("Sync sample box (stss) not found")
	}

	// Keyframes at sample 1 and 4 (1-indexed)
	expectedSyncSamples := []uint32{1, 4}
	if len(stss.SampleNumber) != len(expectedSyncSamples) {
		t.Errorf("Expected %d sync samples, got %d", len(expectedSyncSamples), len(stss.SampleNumber))
	}
	for i, expected := range expectedSyncSamples {
		if i < len(stss.SampleNumber) && stss.SampleNumber[i] != expected {
			t.Errorf("Sync sample %d: expected %d, got %d", i, expected, stss.SampleNumber[i])
		}
	}
}

func TestConvertToAVCC_AlreadyAVCC(t *testing.T) {
	// AVCC format: 4-byte length prefix
	nalData := []byte{0x65, 0x88, 0x84, 0x00}
	avccData := make([]byte, 4+len(nalData))
	binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
	copy(avccData[4:], nalData)

	result := convertToAVCC(avccData)

	// Should return same data (already AVCC)
	if !bytes.Equal(result, avccData) {
		t.Error("AVCC data should pass through unchanged")
	}
}

func TestConvertToAVCC_AnnexB4ByteStartCode(t *testing.T) {
	// Annex-B with 4-byte start code
	annexB := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84}

	result := convertToAVCC(annexB)

	// Should have 4-byte length prefix instead of start code
	if len(result) < 4 {
		t.Fatal("Result too short")
	}
	length := binary.BigEndian.Uint32(result[0:4])
	// NAL data is 3 bytes
	if length != 3 {
		t.Errorf("Expected length 3, got %d", length)
	}
	if result[4] != 0x65 {
		t.Errorf("NAL type byte wrong: %x", result[4])
	}
}

func TestConvertToAVCC_AnnexB3ByteStartCode(t *testing.T) {
	// Annex-B with 3-byte start code
	annexB := []byte{0x00, 0x00, 0x01, 0x65, 0x88}

	result := convertToAVCC(annexB)

	if len(result) < 4 {
		t.Fatal("Result too short")
	}
	length := binary.BigEndian.Uint32(result[0:4])
	// NAL data is 2 bytes
	if length != 2 {
		t.Errorf("Expected length 2, got %d", length)
	}
}

func TestConvertToAVCC_MultipleNALUnits(t *testing.T) {
	// Annex-B with multiple NAL units
	annexB := []byte{
		// SPS
		0x00, 0x00, 0x00, 0x01, 0x67, 0x64, 0x00,
		// PPS
		0x00, 0x00, 0x00, 0x01, 0x68, 0xeb, 0xe3,
		// IDR
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88,
	}

	result := convertToAVCC(annexB)

	// Should have 3 NAL units with length prefixes
	if len(result) < 12 {
		t.Fatalf("Result too short: %d bytes", len(result))
	}

	// First NAL
	len1 := binary.BigEndian.Uint32(result[0:4])
	if result[4] != 0x67 {
		// SPS NAL type
		t.Errorf("First NAL should be SPS (0x67), got 0x%x", result[4])
	}

	// Verify structure has multiple NALs
	offset := 4 + len1
	if offset >= uint32(len(result)) {
		t.Fatal("Not enough data for second NAL")
	}
	len2 := binary.BigEndian.Uint32(result[offset : offset+4])
	if result[offset+4] != 0x68 {
		// PPS NAL type
		t.Errorf("Second NAL should be PPS (0x68), got 0x%x", result[offset+4])
	}

	offset += 4 + len2
	if offset >= uint32(len(result)) {
		t.Fatal("Not enough data for third NAL")
	}
	if result[offset+4] != 0x65 {
		// IDR NAL type
		t.Errorf("Third NAL should be IDR (0x65), got 0x%x", result[offset+4])
	}
}

func TestConvertToAVCC_ShortData(t *testing.T) {
	// Very short data should pass through
	short := []byte{0x01, 0x02}
	result := convertToAVCC(short)

	if !bytes.Equal(result, short) {
		t.Error("Short data should pass through unchanged")
	}
}

func TestConvertToAVCC_RawNAL(t *testing.T) {
	// Raw NAL without start codes or length prefix
	// This happens when data doesn't match any known format
	// Non-IDR slice
	rawNAL := []byte{0x41, 0x9a, 0x24, 0x6c, 0x45}

	result := convertToAVCC(rawNAL)

	// Should wrap with length prefix
	if len(result) != 4+len(rawNAL) {
		t.Errorf("Expected length %d, got %d", 4+len(rawNAL), len(result))
	}
	length := binary.BigEndian.Uint32(result[0:4])
	if length != uint32(len(rawNAL)) {
		t.Errorf("Length prefix wrong: expected %d, got %d", len(rawNAL), length)
	}
}

func TestDuration_NoSamples(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	dur := muxer.Duration()
	if dur != 0 {
		t.Errorf("Expected 0 duration, got %v", dur)
	}
}

func TestDuration_WithSamples(t *testing.T) {
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	err = muxer.WriteHeader([]av.CodecData{h264Codec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Write packets spanning 2 seconds at 30fps
	for i := 0; i < 60; i++ {
		nalData := []byte{0x65, byte(i)}
		avccData := make([]byte, 4+len(nalData))
		binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
		copy(avccData[4:], nalData)

		pkt := av.Packet{
			Idx:        0,
			Data:       avccData,
			Time:       time.Duration(i) * time.Second / 30,
			IsKeyFrame: i%30 == 0,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket failed: %v", err)
		}
	}

	dur := muxer.Duration()
	// Last sample at 59/30 seconds ~ 1.966s
	expectedDur := time.Duration(59) * time.Second / 30
	tolerance := time.Millisecond * 50

	diff := dur - expectedDur
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("Expected duration ~%v, got %v", expectedDur, dur)
	}
}

func TestFullMuxerFlow(t *testing.T) {
	// Integration test: full muxer flow from creation to valid MP4
	w := newMockWriteSeeker()
	muxer := NewMP4ffMuxer(w)

	// Setup codecs
	h264Codec, err := h264parser.NewCodecDataFromSPSAndPPS(createTestSPS(), createTestPPS())
	if err != nil {
		t.Fatalf("Failed to create H264 codec: %v", err)
	}
	aacCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(createTestAACConfig())
	if err != nil {
		t.Fatalf("Failed to create AAC codec: %v", err)
	}

	err = muxer.WriteHeader([]av.CodecData{h264Codec, aacCodec})
	if err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Simulate 1 second of content at 30fps video + audio
	// ~1024 samples per frame at 48kHz ~ 21.3ms
	videoFrames := 30
	audioFrames := 47

	for i := 0; i < videoFrames; i++ {
		// Variable size frames
		nalData := make([]byte, 1000+i*10)
		// IDR for first frame, Non-IDR for rest
		nalData[0] = 0x65
		if i > 0 {
			nalData[0] = 0x41
		}

		avccData := make([]byte, 4+len(nalData))
		binary.BigEndian.PutUint32(avccData, uint32(len(nalData)))
		copy(avccData[4:], nalData)

		// ~1 frame delay for composition time
		pkt := av.Packet{
			Idx:             0,
			Data:            avccData,
			Time:            time.Duration(i) * time.Second / 30,
			CompositionTime: time.Millisecond * 33,
			IsKeyFrame:      i == 0,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket video %d failed: %v", i, err)
		}
	}

	for i := 0; i < audioFrames; i++ {
		// AAC frame
		pkt := av.Packet{
			Idx:  1,
			Data: make([]byte, 200),
			Time: time.Duration(i) * time.Millisecond * 21,
		}
		err = muxer.WritePacket(pkt)
		if err != nil {
			t.Fatalf("WritePacket audio %d failed: %v", i, err)
		}
	}

	err = muxer.WriteTrailer()
	if err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}

	// Verify output is valid MP4
	reader := bytes.NewReader(w.Bytes())
	parsedFile, err := mp4.DecodeFile(reader)
	if err != nil {
		t.Fatalf("Generated MP4 is not valid: %v", err)
	}

	// Check structure
	if parsedFile.Ftyp == nil {
		t.Error("Missing ftyp box")
	}
	if parsedFile.Moov == nil {
		t.Fatal("Missing moov box")
	}
	if len(parsedFile.Moov.Traks) != 2 {
		t.Errorf("Expected 2 tracks, got %d", len(parsedFile.Moov.Traks))
	}

	// Verify video track
	videoTrak := parsedFile.Moov.Traks[0]
	if videoTrak.Mdia.Hdlr.HandlerType != "vide" {
		t.Errorf("First track should be video, got %s", videoTrak.Mdia.Hdlr.HandlerType)
	}
	stbl := videoTrak.Mdia.Minf.Stbl
	if stbl.Stsz.SampleNumber != uint32(videoFrames) {
		t.Errorf("Video track should have %d samples, got %d", videoFrames, stbl.Stsz.SampleNumber)
	}
	if stbl.Stss == nil || len(stbl.Stss.SampleNumber) == 0 {
		t.Error("Video track should have sync sample table")
	}
	if stbl.Ctts == nil {
		t.Error("Video track should have composition time table")
	}

	// Verify audio track
	audioTrak := parsedFile.Moov.Traks[1]
	if audioTrak.Mdia.Hdlr.HandlerType != "soun" {
		t.Errorf("Second track should be audio, got %s", audioTrak.Mdia.Hdlr.HandlerType)
	}
	audioStbl := audioTrak.Mdia.Minf.Stbl
	if audioStbl.Stsz.SampleNumber != uint32(audioFrames) {
		t.Errorf("Audio track should have %d samples, got %d", audioFrames, audioStbl.Stsz.SampleNumber)
	}

	t.Logf("Generated valid MP4: %d bytes, video: %d samples, audio: %d samples",
		len(w.Bytes()), videoFrames, audioFrames)
}
