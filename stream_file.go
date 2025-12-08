package videoserver

import (
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// runLocalFileStream runs stream from a local video file (MP4)
func (app *Application) runLocalFileStream(streamID uuid.UUID, filePath string, loop, hlsEnabled, archiveEnabled bool, streamVerboseLevel VerboseLevel) error {
	var stopHlsCast, stopMP4Cast chan StopSignal

	if hlsEnabled {
		stopHlsCast = make(chan StopSignal, 1)
	}
	if archiveEnabled {
		stopMP4Cast = make(chan StopSignal, 1)
	}

	errorSignal := make(chan error, 1)

	if streamVerboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DIAL).Str("stream_id", streamID.String()).Str("file_path", filePath).Bool("hls_enabled", hlsEnabled).Bool("loop", loop).Msg("Opening local file for streaming")
	}

	// Create MP4 demuxer using mp4ff (no size limits)
	demuxer, err := NewMP4Demuxer(filePath)
	if err != nil {
		return errors.Wrapf(err, "Can't create demuxer for file '%s'", filePath)
	}
	defer func() {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DIAL).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Closing file")
		}
		if hlsEnabled {
			stopHlsCast <- STOP_SIGNAL_STOP_DIAL
		}
		if archiveEnabled {
			stopMP4Cast <- STOP_SIGNAL_STOP_DIAL
		}
		demuxer.Close()
	}()

	// Get codec data from the file
	codecData := demuxer.Streams()

	if len(codecData) == 0 {
		return errors.Errorf("No streams found in file '%s'", filePath)
	}

	if streamVerboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_CODEC_MET).Str("stream_id", streamID.String()).Str("file_path", filePath).Int("codec_count", len(codecData)).Msg("Found codecs in file")
	}

	// Add codecs to stream storage
	err = app.Streams.AddCodecForStream(streamID, codecData)
	if err != nil {
		return errors.Wrapf(err, "Can't update codec data for stream %s", streamID)
	}

	// Update stream status
	err = app.Streams.UpdateStreamStatus(streamID, true)
	if err != nil {
		return errors.Wrapf(err, "Can't update status for stream %s", streamID)
	}

	// Check if audio-only
	isAudioOnly := false
	if len(codecData) == 1 && codecData[0].Type().IsAudio() {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_AUDIO_MET).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("File contains only audio")
		}
		isAudioOnly = true
	}

	// Start HLS casting if enabled
	if hlsEnabled {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_HLS_CAST).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Starting HLS casting")
		}
		err = app.startHlsCast(streamID, stopHlsCast)
		if err != nil {
			if streamVerboseLevel > VERBOSE_NONE {
				log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_HLS_CAST).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Can't start HLS casting")
			}
		}
	}

	// Start MP4 archive casting if enabled
	if archiveEnabled {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Starting MP4 archive casting")
		}
		archive := app.Streams.GetStreamArchiveStorage(streamID)
		if archive == nil {
			log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Empty archive configuration")
		} else {
			err = app.startMP4Cast(archive, streamID, stopMP4Cast, errorSignal, streamVerboseLevel)
			if err != nil {
				if streamVerboseLevel > VERBOSE_NONE {
					log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Can't start MP4 archive casting")
				}
			}
		}
	}

	// Playback loop
	playbackLoop := true

	// Track cumulative time offset for looping (MSE needs continuous timestamps)
	var timeOffset time.Duration
	var lastPacketTime time.Duration

	// Track real-time pacing across entire playback (not per-loop)
	var globalStartTime time.Time
	globalFirstPacket := true

	for playbackLoop {
		// Reset demuxer to start for looping
		demuxer.SeekToStart()

		pingStream := time.NewTimer(pingDuration)

		// Read and cast packets
	packetLoop:
		for {
			select {
			case <-pingStream.C:
				log.Error().Err(ErrStreamHasNoVideo).Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_EXIT_SIGNAL).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("Stream has no video (timeout)")
				if hlsEnabled {
					stopHlsCast <- STOP_SIGNAL_NO_VIDEO
				}
				if archiveEnabled {
					stopMP4Cast <- STOP_SIGNAL_NO_VIDEO
				}
				return errors.Wrapf(ErrStreamHasNoVideo, "File is '%s'", filePath)

			case errS := <-errorSignal:
				return errors.Wrapf(errS, "Received error signal from MP4 casting")

			default:
				// Read next packet from file
				packet, err := demuxer.ReadPacket()
				if err != nil {
					if err == io.EOF {
						if streamVerboseLevel > VERBOSE_NONE {
							log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_EOF).Str("stream_id", streamID.String()).Str("file_path", filePath).Bool("loop", loop).Msg("End of file reached")
						}
						if !loop {
							playbackLoop = false
						}
						break packetLoop
					}
					return errors.Wrapf(err, "Error reading packet from file '%s'", filePath)
				}

				// Apply time offset for looping (makes timestamps continuous)
				originalTime := packet.Time
				packet.Time = packet.Time + timeOffset
				lastPacketTime = packet.Time

				// Real-time pacing: wait until it's time to deliver this packet
				if globalFirstPacket {
					globalStartTime = time.Now()
					globalFirstPacket = false
				} else {
					// Calculate how long we should wait based on packet timestamp
					elapsed := time.Since(globalStartTime)
					waitTime := packet.Time - elapsed

					if waitTime > 0 {
						time.Sleep(waitTime)
					}
				}

				// Reset ping timer on keyframes
				if isAudioOnly || packet.IsKeyFrame {
					if streamVerboseLevel > VERBOSE_ADD {
						log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("file_path", filePath).Bool("is_keyframe", packet.IsKeyFrame).Msg("Resetting ping timer")
					}
					pingStream.Reset(pingDurationRestart)
				}

				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("file_path", filePath).Bool("is_keyframe", packet.IsKeyFrame).Dur("original_time", originalTime).Dur("adjusted_time", packet.Time).Msg("Casting packet")
				}

				// Cast packet to all outputs
				err = app.Streams.CastPacket(streamID, packet, hlsEnabled, archiveEnabled)
				if err != nil {
					if hlsEnabled {
						stopHlsCast <- STOP_SIGNAL_ERR
					}
					if archiveEnabled {
						stopMP4Cast <- STOP_SIGNAL_ERR
					}
					errStatus := app.Streams.UpdateStreamStatus(streamID, false)
					if errStatus != nil {
						return errors.Wrapf(err, "Can't update status for stream %s after casting", streamID)
					}
					return errors.Wrapf(err, "Can't cast packet %s (%s)", streamID, filePath)
				}
			}
		}

		// Update time offset for next loop iteration
		if loop {
			// Add small gap to ensure clean transition
			timeOffset = lastPacketTime + 100*time.Millisecond
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_LOOP).Str("stream_id", streamID.String()).Str("file_path", filePath).Dur("new_time_offset", timeOffset).Msg("Restarting playback (loop enabled)")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Update status when done
	err = app.Streams.UpdateStreamStatus(streamID, false)
	if err != nil {
		return errors.Wrapf(err, "Can't update status for stream %s after file playback ended", streamID)
	}

	if streamVerboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_COMPLETE).Str("stream_id", streamID.String()).Str("file_path", filePath).Msg("File playback completed")
	}

	return nil
}
