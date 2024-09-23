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

func (app *Application) startMP4(archive *StreamArchiveWrapper, streamID uuid.UUID, ch chan av.Packet, stopCast chan bool, streamVerboseLevel VerboseLevel) error {
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
		// @todo Oh, I don't like GOTOs, but it is what it is.
	segmentLoop:
		for {
			select {
			case <-stopCast:
				isConnected = false
				if streamVerboseLevel > VERBOSE_NONE {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_STOP).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Stop cast signal")
				}
				break segmentLoop
			case pck := <-ch:
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_PACKET).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Recieved something in archive channel")
				}
				if pck.Idx == videoStreamIdx && pck.IsKeyFrame {
					if streamVerboseLevel > VERBOSE_ADD {
						log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_KEYFRAME).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Packet is a keyframe")
					}
					start = true
					if segmentLength.Milliseconds() >= archive.msPerSegment {
						if streamVerboseLevel > VERBOSE_ADD {
							log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_SEGMENT_CUT).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Need to cut segment")
						}
						lastKeyFrame = pck
						break segmentLoop
					}
				}
				if !start {
					if streamVerboseLevel > VERBOSE_ADD {
						log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_NO_START).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Still no start")
					}
					continue
				}
				if (pck.Idx == videoStreamIdx && pck.Time > lastPacketTime) || pck.Idx != videoStreamIdx {
					if streamVerboseLevel > VERBOSE_ADD {
						log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Writing to archive segment")
					}
					err = tsMuxer.WritePacket(pck)
					if err != nil {
						return errors.Wrap(err, fmt.Sprintf("Can't write packet for TS muxer for stream %s (2)", streamID))
					}
					if pck.Idx == videoStreamIdx {
						// Evaluate segment length
						packetLength = pck.Time - lastPacketTime
						lastPacketTime = pck.Time
						if packetLength.Milliseconds() > archive.msPerSegment { // If comment this you get [0; keyframe time] interval for the very first video file
							continue
						}
						segmentLength += packetLength
					}
					segmentCount++
				} else {
					// fmt.Println("Current packet time < previous ")
				}
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_CHAN_PACKET).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Wait other in archive channel")
				}
			}
		}
		if err := tsMuxer.WriteTrailer(); err != nil {
			log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE_TRAIL).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("Can't write trailing data for TS muxer")
			// @todo: handle?
		}

		log.Info().Str("scope", SCOPE_ARCHIVE).Str("event", EVENT_ARCHIVE_CLOSE_FILE).Str("stream_id", streamID.String()).Str("segment_path", segmentPath).Int64("ms", archive.msPerSegment).Msg("Closing segment")
		if err := outFile.Close(); err != nil {
			log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_CLOSE).Str("stream_id", streamID.String()).Str("out_filename", outFile.Name()).Msg("Can't close file")
			// @todo: handle?
		}

		if archive.store.Type() == storage.STORAGE_MINIO {
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Drop segment to minio")
			}
			// _, err = outFile.Seek(0, io.SeekStart)
			// if err != nil {
			// 	log.Error().Err(err).Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_SAVE_MINIO).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Can't seek to the start of file")
			// 	return err
			// }
			// if streamVerboseLevel > VERBOSE_ADD {
			// 	log.Info().Str("scope", SCOPE_MP4).Str("event", EVENT_MP4_WRITE).Str("stream_id", streamID.String()).Str("segment_name", segmentName).Msg("Done seek to start of segment file")
			// }
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

func UploadToMinio(minioStorage storage.ArchiveStorage, segmentName, bucket, sourceFileName string) (string, error) {
	obj := storage.ArchiveUnit{
		// Payload:     outFile,
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
