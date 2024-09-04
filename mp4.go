package videoserver

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/mp4"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func (app *Application) startMP4(streamID uuid.UUID, ch chan av.Packet, stopCast chan bool) error {
	var err error
	archive := app.getStreamArchive(streamID)
	if archive == nil {
		return errors.Wrap(err, "Bad archive stream")
	}

	err = ensureDir(archive.dir)
	if err != nil {
		return errors.Wrap(err, "Can't create directory for mp4 temporary files")
	}

	isConnected := true
	segmentNumber := 0
	lastPacketTime := time.Duration(0)
	lastKeyFrame := av.Packet{}

	// time.Sleep(5 * time.Second) // Artificial delay to wait for first key frame
	for isConnected {
		// Create new segment file
		segmentName := fmt.Sprintf("%s%04d.mp4", streamID, segmentNumber)
		segmentPath := filepath.Join(archive.dir, segmentName)
		outFile, err := os.Create(segmentPath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Can't create mp4-segment for stream %s", streamID))
		}
		tsMuxer := mp4.NewMuxer(outFile)

		// Write header
		codecData, err := app.getCodec(streamID)
		if err != nil {
			return errors.Wrap(err, streamID.String())
		}
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
					if segmentLength.Milliseconds() >= archive.msPerSegment {
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
		segmentNumber++
	}
	return nil
}
