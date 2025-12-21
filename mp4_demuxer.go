package videoserver

import (
	"bytes"
	"io"
	"os"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/pkg/errors"
)

// MP4Demuxer wraps mp4ff to provide av.Packet compatible output
type MP4Demuxer struct {
	file      *os.File
	mp4File   *mp4.File
	tracks    []*demuxerTrack
	codecData []av.CodecData
}

type demuxerTrack struct {
	trak  *mp4.TrakBox
	codec av.CodecData
	// 1-based sample number (mp4ff uses 1-based)
	sampleNr  uint32
	sampleCnt uint32
	timescale uint32
	// Edit list media_time offset (in timescale units)
	mediaTimeOffset uint64
	isVideo         bool
	isAudio         bool
	streamIdx       int8
}

// NewMP4Demuxer creates a new demuxer for the given file
func NewMP4Demuxer(filePath string) (*MP4Demuxer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open file %s", filePath)
	}

	// Use lazy mdat mode so we don't load entire file into memory
	mp4File, err := mp4.DecodeFile(file, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		file.Close()
		return nil, errors.Wrapf(err, "failed to decode MP4 file %s", filePath)
	}

	if mp4File.Moov == nil {
		file.Close()
		return nil, errors.New("no moov box found in MP4 file")
	}

	d := &MP4Demuxer{
		file:    file,
		mp4File: mp4File,
		tracks:  make([]*demuxerTrack, 0),
	}

	// Parse tracks and extract codec data
	streamIdx := int8(0)
	for _, trak := range mp4File.Moov.Traks {
		track := &demuxerTrack{
			trak:      trak,
			timescale: trak.Mdia.Mdhd.Timescale,
			streamIdx: streamIdx,
			sampleNr:  1, // Start at sample 1 (1-based)
		}

		// Get sample count
		stbl := trak.Mdia.Minf.Stbl
		if stbl.Stsz != nil {
			track.sampleCnt = stbl.Stsz.SampleNumber
		}

		// Get edit list media_time offset (used to normalize timestamps)
		if trak.Edts != nil && len(trak.Edts.Elst) > 0 && len(trak.Edts.Elst[0].Entries) > 0 {
			mediaTime := trak.Edts.Elst[0].Entries[0].MediaTime
			if mediaTime > 0 {
				track.mediaTimeOffset = uint64(mediaTime)
			}
		}

		// Get handler type
		hdlrType := trak.Mdia.Hdlr.HandlerType
		stsd := stbl.Stsd

		if hdlrType == "vide" {
			track.isVideo = true
			// Look for AVC1 (H.264)
			if stsd.AvcX != nil && stsd.AvcX.AvcC != nil {
				// Encode AVCC to bytes for vdk
				avcCBytes := encodeAvcC(stsd.AvcX.AvcC)
				codecData, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(avcCBytes)
				if err == nil {
					track.codec = codecData
					d.codecData = append(d.codecData, codecData)
				}
			}
		} else if hdlrType == "soun" {
			track.isAudio = true
			// Look for MP4A (AAC)
			if stsd.Mp4a != nil && stsd.Mp4a.Esds != nil && stsd.Mp4a.Esds.DecConfigDescriptor != nil {
				// Get AAC specific config
				decConfig := stsd.Mp4a.Esds.DecConfigDescriptor.DecSpecificInfo.DecConfig
				codecData, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(decConfig)
				if err == nil {
					track.codec = codecData
					d.codecData = append(d.codecData, codecData)
				}
			}
		}

		if track.codec != nil && track.sampleCnt > 0 {
			d.tracks = append(d.tracks, track)
			streamIdx++
		}
	}

	if len(d.codecData) == 0 {
		file.Close()
		return nil, errors.New("no supported video/audio tracks found")
	}

	return d, nil
}

// encodeAvcC encodes an AvcC box to the raw AVCC bytes format expected by vdk
func encodeAvcC(avcC *mp4.AvcCBox) []byte {
	buf := bytes.Buffer{}
	// Write configuration version
	buf.WriteByte(1)
	// Write AVC profile indication
	buf.WriteByte(avcC.AVCProfileIndication)
	// Write profile compatibility
	buf.WriteByte(avcC.ProfileCompatibility)
	// Write AVC level indication
	buf.WriteByte(avcC.AVCLevelIndication)
	// Write length size minus one (with reserved bits) - always 4 bytes (0x03)
	buf.WriteByte(0xFF) // 0xFC | 0x03
	// Write number of SPS (with reserved bits)
	buf.WriteByte(0xE0 | byte(len(avcC.SPSnalus)&0x1F))
	// Write SPS
	for _, sps := range avcC.SPSnalus {
		buf.WriteByte(byte(len(sps) >> 8))
		buf.WriteByte(byte(len(sps) & 0xFF))
		buf.Write(sps)
	}
	// Write number of PPS
	buf.WriteByte(byte(len(avcC.PPSnalus)))
	// Write PPS
	for _, pps := range avcC.PPSnalus {
		buf.WriteByte(byte(len(pps) >> 8))
		buf.WriteByte(byte(len(pps) & 0xFF))
		buf.Write(pps)
	}
	return buf.Bytes()
}

// Streams returns the codec data for all tracks
func (d *MP4Demuxer) Streams() []av.CodecData {
	return d.codecData
}

// ReadPacket reads the next packet from the demuxer
func (d *MP4Demuxer) ReadPacket() (av.Packet, error) {
	// Find the track with the earliest pending sample
	var earliestTrack *demuxerTrack
	var earliestTime uint64 = ^uint64(0) // Max uint64

	for _, track := range d.tracks {
		if track.sampleNr > track.sampleCnt {
			continue
		}
		// Get decode time for current sample
		stbl := track.trak.Mdia.Minf.Stbl
		decTime, _ := stbl.Stts.GetDecodeTime(track.sampleNr)
		// Convert to common time base (microseconds) for comparison
		timeUs := decTime * 1000000 / uint64(track.timescale)
		if timeUs < earliestTime {
			earliestTime = timeUs
			earliestTrack = track
		}
	}

	if earliestTrack == nil {
		return av.Packet{}, io.EOF
	}

	// Read sample data
	stbl := earliestTrack.trak.Mdia.Minf.Stbl
	sampleNr := earliestTrack.sampleNr

	// Get chunk info
	chunkNr, sampleNrAtChunkStart, err := stbl.Stsc.ChunkNrFromSampleNr(int(sampleNr))
	if err != nil {
		return av.Packet{}, errors.Wrapf(err, "failed to get chunk for sample %d", sampleNr)
	}

	// Get offset
	var offset int64
	if stbl.Stco != nil {
		offset = int64(stbl.Stco.ChunkOffset[chunkNr-1])
	} else if stbl.Co64 != nil {
		offset = int64(stbl.Co64.ChunkOffset[chunkNr-1])
	}

	// Add offset for samples before this one in the chunk
	for sNr := sampleNrAtChunkStart; sNr < int(sampleNr); sNr++ {
		offset += int64(stbl.Stsz.GetSampleSize(sNr))
	}

	// Get sample size
	size := stbl.Stsz.GetSampleSize(int(sampleNr))

	// Read sample data
	_, err = d.file.Seek(offset, io.SeekStart)
	if err != nil {
		return av.Packet{}, errors.Wrapf(err, "failed to seek to sample %d", sampleNr)
	}
	sampleData := make([]byte, size)
	_, err = io.ReadFull(d.file, sampleData)
	if err != nil {
		return av.Packet{}, errors.Wrapf(err, "failed to read sample %d", sampleNr)
	}

	// Get timing info
	decTime, _ := stbl.Stts.GetDecodeTime(sampleNr)
	// Don't apply edit list offset here - let the streaming handler normalize
	// The edit list offset in archive files equals the first sample's time,
	// so subtracting it would collapse all times to near-0
	pts := time.Duration(decTime) * time.Second / time.Duration(earliestTrack.timescale)

	// Get composition time offset if present
	var compositionTime time.Duration
	if stbl.Ctts != nil {
		cto := stbl.Ctts.GetCompositionTimeOffset(sampleNr)
		compositionTime = time.Duration(cto) * time.Second / time.Duration(earliestTrack.timescale)
	}

	// Check if sync sample (keyframe)
	isKeyFrame := true
	if stbl.Stss != nil {
		isKeyFrame = stbl.Stss.IsSyncSample(sampleNr)
	}

	// Advance to next sample
	earliestTrack.sampleNr++

	pkt := av.Packet{
		Idx:             earliestTrack.streamIdx,
		IsKeyFrame:      isKeyFrame,
		Time:            pts,
		CompositionTime: compositionTime,
		Data:            sampleData,
	}

	return pkt, nil
}

// SeekToStart resets all tracks to the beginning
func (d *MP4Demuxer) SeekToStart() {
	for _, track := range d.tracks {
		track.sampleNr = 1
	}
}

// Close closes the demuxer and underlying file
func (d *MP4Demuxer) Close() error {
	if d.file != nil {
		return d.file.Close()
	}
	return nil
}
