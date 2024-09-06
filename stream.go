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
func (app *Application) runStream(streamID uuid.UUID, url string, hlsEnabled, archiveEnabled bool) error {
	log.Info().Str("scope", "streaming").Str("event", "stream_dialing").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Trying to dial")
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
		log.Info().Str("scope", "streaming").Str("event", "stream_codec_met").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Any("codec_data", session.CodecData).Msg("Found codec. Adding this one")
		app.addCodec(streamID, session.CodecData)
		log.Info().Str("scope", "streaming").Str("event", "stream_status_update").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Update stream status")
		err = app.updateStreamStatus(streamID, true)
		if err != nil {
			return errors.Wrapf(err, "Can't update status for stream %s", streamID)
		}
	}

	isAudioOnly := false
	if len(session.CodecData) == 1 {
		if session.CodecData[0].Type().IsAudio() {
			log.Info().Str("scope", "streaming").Str("event", "stream_audio_met").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("hls_enabled", hlsEnabled).Msg("Only audio")
			isAudioOnly = true
		}
	}

	var stopHlsCast chan bool
	if hlsEnabled {
		log.Info().Str("scope", "streaming").Str("event", "stream_hls_req").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Need to start casting for HLS")
		stopHlsCast = make(chan bool, 1)
		err = app.startHlsCast(streamID, stopHlsCast)
		if err != nil {
			log.Warn().Str("scope", "streaming").Str("event", "stream_hls_req").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Can't start HLS casting")
		}
	}

	var stopMP4Cast chan bool
	if archiveEnabled {
		log.Info().Str("scope", "streaming").Str("event", "stream_mp4_req").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Need to start casting to MP4 archive")
		archive := app.getStreamArchive(streamID)
		if archive == nil {
			log.Warn().Str("scope", "streaming").Str("event", "stream_mp4_req").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Empty archive configuration for the given stream")
		} else {
			stopMP4Cast = make(chan bool, 1)
			err = app.startMP4Cast(streamID, stopMP4Cast)
			if err != nil {
				log.Warn().Str("scope", "streaming").Str("event", "stream_mp4_req").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Can't start MP4 archive process")
			}
		}
	}

	pingStream := time.NewTimer(pingDuration)
	for {
		select {
		case <-pingStream.C:
			log.Error().Err(ErrStreamHasNoVideo).Str("scope", "streaming").Str("event", "stream_exit_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Stream has no video")
			return errors.Wrapf(ErrStreamHasNoVideo, "URL is '%s'", url)
		case signals := <-session.Signals:
			switch signals {
			case rtspv2.SignalCodecUpdate:
				log.Info().Str("scope", "streaming").Str("event", "stream_codec_update_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Any("codec_data", session.CodecData).Msg("Recieved update codec signal")
				app.addCodec(streamID, session.CodecData)
				err = app.updateStreamStatus(streamID, true)
				if err != nil {
					return errors.Wrapf(err, "Can't update status for stream %s", streamID)
				}
			case rtspv2.SignalStreamRTPStop:
				log.Info().Str("scope", "streaming").Str("event", "stream_stop_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Recieved stop signal")
				err = app.updateStreamStatus(streamID, false)
				if err != nil {
					errors.Wrapf(err, "Can't switch status to False for stream '%s'", url)
				}
				return errors.Wrapf(ErrStreamDisconnected, "URL is '%s'", url)
			}
		case packetAV := <-session.OutgoingPacketQueue:
			// log.Info().Str("scope", "streaming").Str("event", "stream_packet_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Msg("Recieved outgoing packet from queue")
			if isAudioOnly || packetAV.IsKeyFrame {
				log.Info().Str("scope", "streaming").Str("event", "stream_packet_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to reset ping for stream")
				pingStream.Reset(pingDurationRestart)
			}
			// log.Info().Str("scope", "streaming").Str("event", "stream_packet_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Casting packet")
			err = app.cast(streamID, *packetAV, hlsEnabled)
			if err != nil {
				if hlsEnabled {
					log.Info().Str("scope", "streaming").Str("event", "stream_packet_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to stop HLS cast")
					stopHlsCast <- true
				}
				if archiveEnabled {
					log.Info().Str("scope", "streaming").Str("event", "stream_packet_signal").Str("stream_id", streamID.String()).Str("stream_url", url).Bool("only_audio", isAudioOnly).Bool("is_keyframe", packetAV.IsKeyFrame).Msg("Need to stop MP4 cast")
					stopMP4Cast <- true
				}
				errStatus := app.updateStreamStatus(streamID, false)
				if errStatus != nil {
					errors.Wrapf(errors.Wrapf(err, "Can't cast packet %s (%s)", streamID, url), "Can't switch status to False for stream '%s'", url)
				}
				return errors.Wrapf(err, "Can't cast packet %s (%s)", streamID, url)
			}
		}
	}
}
