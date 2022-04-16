package videoserver

import (
	"fmt"
	"time"

	"github.com/deepch/vdk/format/rtspv2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// runStream runs RTSP grabbing process
func (app *Application) runStream(streamID uuid.UUID, url string, hlsEnabled bool) error {
	session, err := rtspv2.Dial(rtspv2.RTSPClientOptions{
		URL:              url,
		DisableAudio:     true,
		DialTimeout:      3 * time.Second,
		ReadWriteTimeout: 3 * time.Second,
		Debug:            false,
	})
	if err != nil {
		return errors.Wrapf(err, "Can't connect to stream '%s'", url)
	}
	defer session.Close()
	if session.CodecData != nil {
		app.codecAdd(streamID, session.CodecData)
		err = app.updateStreamStatus(streamID, true)
		if err != nil {
			return errors.Wrapf(err, "Can't update status for stream %s", streamID)
		}
	}
	isAudioOnly := false
	if len(session.CodecData) == 1 {
		if session.CodecData[0].Type().IsAudio() {
			isAudioOnly = true
		}
	}
	var stopHlsCast chan bool
	if hlsEnabled {
		fmt.Println("HLS enabled")
		stopHlsCast = make(chan bool, 1)
		app.startHlsCast(streamID, stopHlsCast)
	}
	pingStream := time.NewTimer(15 * time.Second)
	for {
		select {
		case <-pingStream.C:
			return errors.Wrapf(ErrStreamHasNoVideo, "URL is '%s'", url)
		case signals := <-session.Signals:
			switch signals {
			case rtspv2.SignalCodecUpdate:
				app.codecAdd(streamID, session.CodecData)
				err = app.updateStreamStatus(streamID, true)
				if err != nil {
					return errors.Wrapf(err, "Can't update status for stream %s", streamID)
				}
			case rtspv2.SignalStreamRTPStop:
				_ = app.updateStreamStatus(streamID, false)
				return errors.Wrapf(ErrStreamDistonnected, "URL is '%s'", url)
			}
		case packetAV := <-session.OutgoingPacketQueue:
			if isAudioOnly || packetAV.IsKeyFrame {
				pingStream.Reset(16 * time.Second)
			}
			err = app.cast(streamID, *packetAV, hlsEnabled)
			if err != nil {
				if hlsEnabled {
					stopHlsCast <- true
				}
				_ = app.updateStreamStatus(streamID, false)
				return fmt.Errorf("Can't cast packet %s (%s): %s\n", streamID, url, err.Error())
			}
		}
	}
}
