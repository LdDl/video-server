package videoserver

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/pkg/errors"
)

// MP4ffMuxer wraps mp4ff library to create MP4 files
type MP4ffMuxer struct {
	writer      io.WriteSeeker
	moov        *mp4.MoovBox
	mdat        *mp4.MdatBox
	tracks      []*mp4ffTrack
	videoTrack  *mp4ffTrack
	audioTrack  *mp4ffTrack
	mdatStart   int64
	initialized bool
}

type mp4ffTrack struct {
	trackID uint32
	// Packet index from codec order
	pktIdx    int8
	timescale uint32
	codec     av.CodecData
	samples   []mp4ffSample
	isVideo   bool
	isAudio   bool
	// H.264 specific
	sps []byte
	pps []byte
	// AAC specific
	aacConfig []byte
}

type mp4ffSample struct {
	// size of data in bytes
	size   uint32
	offset int64 // file offset in mdat
	// In timescale units
	pts      uint64
	dts      uint64
	duration uint32
	// CTS offset in timescale units
	compositionTime int32
	isKeyFrame      bool
}

// NewMP4ffMuxer creates a new muxer using mp4ff library
func NewMP4ffMuxer(w io.WriteSeeker) *MP4ffMuxer {
	return &MP4ffMuxer{
		writer: w,
		tracks: make([]*mp4ffTrack, 0),
	}
}

// WriteHeader initializes the muxer with codec information
func (m *MP4ffMuxer) WriteHeader(codecs []av.CodecData) error {
	if len(codecs) == 0 {
		return errors.New("no codecs provided")
	}

	trackID := uint32(1)
	for idx, codec := range codecs {
		track := &mp4ffTrack{
			trackID: trackID,
			pktIdx:  int8(idx),
			codec:   codec,
			samples: make([]mp4ffSample, 0),
		}

		switch codec.Type() {
		case av.H264:
			h264Codec := codec.(h264parser.CodecData)
			track.isVideo = true
			track.timescale = 90000 // Standard video timescale
			track.sps = h264Codec.SPS()
			track.pps = h264Codec.PPS()
			m.videoTrack = track
		case av.AAC:
			aacCodec := codec.(aacparser.CodecData)
			track.isAudio = true
			track.timescale = uint32(aacCodec.SampleRate())
			track.aacConfig = aacCodec.MPEG4AudioConfigBytes()
			m.audioTrack = track
		default:
			continue
		}

		m.tracks = append(m.tracks, track)
		trackID++
	}

	if len(m.tracks) == 0 {
		return errors.New("no supported tracks found")
	}

	// Write ftyp box
	ftyp := mp4.NewFtyp("isom", 0x200, []string{"isom", "iso2", "avc1", "mp41"})
	err := ftyp.Encode(m.writer)
	if err != nil {
		return errors.Wrap(err, "failed to write ftyp")
	}

	// Reserve space for moov (we'll come back and write it at the end)
	// For now, start mdat
	m.mdatStart, err = m.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrap(err, "failed to get mdat start position")
	}

	// Write mdat header with placeholder size (will update later)
	// Using 64-bit extended size for large files
	m.mdat = &mp4.MdatBox{}
	// Write placeholder: 8 bytes for extended header + actual header
	placeholder := make([]byte, 16)
	// size = 1 means extended size follows
	binary.BigEndian.PutUint32(placeholder[0:4], 1)
	copy(placeholder[4:8], "mdat")
	// Extended size will be filled in WriteTrailer
	_, err = m.writer.Write(placeholder)
	if err != nil {
		return errors.Wrap(err, "failed to write mdat header")
	}

	m.initialized = true
	return nil
}

// WritePacket writes a packet to the muxer
func (m *MP4ffMuxer) WritePacket(pkt av.Packet) error {
	if !m.initialized {
		return errors.New("muxer not initialized")
	}

	// Find the track for this packet by matching packet index
	var track *mp4ffTrack
	for _, t := range m.tracks {
		if t.pktIdx == pkt.Idx {
			track = t
			break
		}
	}

	if track == nil {
		return nil // Skip unknown tracks
	}

	// Convert time to timescale units
	pts := uint64(pkt.Time.Seconds() * float64(track.timescale))
	dts := pts
	var compositionTime int32 = 0

	if pkt.CompositionTime > 0 {
		ctsOffset := uint64(pkt.CompositionTime.Seconds() * float64(track.timescale))
		compositionTime = int32(ctsOffset)
		// Prevent unsigned underflow - dts can't be negative
		if pts >= ctsOffset {
			dts = pts - ctsOffset
		} else {
			dts = 0
		}
	}

	// Process data
	var data []byte
	if track.isVideo {
		// Convert Annex-B to AVCC format if needed
		data = convertToAVCC(pkt.Data)
	} else {
		data = pkt.Data
	}

	// Get current file offset before writing
	offset, err := m.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrap(err, "failed to get file offset")
	}

	// Write data to mdat
	_, err = m.writer.Write(data)
	if err != nil {
		return errors.Wrap(err, "failed to write sample data")
	}

	sample := mp4ffSample{
		size:            uint32(len(data)),
		offset:          offset,
		pts:             pts,
		dts:             dts,
		isKeyFrame:      pkt.IsKeyFrame,
		compositionTime: compositionTime,
	}
	track.samples = append(track.samples, sample)

	return nil
}

// WriteTrailer finalizes the file with moov box
func (m *MP4ffMuxer) WriteTrailer() error {
	if !m.initialized {
		return errors.New("muxer not initialized")
	}

	// Calculate mdat size and update header
	mdatEnd, err := m.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrap(err, "failed to get mdat end position")
	}
	mdatSize := mdatEnd - m.mdatStart

	// Go back and write the correct mdat size
	_, err = m.writer.Seek(m.mdatStart+8, io.SeekStart)
	if err != nil {
		return errors.Wrap(err, "failed to seek to mdat size position")
	}
	extSize := make([]byte, 8)
	binary.BigEndian.PutUint64(extSize, uint64(mdatSize))
	_, err = m.writer.Write(extSize)
	if err != nil {
		return errors.Wrap(err, "failed to write mdat size")
	}

	// Seek back to end
	_, err = m.writer.Seek(mdatEnd, io.SeekStart)
	if err != nil {
		return errors.Wrap(err, "failed to seek to end")
	}

	// Create moov box
	moov := mp4.NewMoovBox()

	// Calculate total duration for mvhd (relative duration, not absolute)
	var maxDuration uint64 = 0
	for _, track := range m.tracks {
		if len(track.samples) > 1 {
			firstSample := track.samples[0]
			lastSample := track.samples[len(track.samples)-1]
			// Duration is relative: last - first
			trackDur := lastSample.pts - firstSample.pts
			// Convert to 1000 timescale for mvhd
			trackDurMvhd := trackDur * 1000 / uint64(track.timescale)
			if trackDurMvhd > maxDuration {
				maxDuration = trackDurMvhd
			}
		}
	}

	// Create mvhd (movie header)
	mvhd := mp4.CreateMvhd()
	mvhd.Timescale = 1000
	mvhd.Duration = maxDuration
	mvhd.NextTrackID = uint32(len(m.tracks) + 1)
	moov.AddChild(mvhd)

	// Create trak boxes for each track
	for _, track := range m.tracks {
		trak, err := m.createTrak(track)
		if err != nil {
			return errors.Wrap(err, "failed to create trak")
		}
		moov.AddChild(trak)
	}

	// Write moov
	err = moov.Encode(m.writer)
	if err != nil {
		return errors.Wrap(err, "failed to write moov")
	}

	return nil
}

func (m *MP4ffMuxer) createTrak(track *mp4ffTrack) (*mp4.TrakBox, error) {
	trak := &mp4.TrakBox{}

	// Calculate track duration and first sample time
	var trackDuration uint64 = 0
	var firstSampleDTS uint64 = 0
	if len(track.samples) > 0 {
		lastSample := track.samples[len(track.samples)-1]
		firstSampleDTS = track.samples[0].dts
		// Duration should be relative (last - first), not absolute
		trackDuration = lastSample.pts - track.samples[0].pts
	}

	// Calculate actual media duration (last DTS - first DTS + last sample duration estimate)
	var mediaDuration uint64 = 0
	if len(track.samples) > 1 {
		mediaDuration = track.samples[len(track.samples)-1].dts - firstSampleDTS
		// Add estimated duration of last sample
		mediaDuration += uint64(track.timescale / 30) // ~30fps default
	} else if len(track.samples) == 1 {
		mediaDuration = uint64(track.timescale / 30)
	}

	// tkhd (track header)
	// Duration should be the edit duration, not raw media duration
	tkhdDuration := mediaDuration * 1000 / uint64(track.timescale)
	tkhd := &mp4.TkhdBox{
		Version:  0,
		Flags:    0x000003, // Track enabled and in movie
		TrackID:  track.trackID,
		Duration: tkhdDuration,
	}
	if track.isVideo && track.codec != nil {
		h264Codec := track.codec.(h264parser.CodecData)
		tkhd.Width = mp4.Fixed32(h264Codec.Width() << 16)
		tkhd.Height = mp4.Fixed32(h264Codec.Height() << 16)
	}
	trak.Tkhd = tkhd
	trak.Children = append(trak.Children, tkhd)

	// edts (edit box) with elst (edit list) - crucial for VLC playback
	// This tells the player to start from firstSampleDTS
	if len(track.samples) > 0 {
		edts := &mp4.EdtsBox{}
		elst := &mp4.ElstBox{
			Version: 0,
			Flags:   0,
		}
		// Single edit entry: play all media starting from firstSampleDTS
		// segment_duration: duration in movie timescale (1000)
		// media_time: start time in media timescale
		// media_rate: 1.0 (0x00010000 in fixed-point)
		segmentDuration := mediaDuration * 1000 / uint64(track.timescale)
		elst.Entries = []mp4.ElstEntry{
			{
				SegmentDuration:   segmentDuration,
				MediaTime:         int64(firstSampleDTS),
				MediaRateInteger:  1,
				MediaRateFraction: 0,
			},
		}
		edts.Elst = []*mp4.ElstBox{elst}
		edts.Children = append(edts.Children, elst)
		trak.Edts = edts
		trak.Children = append(trak.Children, edts)
	}

	// mdia
	mdia := &mp4.MdiaBox{}

	// mdhd (media header)
	mdhd := &mp4.MdhdBox{
		Timescale: track.timescale,
		Duration:  trackDuration,
	}
	mdia.Mdhd = mdhd
	mdia.Children = append(mdia.Children, mdhd)

	// hdlr (handler)
	var hdlr *mp4.HdlrBox
	if track.isVideo {
		hdlr = &mp4.HdlrBox{
			HandlerType: "vide",
			Name:        "VideoHandler",
		}
	} else {
		hdlr = &mp4.HdlrBox{
			HandlerType: "soun",
			Name:        "SoundHandler",
		}
	}
	mdia.Hdlr = hdlr
	mdia.Children = append(mdia.Children, hdlr)

	// minf
	minf := &mp4.MinfBox{}

	// vmhd or smhd
	if track.isVideo {
		vmhd := &mp4.VmhdBox{Flags: 0x000001}
		minf.Vmhd = vmhd
		minf.Children = append(minf.Children, vmhd)
	} else {
		smhd := &mp4.SmhdBox{}
		minf.Smhd = smhd
		minf.Children = append(minf.Children, smhd)
	}

	// dinf - CreateDref already includes a self-contained url
	dinf := &mp4.DinfBox{}
	dref := mp4.CreateDref()
	dinf.Dref = dref
	dinf.Children = append(dinf.Children, dref)
	minf.Dinf = dinf
	minf.Children = append(minf.Children, dinf)

	// stbl
	stbl, err := m.createStbl(track)
	if err != nil {
		return nil, err
	}
	minf.Stbl = stbl
	minf.Children = append(minf.Children, stbl)

	mdia.Minf = minf
	mdia.Children = append(mdia.Children, minf)

	trak.Mdia = mdia
	trak.Children = append(trak.Children, mdia)

	return trak, nil
}

func (m *MP4ffMuxer) createStbl(track *mp4ffTrack) (*mp4.StblBox, error) {
	stbl := &mp4.StblBox{}

	// stsd (sample description)
	stsd := &mp4.StsdBox{SampleCount: 1}
	if track.isVideo {
		// Create AVC1 sample entry using helper
		spsNALUs := [][]byte{track.sps}
		ppsNALUs := [][]byte{track.pps}

		avcC, err := mp4.CreateAvcC(spsNALUs, ppsNALUs, true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create avcC")
		}

		// Parse SPS for dimensions (use defaults if parsing fails)
		width := uint16(1920)
		height := uint16(1080)
		if spsInfo, err := avc.ParseSPSNALUnit(track.sps, true); err == nil && spsInfo != nil {
			width = uint16(spsInfo.Width)
			height = uint16(spsInfo.Height)
		}

		avc1 := mp4.CreateVisualSampleEntryBox("avc1", width, height, avcC)
		stsd.AvcX = avc1
		stsd.Children = append(stsd.Children, avc1)
	} else {
		// Create MP4A sample entry for AAC
		esds := mp4.CreateEsdsBox(track.aacConfig)

		aacCodec := track.codec.(aacparser.CodecData)
		mp4a := mp4.CreateAudioSampleEntryBox("mp4a",
			uint16(aacCodec.ChannelLayout().Count()),
			16, // Sample size
			uint16(aacCodec.SampleRate()),
			esds)
		stsd.Mp4a = mp4a
		stsd.Children = append(stsd.Children, mp4a)
	}
	stbl.Stsd = stsd
	stbl.Children = append(stbl.Children, stsd)

	// stts (decoding time to sample)
	stts := &mp4.SttsBox{}
	if len(track.samples) > 0 {
		var sampleCounts []uint32
		var sampleDeltas []uint32
		var lastDuration uint32 = 0
		var count uint32 = 0

		for i := 0; i < len(track.samples); i++ {
			var duration uint32
			if i < len(track.samples)-1 {
				// Prevent negative duration from DTS going backwards
				if track.samples[i+1].dts > track.samples[i].dts {
					duration = uint32(track.samples[i+1].dts - track.samples[i].dts)
				} else {
					// DTS not increasing - use default duration
					duration = uint32(track.timescale / 30) // Default ~30fps
				}
			} else {
				// Use last known duration for final sample
				if lastDuration > 0 {
					duration = lastDuration
				} else {
					duration = uint32(track.timescale / 30) // Default ~30fps
				}
			}

			if duration == lastDuration && count > 0 {
				count++
			} else {
				if count > 0 {
					sampleCounts = append(sampleCounts, count)
					sampleDeltas = append(sampleDeltas, lastDuration)
				}
				lastDuration = duration
				count = 1
			}
		}
		if count > 0 {
			sampleCounts = append(sampleCounts, count)
			sampleDeltas = append(sampleDeltas, lastDuration)
		}
		stts.SampleCount = sampleCounts
		stts.SampleTimeDelta = sampleDeltas
	}
	stbl.Stts = stts
	stbl.Children = append(stbl.Children, stts)

	// ctts (composition time to sample) - only for video with B-frames
	if track.isVideo {
		hasCTS := false
		for _, s := range track.samples {
			if s.compositionTime != 0 {
				hasCTS = true
				break
			}
		}
		if hasCTS {
			ctts := &mp4.CttsBox{Version: 1}
			// Build sample counts (each entry is 1 sample) and offsets
			var counts []uint32
			var offsets []int32
			for _, s := range track.samples {
				counts = append(counts, 1)
				offsets = append(offsets, s.compositionTime)
			}
			ctts.AddSampleCountsAndOffset(counts, offsets)
			stbl.Ctts = ctts
			stbl.Children = append(stbl.Children, ctts)
		}
	}

	// stss (sync sample - keyframes) - only for video
	if track.isVideo {
		stss := &mp4.StssBox{}
		for i, s := range track.samples {
			if s.isKeyFrame {
				stss.SampleNumber = append(stss.SampleNumber, uint32(i+1))
			}
		}
		if len(stss.SampleNumber) > 0 {
			stbl.Stss = stss
			stbl.Children = append(stbl.Children, stss)
		}
	}

	// stsz (sample sizes)
	stsz := &mp4.StszBox{}
	for _, s := range track.samples {
		stsz.SampleSize = append(stsz.SampleSize, s.size)
	}
	stsz.SampleNumber = uint32(len(track.samples))
	stbl.Stsz = stsz
	stbl.Children = append(stbl.Children, stsz)

	// stsc (sample to chunk) - one sample per chunk for interleaved data
	stsc := &mp4.StscBox{
		Entries: []mp4.StscEntry{
			{
				FirstChunk:      1,
				SamplesPerChunk: 1, // One sample per chunk
			},
		},
		SampleDescriptionID: []uint32{1},
	}
	stbl.Stsc = stsc
	stbl.Children = append(stbl.Children, stsc)

	// stco (chunk offsets) - one offset per sample since samples are interleaved
	// Check if we need 64-bit offsets
	need64bit := false
	for _, s := range track.samples {
		if s.offset > 0xFFFFFFFF {
			need64bit = true
			break
		}
	}

	if need64bit {
		co64 := &mp4.Co64Box{}
		for _, s := range track.samples {
			co64.ChunkOffset = append(co64.ChunkOffset, uint64(s.offset))
		}
		stbl.Co64 = co64
		stbl.Children = append(stbl.Children, co64)
	} else {
		stco := &mp4.StcoBox{}
		for _, s := range track.samples {
			stco.ChunkOffset = append(stco.ChunkOffset, uint32(s.offset))
		}
		stbl.Stco = stco
		stbl.Children = append(stbl.Children, stco)
	}

	return stbl, nil
}

// convertToAVCC converts H.264 data to AVCC format (4-byte length prefix)
func convertToAVCC(data []byte) []byte {
	if len(data) < 4 {
		return data
	}

	// Look for Annex-B start codes
	if data[0] == 0 && data[1] == 0 && data[2] == 0 && data[3] == 1 {
		// 4-byte start code - convert from Annex-B
		return annexBToAVCC(data)
	}
	if data[0] == 0 && data[1] == 0 && data[2] == 1 {
		// 3-byte start code - convert from Annex-B
		return annexBToAVCC(data)
	}

	// Check if already in AVCC format (first 4 bytes = length of remaining data)
	// This handles multi-NAL AVCC packets
	possibleLen := binary.BigEndian.Uint32(data[0:4])
	if possibleLen > 0 && possibleLen <= uint32(len(data)-4) {
		// Verify it looks like valid AVCC by checking NAL type
		if len(data) > 4 {
			nalType := data[4] & 0x1F
			if nalType >= 1 && nalType <= 12 {
				// Looks like valid AVCC format already
				return data
			}
		}
	}

	// Raw NAL data without start codes or length prefix - wrap with 4-byte length
	result := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(result[0:4], uint32(len(data)))
	copy(result[4:], data)
	return result
}

func annexBToAVCC(data []byte) []byte {
	result := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		// Find start code
		startCodeLen := 0
		if i+4 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			startCodeLen = 4
		} else if i+3 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			startCodeLen = 3
		}

		if startCodeLen == 0 {
			i++
			continue
		}

		nalStart := i + startCodeLen
		nalEnd := len(data)

		// Find next start code
		for j := nalStart; j < len(data)-3; j++ {
			if data[j] == 0 && data[j+1] == 0 {
				if j+2 < len(data) && data[j+2] == 1 {
					nalEnd = j
					break
				}
				if j+3 < len(data) && data[j+2] == 0 && data[j+3] == 1 {
					nalEnd = j
					break
				}
			}
		}

		nalData := data[nalStart:nalEnd]
		if len(nalData) > 0 {
			// Write 4-byte length prefix
			lenBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lenBuf, uint32(len(nalData)))
			result = append(result, lenBuf...)
			result = append(result, nalData...)
		}

		i = nalEnd
	}

	if len(result) == 0 {
		return data // Fallback
	}
	return result
}

// Duration returns the duration of the longest track (relative, not absolute)
func (m *MP4ffMuxer) Duration() time.Duration {
	var maxDur time.Duration
	for _, track := range m.tracks {
		if len(track.samples) > 1 {
			firstPts := track.samples[0].pts
			lastPts := track.samples[len(track.samples)-1].pts
			dur := time.Duration(float64(lastPts-firstPts) / float64(track.timescale) * float64(time.Second))
			if dur > maxDur {
				maxDur = dur
			}
		}
	}
	return maxDur
}
