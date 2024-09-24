package videoserver

import (
	"time"

	"github.com/deepch/vdk/format/rtspv2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const (
	pingDuration        = 15 * time.Second
	pingDurationRestart = pingDuration + 1*time.Second
	dialTimeoutDuration = 33 * time.Second
	readTimeoutDuration = 33 * time.Second
)

// runStream runs RTSP grabbing process
func (app *Application) runStream(streamID uuid.UUID, url string, hlsEnabled, archiveEnabled bool, streamVerboseLevel VerboseLevel) error {
	if streamVerboseLevel > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_DIAL).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Trying to dial")
	}
	session, err := rtspv2.Dial(rtspv2.RTSPClientOptions{
		URL:              url,
		DisableAudio:     true,
		DialTimeout:      dialTimeoutDuration,
		ReadWriteTimeout: readTimeoutDuration,
		Debug:            false,
	})
	if err != nil {
		return errors.Wrapf(err, "Can't connect to stream '%s'", url)
	}
	defer session.Close()
	if len(session.CodecData) != 0 {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_CODEC_MET).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Any("codec_data", session.CodecData).Msg("Found codec. Adding this one")
		}
		app.Streams.AddCodecForStream(streamID, session.CodecData)
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_STATUS_UPDATE).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Update stream status")
		}
		err = app.Streams.UpdateStreamStatus(streamID, true)
		if err != nil {
			return errors.Wrapf(err, "Can't update status for stream %s on empty codecs", streamID)
		}
	}

	isAudioOnly := false
	if len(session.CodecData) == 1 {
		if session.CodecData[0].Type().IsAudio() {
			if streamVerboseLevel > VERBOSE_NONE {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_AUDIO_MET).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Only audio")
			}
			isAudioOnly = true
		}
	}

	var stopHlsCast chan bool
	if hlsEnabled {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_HLS_CAST).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Need to start casting for HLS")
		}
		stopHlsCast = make(chan bool, 1)
		err = app.startHlsCast(streamID, stopHlsCast)
		if err != nil {
			if streamVerboseLevel > VERBOSE_NONE {
				log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_HLS_CAST).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Can't start HLS casting")
			}
		}
	}

	var stopMP4Cast chan bool
	if archiveEnabled {
		if streamVerboseLevel > VERBOSE_NONE {
			log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Need to start casting to MP4 archive")
		}
		archive := app.Streams.GetStreamArchiveStorage(streamID)
		if archive == nil {
			log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Empty archive configuration for the given stream")
		} else {
			stopMP4Cast = make(chan bool, 1)
			err = app.startMP4Cast(archive, streamID, stopMP4Cast, streamVerboseLevel)
			if err != nil {
				if streamVerboseLevel > VERBOSE_NONE {
					log.Warn().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_MP4_CAST).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Can't start MP4 archive process")
				}
			}
		}
	}

	pingStream := time.NewTimer(pingDuration)
	for {
		select {
		case <-pingStream.C:
			log.Error().Err(ErrStreamHasNoVideo).Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_EXIT_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Stream has no video")
			return errors.Wrapf(ErrStreamHasNoVideo, "URL is '%s'", url)
		case signals := <-session.Signals:
			switch signals {
			case rtspv2.SignalCodecUpdate:
				if streamVerboseLevel > VERBOSE_NONE {
					log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_CODEC_UPDATE_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Any("codec_data", session.CodecData).Msg("Recieved update codec signal")
				}
				app.Streams.AddCodecForStream(streamID, session.CodecData)
				err = app.Streams.UpdateStreamStatus(streamID, true)
				if err != nil {
					return errors.Wrapf(err, "Can't update status for stream %s after codecs update", streamID)
				}
			case rtspv2.SignalStreamRTPStop:
				if streamVerboseLevel > VERBOSE_NONE {
					log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_STOP_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Recieved stop signal")
				}
				err = app.Streams.UpdateStreamStatus(streamID, false)
				if err != nil {
					return errors.Wrapf(err, "Can't update status for stream %s after RTP stops", streamID)
				}
				return errors.Wrapf(ErrStreamDisconnected, "URL is '%s'", url)
			}
		case packetAV := <-session.OutgoingPacketQueue:
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Recieved outgoing packet from queue")
			}
			if isAudioOnly || packetAV.IsKeyFrame {
				if streamVerboseLevel > VERBOSE_ADD {
					log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to reset ping for stream")
				}
				pingStream.Reset(pingDurationRestart)
			}
			if streamVerboseLevel > VERBOSE_ADD {
				log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Casting packet")
			}
			err = app.Streams.CastPacket(streamID, *packetAV, hlsEnabled, archiveEnabled)
			if err != nil {
				if hlsEnabled {
					if streamVerboseLevel > VERBOSE_NONE {
						log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to stop HLS cast")
					}
					stopHlsCast <- true
				}
				if archiveEnabled {
					if streamVerboseLevel > VERBOSE_NONE {
						log.Info().Str("scope", SCOPE_STREAMING).Str("event", EVENT_STREAMING_PACKET_SIGNAL).Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to stop MP4 cast")
					}
					stopMP4Cast <- true
				}
				errStatus := app.Streams.UpdateStreamStatus(streamID, false)
				if errStatus != nil {
					return errors.Wrapf(err, "Can't update status for stream %s after casting", streamID)
				}
				return errors.Wrapf(err, "Can't cast packet %s (%s)", streamID, url)
			}
		}
	}
}
