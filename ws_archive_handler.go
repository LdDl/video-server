package videoserver

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/format/mp4f"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// Chunk size for archive streaming (5 minutes worth of video time)
const archiveChunkDuration = 5 * time.Minute

// ArchiveWSHandler handles WebSocket connections for archive playback
// Query params: stream_id, start (unix timestamp), duration (seconds, optional)
// Supports chunked streaming: server sends ~5 minutes, then waits for "more" message
func ArchiveWSHandler(app *Application, wsUpgrader *websocket.Upgrader, verboseLevel VerboseLevel) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		streamIDStr := r.FormValue("stream_id")
		startStr := r.FormValue("start")
		durationStr := r.FormValue("duration")

		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("scope", SCOPE_WS_HANDLER).Str("event", "archive_ws").
				Str("remote_addr", r.RemoteAddr).
				Str("stream_id", streamIDStr).
				Str("start", startStr).
				Str("duration", durationStr).
				Msg("Archive WebSocket connection")
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			if verboseLevel > VERBOSE_NONE {
				log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Msg("Can't upgrade to WebSocket")
			}
			return
		}
		defer conn.Close()

		// Parse stream ID
		streamID, err := uuid.Parse(streamIDStr)
		if err != nil {
			closeWSwithError(conn, 1011, "Invalid stream_id")
			return
		}

		// Parse start time
		startUnix, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			closeWSwithError(conn, 1011, "Invalid start time (use Unix timestamp)")
			return
		}
		startTime := time.Unix(startUnix, 0)

		// Parse duration (optional - if not specified, stream until archive ends)
		var endTime time.Time
		if durationStr != "" {
			duration, _ := strconv.ParseInt(durationStr, 10, 64)
			if duration > 0 {
				endTime = startTime.Add(time.Duration(duration) * time.Second)
			}
		}
		// If no duration specified, use far future to get all available segments
		if endTime.IsZero() {
			// 1 year ahead
			endTime = time.Now().Add(24 * 365 * time.Hour)
		}

		// Get archive storage
		archive := app.Streams.GetStreamArchiveStorage(streamID)
		if archive == nil {
			closeWSwithError(conn, 1011, "Archive not enabled for this stream")
			return
		}

		// List segments in time range
		segments, err := archive.store.ListFiles(r.Context(), archive.filesystemDir, streamID.String(), startTime, endTime)
		if err != nil {
			log.Error().Err(err).Str("stream_id", streamIDStr).Msg("Failed to list archive segments")
			closeWSwithError(conn, 1011, "Failed to list archive segments")
			return
		}

		if len(segments) == 0 {
			closeWSwithError(conn, 1011, "No segments found for this time range")
			return
		}

		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("stream_id", streamIDStr).Int("segment_count", len(segments)).Msg("Found archive segments")
			for i, seg := range segments {
				log.Info().Int("idx", i).Str("name", seg.SegmentName).Time("start", seg.StartTime).Msg("Segment")
			}
		}

		// Find first valid segment to get codec info
		// Some segments may be corrupted (not finalized properly), skip them
		var demuxer *MP4Demuxer
		var firstValidSegmentIdx int = -1

		for i, seg := range segments {
			segPath, err := archive.store.GetFilePath(r.Context(), archive.filesystemDir, seg.SegmentName)
			if err != nil {
				if verboseLevel > VERBOSE_NONE {
					log.Warn().Err(err).Str("segment", seg.SegmentName).Msg("Failed to get segment path, skipping")
				}
				continue
			}

			demuxer, err = NewMP4Demuxer(segPath)
			if err != nil {
				if verboseLevel > VERBOSE_NONE {
					log.Warn().Err(err).Str("path", segPath).Msg("Failed to open archive segment (possibly corrupted), skipping")
				}
				continue
			}

			codecData := demuxer.Streams()
			if len(codecData) == 0 {
				demuxer.Close()
				if verboseLevel > VERBOSE_NONE {
					log.Warn().Str("path", segPath).Msg("No codec data in segment, skipping")
				}
				continue
			}

			firstValidSegmentIdx = i
			break
		}

		if firstValidSegmentIdx < 0 || demuxer == nil {
			closeWSwithError(conn, 1011, "No valid archive segments found")
			return
		}

		codecData := demuxer.Streams()

		// Create muxer and send init
		muxer := mp4f.NewMuxer(nil)
		err = muxer.WriteHeader(codecData)
		if err != nil {
			demuxer.Close()
			closeWSwithError(conn, 1011, "Failed to write muxer header")
			return
		}

		meta, init := muxer.GetInit(codecData)

		// Send codec meta (byte 9 prefix)
		err = conn.WriteMessage(websocket.BinaryMessage, append([]byte{9}, meta...))
		if err != nil {
			demuxer.Close()
			closeWSwithError(conn, 1011, "Failed to send meta")
			return
		}

		// Send init segment
		err = conn.WriteMessage(websocket.BinaryMessage, init)
		if err != nil {
			demuxer.Close()
			closeWSwithError(conn, 1011, "Failed to send init")
			return
		}

		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("stream_id", streamIDStr).Str("meta", meta).Msg("Sent archive init segment")
		}

		// Handle client messages (ping/pong and "more" for flow control) in goroutine
		quitCh := make(chan bool, 1)
		moreCh := make(chan bool, 1)
		go func() {
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					quitCh <- true
					return
				}
				if msgType == websocket.TextMessage {
					switch string(data) {
					case "ping":
						conn.WriteMessage(websocket.TextMessage, []byte("pong"))
					case "more":
						select {
						case moreCh <- true:
						default:
							// Channel full, ignore duplicate
						}
					}
				}
			}
		}()

		// Stream all segments starting from first valid one
		var totalPackets int
		var currentDemuxer *MP4Demuxer = demuxer

		// For seamless multi-segment playback:
		// - segmentBaseTime: offset of first packet within current segment
		// - accumulatedTime: total time from previous segments
		// - Each packet's final time = (pkt.Time - segmentBaseTime) + accumulatedTime
		var segmentBaseTime time.Duration
		var accumulatedTime time.Duration
		var lastPacketTime time.Duration
		var segmentFirstPacket bool

		// For chunked streaming flow control:
		// - chunkStartTime: normalized time when current chunk started
		// - After sending archiveChunkDuration worth of video, pause and wait for "more"
		var chunkStartTime time.Duration
		var isPaused bool

		for segIdx, seg := range segments {
			// Skip segments before the first valid one (already processed during init)
			if segIdx < firstValidSegmentIdx {
				continue
			}

			select {
			case <-quitCh:
				if currentDemuxer != nil {
					currentDemuxer.Close()
				}
				return
			default:
			}

			// For segments after the first valid one, open new demuxer
			if segIdx > firstValidSegmentIdx {
				// Before switching segments, update accumulated time
				// Add the duration of the previous segment
				if lastPacketTime > 0 {
					accumulatedTime = lastPacketTime
				}

				if currentDemuxer != nil {
					currentDemuxer.Close()
					currentDemuxer = nil
				}
				segPath, err := archive.store.GetFilePath(r.Context(), archive.filesystemDir, seg.SegmentName)
				if err != nil {
					if verboseLevel > VERBOSE_NONE {
						log.Warn().Err(err).Str("segment", seg.SegmentName).Msg("Failed to get segment path, skipping")
					}
					continue
				}
				currentDemuxer, err = NewMP4Demuxer(segPath)
				if err != nil {
					if verboseLevel > VERBOSE_NONE {
						log.Warn().Err(err).Str("segment", seg.SegmentName).Msg("Failed to open segment (possibly corrupted), skipping")
					}
					currentDemuxer = nil
					continue
				}
			}

			// Skip if demuxer failed to open
			if currentDemuxer == nil {
				continue
			}

			// Reset for new segment
			segmentFirstPacket = true
			segmentBaseTime = 0

			// Read and send packets from this segment
			for {
				select {
				case <-quitCh:
					if currentDemuxer != nil {
						currentDemuxer.Close()
					}
					return
				default:
				}

				pkt, err := currentDemuxer.ReadPacket()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}

				// For first packet of each segment, capture the base time
				if segmentFirstPacket {
					segmentBaseTime = pkt.Time
					segmentFirstPacket = false
				}

				// Normalize: subtract segment's base time, add accumulated time from previous segments
				pkt.Time = (pkt.Time - segmentBaseTime) + accumulatedTime
				lastPacketTime = pkt.Time

				ready, buf, err := muxer.WritePacket(pkt, false)
				if err != nil {
					continue
				}

				if ready {
					err = conn.SetWriteDeadline(time.Now().Add(deadlineTimeout))
					if err != nil {
						if currentDemuxer != nil {
							currentDemuxer.Close()
						}
						return
					}
					err = conn.WriteMessage(websocket.BinaryMessage, buf)
					if err != nil {
						if currentDemuxer != nil {
							currentDemuxer.Close()
						}
						return
					}
				}
				totalPackets++

				// Check if we've sent a chunk worth of video and need to pause
				if !isPaused && (lastPacketTime-chunkStartTime) >= archiveChunkDuration {
					// Send pause message to client
					err = conn.WriteMessage(websocket.TextMessage, []byte("pause"))
					if err != nil {
						if currentDemuxer != nil {
							currentDemuxer.Close()
						}
						return
					}
					isPaused = true

					// Wait for "more" message or quit
					select {
					case <-quitCh:
						if currentDemuxer != nil {
							currentDemuxer.Close()
						}
						return
					case <-moreCh:
						isPaused = false
						// Reset chunk start for next chunk
						chunkStartTime = lastPacketTime
					}
				}
			}
		}

		if currentDemuxer != nil {
			currentDemuxer.Close()
		}

		// Flush any remaining buffered data from muxer
		if totalPackets > 0 {
			_, buf, _ := muxer.WritePacket(av.Packet{}, true)
			if len(buf) > 0 {
				conn.SetWriteDeadline(time.Now().Add(deadlineTimeout))
				conn.WriteMessage(websocket.BinaryMessage, buf)
			}
		}

		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("stream_id", streamIDStr).Int("total_packets", totalPackets).Msg("Archive playback complete")
		}

		// Send end-of-stream message
		conn.WriteMessage(websocket.TextMessage, []byte("eos"))
	}
}
