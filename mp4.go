package videoserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/LdDl/video-server/storage"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/mp4"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const (
	maxFailureDuration = 3 * time.Second
)

var (
	ErrTimeFailure = fmt.Errorf("bad packet times")
)

func (app *Application) startMP4(archive *StreamArchiveWrapper, streamID uuid.UUID, ch chan av.Packet, stopCast chan StopSignal, streamVerboseLevel VerboseLevel) error {
	if archive == nil {
		return ErrNullArchive
	}
	var err error
	err = archive.store.MakeBucket(archive.bucket)
	if err != nil {
		return errors.Wrap(err, "Can't prepare bucket")
	}
	err = ensureDir(archive.filesystemDir)
	if err != nil {
		return errors.Wrap(err, "Can't create directory for mp4 temporary files")
	}

	isConnected := true
	lastSegmentTime := time.Now()
	lastPacketTime := time.Duration(0)
	lastKeyFrame := av.Packet{}

	// time.Sleep(5 * time.Second) // Artificial delay to wait for first key frame
	for isConnected {
		// Create new segment file
		st := time.Now()
		segmentName := fmt.Sprintf("%s_%d.mp4", streamID, lastSegmentTime.Unix())
		segmentPath := filepath.Join(archive.filesystemDir, segmentName)

		outFile, err := os.Create(segmentPath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't create mp4-segment for stream %s", streamID))
		}

		fileClosed := false
		defer func(file *os.File) {
			if fileClosed {
				return
			}
			log.Warn().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_MP4_CLOSE).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("File has not been closed in right order")
			if err := file.Close(); err != nil {
				log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_CLOSE).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("Can't close file")
				// @todo: handle?
			}
		}(outFile)

		tsMuxer := mp4.NewMuxer(outFile)
		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CREATE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Msg("Create segment")
		codecData, err := app.Streams.GetCodecsDataForStream(streamID)
		if err != nil {
			return errors.Wrap(err, streamID.String())
		}
		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CREATE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Msg("Write header")

		err = tsMuxer.WriteHeader(codecData)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't write header for mp4 muxer for stream %s", streamID))
		}

		// Write packets
		videoStreamIdx := int8(0)
		for idx, codec := range codecData {
			if codec.Type().IsVideo() {
				videoStreamIdx = int8(idx)
				break
			}
		}
		segmentLength := time.Duration(0)
		packetLength := time.Duration(0)
		segmentCount := 0
		start := false
		failureDuration := time.Duration(0)

		// Write lastKeyFrame if exist
		if lastKeyFrame.IsKeyFrame {
			start = true
			if err = tsMuxer.WritePacket(lastKeyFrame); err != nil {
				return errors.Wrap(err, fmt.Sprintf("Can't write packet for TS muxer for stream %s (1)", streamID))
			}
			// Evaluate segment's length
			packetLength = lastKeyFrame.Time - lastPacketTime
			lastPacketTime = lastKeyFrame.Time
			segmentLength += packetLength
			segmentCount++
		}
		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CREATE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Msg("Start segment loop")

		lastKeyFrame, lastPacketTime, isConnected, failureDuration, err = processingMP4(streamID, segmentName, isConnected, start, videoStreamIdx, segmentCount, segmentLength, lastKeyFrame, lastPacketTime, packetLength, archive.msPerSegment, tsMuxer, ch, stopCast, failureDuration, streamVerboseLevel)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Dur("failure_dur", failureDuration).Msg("Can't process mp4 channel")
		}

		if err := tsMuxer.WriteTrailer(); err != nil {
			log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE_TRAIL).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("Can't write trailing data for TS muxer")
			// @todo: handle?
		}

		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CLOSE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Int64("ms", archive.msPerSegment).Msg("Closing segment")
		fileClosed = true
		if err := outFile.Close(); err != nil {
			log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_CLOSE).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("Can't close file")
			// @todo: handle?
		}

		if archive.store.Type() == storage.STORAGE_MINIO {
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Drop segment to minio")
			}
			go func() {
				st := time.Now()
				outSegmentName, err := UploadToMinio(archive.store, segmentName, archive.bucket, segmentPath)
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Uploaded segment file")
				}
				elapsed := time.Since(st)
				if err != nil {
					log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_SAVE_MINIO).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("elapsed", elapsed).Msg("Can't upload segment to MinIO")
				}
				if segmentName != outSegmentName {
					log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_SAVE_MINIO).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Dur("elapsed", elapsed).Msg("Can't validate segment")
				}
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_SAVE_MINIO).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("elapsed", elapsed).Msg("Saved to MinIO")
				}
			}()
		}

		lastSegmentTime = lastSegmentTime.Add(time.Since(st))
		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CLOSE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Int64("ms", archive.msPerSegment).Msg("Closed segment")
	}
	return nil
}

func processingMP4(
	streamID uuid.UUID,
	segmentName string,
	isConnected,
	start bool,
	videoStreamIdx int8,
	segmentCount int,
	segmentLength time.Duration,
	lastKeyFrame av.Packet,
	lastPacketTime time.Duration,
	packetLength time.Duration,
	msPerSegment int64,
	tsMuxer *mp4.Muxer,
	ch chan av.Packet,
	stopCast chan StopSignal,
	failureDuration time.Duration,
	streamVerboseLevel VerboseLevel,
) (av.Packet, time.Duration, bool, time.Duration, error) {
	for {
		select {
		case sig := <-stopCast:
			isConnected = false
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_STOP).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Any("stop_signal", sig).Dur("prev_pck_time", lastPacketTime).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Stop cast signal")
			}
			return lastKeyFrame, lastPacketTime, isConnected, failureDuration, nil
		case pck := <-ch:
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_PACKET).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Recieved something in archive channel")
			}
			if pck.Idx == videoStreamIdx && pck.IsKeyFrame {
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_KEYFRAME).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Packet is a keyframe")
				}
				start = true
				if segmentLength.Milliseconds() >= msPerSegment {
					if streamVerboseLevel > VERBOSE_NONE {
						log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_SEGMENT_CUT).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Need to cut segment")
					}
					lastKeyFrame = pck
					return lastKeyFrame, lastPacketTime, isConnected, failureDuration, nil
				}
			}
			if !start {
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_NO_START).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Still no start")
				}
				continue
			}
			if (pck.Idx == videoStreamIdx && pck.Time > lastPacketTime) || pck.Idx != videoStreamIdx {
				failureDuration = time.Duration(0)
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Writing to archive segment")
				}
				err := tsMuxer.WritePacket(pck)
				if err != nil {
					return lastKeyFrame, lastPacketTime, isConnected, failureDuration, errors.Wrap(err, fmt.Sprintf("Can't write packet for TS muxer for stream %s (2)", streamID))
				}
				if pck.Idx == videoStreamIdx {
					// Evaluate segment length
					packetLength = pck.Time - lastPacketTime
					lastPacketTime = pck.Time
					if packetLength.Milliseconds() > msPerSegment { // If comment this you get [0; keyframe time] interval for the very first video file
						if streamVerboseLevel > VERBOSE_NONE {
							log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Very first interval")
						}
						continue
					}
					segmentLength += packetLength
				}
				segmentCount++
			} else {
				if streamVerboseLevel > VERBOSE_NONE {
					log.Warn().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Dur("failure_dur", failureDuration).Msg("Current packet time < previous")
				}
				failureDuration += pck.Duration
				if failureDuration > maxFailureDuration {
					return lastKeyFrame, lastPacketTime, isConnected, failureDuration, ErrTimeFailure
				}
			}
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_PACKET).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Dur("pck_time", pck.Time).Dur("prev_pck_time", lastPacketTime).Dur("pck_dur", pck.Duration).Int8("pck_idx", pck.Idx).Int8("stream_idx", videoStreamIdx).Int("segment_count", segmentCount).Dur("segment_len", segmentLength).Msg("Wait other in archive channel")
			}
		}
	}
	return lastKeyFrame, lastPacketTime, isConnected, failureDuration, nil
}

func UploadToMinio(minioStorage storage.ArchiveStorage, segmentName, bucket, sourceFileName string) (string, error) {
	obj := storage.ArchiveUnit{
		SegmentName: segmentName,
		Bucket:      bucket,
		FileName:    sourceFileName,
	}
	ctx := context.Background()
	outSegmentName, err := minioStorage.UploadFile(ctx, obj)
	if err != nil {
		return "", err
	}
	err = os.Remove(sourceFileName)
	return outSegmentName, err
}
