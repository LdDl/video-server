package videoserver

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/ts"
	"github.com/google/uuid"
	"github.com/grafov/m3u8"
	"github.com/pkg/errors"
)

// startHls starts routine to create m3u8 playlists
func (app *Application) startHls(streamID uuid.UUID, ch chan av.Packet, stopCast chan StopSignal) error {
	err := ensureDir(app.HLS.Directory)
	if err != nil {
		return errors.Wrap(err, "Can't create directory for HLS temporary files")
	}

	// Create playlist for HLS streams
	playlistFileName := filepath.Join(app.HLS.Directory, fmt.Sprintf("%s.m3u8", streamID))
	log.Info().Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_PLAYLIST_PREPARE).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Msg("Need to start HLS for the given stream")
	playlist, err := m3u8.NewMediaPlaylist(app.HLS.WindowSize, app.HLS.Capacity)
	if err != nil {
		return errors.Wrap(err, "Can't create new mediaplayer list")
	}

	isConnected := true
	segmentNumber := 0
	lastPacketTime := time.Duration(0)
	lastKeyFrame := av.Packet{}

	// time.Sleep(5 * time.Second) // Artificial delay to wait for first key frame
	for isConnected {
		// Create new segment file
		segmentName := fmt.Sprintf("%s%04d.ts", streamID, segmentNumber)
		segmentPath := filepath.Join(app.HLS.Directory, segmentName)
		outFile, err := os.Create(segmentPath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't create TS-segment for stream %s", streamID))
		}
		tsMuxer := ts.NewMuxer(outFile)

		// Write header
		codecData, err := app.Streams.GetCodecsDataForStream(streamID)
		if err != nil {
			return errors.Wrap(err, streamID.String())
		}
		err = tsMuxer.WriteHeader(codecData)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't write header for TS muxer for stream %s", streamID))
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

		// @todo Oh, I don't like GOTOs, but it is what it is.
	segmentLoop:
		for {
			select {
			case <-stopCast:
				isConnected = false
				break segmentLoop
			case pck := <-ch:
				if pck.Idx == videoStreamIdx && pck.IsKeyFrame {
					start = true
					if segmentLength.Milliseconds() >= app.HLS.MsPerSegment {
						lastKeyFrame = pck
						break segmentLoop
					}
				}
				if !start {
					continue
				}
				if (pck.Idx == videoStreamIdx && pck.Time > lastPacketTime) || pck.Idx != videoStreamIdx {
					if err = tsMuxer.WritePacket(pck); err != nil {
						return errors.Wrap(err, fmt.Sprintf("Can't write packet for TS muxer for stream %s (2)", streamID))
					}
					if pck.Idx == videoStreamIdx {
						// Evaluate segment length
						packetLength = pck.Time - lastPacketTime
						lastPacketTime = pck.Time
						segmentLength += packetLength
					}
					segmentCount++
				} else {
					// fmt.Println("Current packet time < previous ")
				}
			}
		}

		err = tsMuxer.WriteTrailer()
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_WRITE_TRAIL).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("out_filename", outFile.Name()).Msg("Can't write trailing data for TS muxer")
			// @todo: handle?
		}

		err = outFile.Close()
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_CLOSE_FILE).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("out_filename", outFile.Name()).Msg("Can't close file")
			// @todo: handle?
		}

		// Update playlist
		playlist.Slide(segmentName, segmentLength.Seconds(), "")
		playlistFile, err := os.Create(playlistFileName)
		if err != nil {
			log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_PLAYLIST_CREATE).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("out_filename", outFile.Name()).Msg("Can't create playlist")
			// @todo: handle?
		}
		playlistFile.Write(playlist.Encode().Bytes())
		playlistFile.Close()
		log.Info().Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_PLAYLIST_RESTART).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("out_filename", outFile.Name()).Msg("Playlist restart")
		// Cleanup segments
		if err := app.removeOutdatedSegments(streamID, playlist); err != nil {
			log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_REMOVE_OUTDATED).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("out_filename", outFile.Name()).Msg("Can't remove outdated segments")
			// @todo: handle?
		}

		segmentNumber++
	}

	filesToRemove := make([]string, len(playlist.Segments)+1)

	// Collect obsolete files
	for _, segment := range playlist.Segments {
		if segment != nil {
			filesToRemove = append(filesToRemove, segment.URI)
		}
	}
	_, fileName := filepath.Split(playlistFileName)
	filesToRemove = append(filesToRemove, fileName)

	// Defered removement
	go func(delay time.Duration, filesToRemove []string) {
		time.Sleep(delay)
		for _, file := range filesToRemove {
			if file != "" {
				if err := os.Remove(filepath.Join(app.HLS.Directory, file)); err != nil {
					log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_REMOVE_CHUNK).Str("stream_id", streamID.String()).Str("filename", playlistFileName).Str("chunk_name", file).Msg("Can't remove file (defered)")
					// @todo: handle?
				}
			}
		}
	}(time.Duration(app.HLS.MsPerSegment*int64(playlist.Count()))*time.Millisecond, filesToRemove)

	return nil
}

// removeOutdatedSegments removes outdated *.ts
func (app *Application) removeOutdatedSegments(streamID uuid.UUID, playlist *m3u8.MediaPlaylist) error {
	// Write all playlist segment URIs into map
	currentSegments := make(map[string]struct{}, len(playlist.Segments))
	for _, segment := range playlist.Segments {
		if segment != nil {
			currentSegments[segment.URI] = struct{}{}
		}
	}
	// Find possible segment files in current directory
	segmentFiles, err := filepath.Glob(filepath.Join(app.HLS.Directory, fmt.Sprintf("%s*.ts", streamID)))
	if err != nil {
		return err
	}
	for _, segmentFile := range segmentFiles {
		_, fileName := filepath.Split(segmentFile)
		// Check if file belongs to a playlist's segment
		if _, ok := currentSegments[fileName]; !ok {
			if err := os.Remove(segmentFile); err != nil {
				log.Error().Err(err).Str("scope", SCOPE_HLS).Str("event", EVENT_HLS_REMOVE_OUTDATED_SEGMENT).Str("stream_id", streamID.String()).Str("filename", playlist.String()).Str("segment", segmentFile).Msg("Can't remove outdated segment")
				// @todo: handle?
			}
		}
	}
	return nil
}
