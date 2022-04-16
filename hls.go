package videoserver

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/ts"
	"github.com/google/uuid"
	"github.com/grafov/m3u8"
	"github.com/pkg/errors"
)

func (app *Application) startHls(streamID uuid.UUID, ch chan av.Packet, stopCast chan bool) error {

	err := ensureDir(app.HlsDirectory)
	if err != nil {
		return errors.Wrap(err, "Can't create directory for HLS temporary files")
	}

	// Create playlist for HLS streams
	playlistFileName := filepath.Join(app.HlsDirectory, fmt.Sprintf("%s.m3u8", streamID))
	log.Printf("Need to start HLS: %s\n", playlistFileName)
	playlist, err := m3u8.NewMediaPlaylist(app.HlsWindowSize, app.HlsCapacity)
	if err != nil {
		return errors.Wrap(err, "Can't create new mediaplayer list")
	}

	isConnected := true
	segmentNumber := 0
	lastPacketTime := time.Duration(0)
	lastKeyFrame := av.Packet{}

	for isConnected {
		// Create new segment file
		segmentName := fmt.Sprintf("%s%04d.ts", streamID, segmentNumber)
		segmentPath := filepath.Join(app.HlsDirectory, segmentName)
		outFile, err := os.Create(segmentPath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't create TS-segment for stream %s", streamID))
		}
		tsMuxer := ts.NewMuxer(outFile)

		// Write header
		codecData, err := app.codecGet(streamID)
		if err != nil {
			return errors.Wrap(err, streamID.String())
		}
		if err := tsMuxer.WriteHeader(codecData); err != nil {
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

	segmentLoop:
		for {
			select {
			case <-stopCast:
				isConnected = false
				break segmentLoop
			case pck := <-ch:
				if pck.Idx == videoStreamIdx && pck.IsKeyFrame {
					start = true
					if segmentLength.Milliseconds() >= app.HlsMsPerSegment {
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

		if err := tsMuxer.WriteTrailer(); err != nil {
			log.Printf("Can't write trailing data for TS muxer for %s: %s\n", streamID, err.Error())
		}

		if err := outFile.Close(); err != nil {
			log.Printf("Can't close file %s: %s\n", outFile.Name(), err.Error())
		}

		// Update playlist
		playlist.Slide(segmentName, segmentLength.Seconds(), "")
		playlistFile, err := os.Create(playlistFileName)
		if err != nil {
			log.Printf("Can't create playlist %s: %s\n", playlistFileName, err.Error())
		}
		playlistFile.Write(playlist.Encode().Bytes())
		playlistFile.Close()
		// log.Printf("m3u8 file has been re-created: %s\n", playlistFileName)

		// Cleanup segments
		if err := app.removeOutdatedSegments(streamID, playlist); err != nil {
			log.Printf("Can't call removeOutdatedSegments on stream %s: %s\n", streamID, err.Error())
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
				if err := os.Remove(filepath.Join(app.HlsDirectory, file)); err != nil {
					log.Printf("Can't call defered file remove: %s\n", err.Error())
				}
			}
		}
	}(time.Duration(app.HlsMsPerSegment*int64(playlist.Count()))*time.Millisecond, filesToRemove)

	return nil
}

func (app *Application) removeOutdatedSegments(streamID uuid.UUID, playlist *m3u8.MediaPlaylist) error {
	// Write all playlist segment URIs into map
	currentSegments := make(map[string]struct{}, len(playlist.Segments))
	for _, segment := range playlist.Segments {
		if segment != nil {
			currentSegments[segment.URI] = struct{}{}
		}
	}
	// Find possible segment files in current directory
	segmentFiles, err := filepath.Glob(filepath.Join(app.HlsDirectory, fmt.Sprintf("%s*.ts", streamID)))
	if err != nil {
		log.Printf("Can't find glob for '%s': %s\n", streamID, err.Error())
		return err
	}
	for _, segmentFile := range segmentFiles {
		_, fileName := filepath.Split(segmentFile)
		// Check if file belongs to a playlist's segment
		if _, ok := currentSegments[fileName]; !ok {
			if err := os.Remove(segmentFile); err != nil {
				log.Printf("Can't call removeOutdatedSegments() for segment %s: %s\n", segmentFile, err.Error())
			}
		}
	}
	return nil
}

func ensureDir(dirName string) error {
	err := os.MkdirAll(dirName, 0777)
	if err == nil || os.IsExist(err) {
		return nil
	}
	return err
}
