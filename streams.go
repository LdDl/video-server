package videoserver

import (
	"fmt"
	"log"
	"time"

	"github.com/LdDl/video-server/internal/hlserror"
	"github.com/deepch/vdk/format/rtsp"
	"github.com/deepch/vdk/format/rtspv2"
	"github.com/google/uuid"
)

// StartStreams Starts all video streams
func (app *Application) StartStreams() {
	rtsp.DebugRtsp = false
	keys := app.Streams.getKeys()
	for _, streamID := range keys {
		app.StartStream(streamID)
	}
}

// StartStream Starts single video stream
func (app *Application) StartStream(k uuid.UUID) {
	app.Streams.Lock()
	url := app.Streams.Streams[k].URL
	supportedTypes := app.Streams.Streams[k].SupportedStreamTypes
	app.Streams.Unlock()

	hlsEnabled := typeExists("hls", supportedTypes)

	go func(streamID uuid.UUID, hlsEnabled bool, url string) {
		for {
			log.Printf("Stream must be establishment for '%s' by connecting to %s", streamID, url)
			rtspClient, err := rtspv2.Dial(rtspv2.RTSPClientOptions{URL: url, DisableAudio: true, DialTimeout: 3 * time.Second, ReadWriteTimeout: 3 * time.Second, Debug: false})
			if err != nil {
				log.Printf("rtsp.Dial error for %s (%s): %s\n", streamID, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}
			// defer rtspClient.Close()
			if rtspClient.CodecData != nil {
				app.codecAdd(streamID, rtspClient.CodecData)
				err = app.updateStatus(streamID, true)
				if err != nil {
					log.Printf("Can't update status 'true' for %s (%s): %s\n", streamID, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
			}

			// isAudioOnly := false
			// if len(rtspClient.CodecData) == 1 && rtspClient.CodecData[0].Type().IsAudio() {
			// 	isAudioOnly = true
			// }

			if hlsEnabled {
				stopHlsCast := make(chan bool, 1)
				app.startHlsCast(streamID, stopHlsCast)
				for {
					err := app.ReadAVPacket(rtspClient, streamID, true)
					if err != nil {
						hlserror.SetError(streamID, 500, fmt.Errorf("Can't read session's packet %s (%s): %s", streamID, url, err.Error()))
						log.Printf("Can't read session's packet %s (%s): %s\n", streamID, url, err.Error())
						stopHlsCast <- true
						break
					}
				}
			} else {
				for {
					err := app.ReadAVPacket(rtspClient, streamID, false)
					if err != nil {
						log.Printf("Can't read session's packet %s (%s): %s\n", streamID, url, err.Error())
						break
					}
				}
			}
			rtspClient.Close()
			err = app.updateStatus(streamID, false)
			if err != nil {
				log.Printf("Can't update status 'false' for %s (%s): %s\n", streamID, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}
			log.Printf("Stream must be re-establishment for '%s' by connecting to %s in next 5 seconds\n", streamID, url)
			time.Sleep(5 * time.Second)
		}
	}(k, hlsEnabled, url)
}

// CloseStreams Stops all video stream
func (app *Application) CloseStreams() {
	keys := app.Streams.getKeys()
	for _, streamID := range keys {
		app.CloseStream(streamID)
	}
}

// CloseStream Stops single video stream
func (app *Application) CloseStream(k uuid.UUID) {
	app.Streams.Lock()
	delete(app.Streams.Streams, k)
	app.Streams.Unlock()
}

func typeExists(typeName string, typesNames []string) bool {
	for i := range typesNames {
		if typesNames[i] == typeName {
			return true
		}
	}
	return false
}

func (app *Application) ReadAVPacket(session *rtspv2.RTSPClient, streamID uuid.UUID, hlsEnabled bool) error {
	for {
		select {
		case signals := <-session.Signals:
			switch signals {
			case rtspv2.SignalCodecUpdate:
				app.codecAdd(streamID, session.CodecData)
				err := app.updateStatus(streamID, true)
				if err != nil {
					log.Printf("[SignalCodecUpdate] Can't set status to value 'true' for stream = %s due the error %s\n", streamID, err.Error())
				}
			case rtspv2.SignalStreamRTPStop:
				err := app.updateStatus(streamID, false)
				if err != nil {
					log.Printf("[SignalStreamRTPStop] Can't set status to value 'true' for stream = %s due the error %s\n", streamID, err.Error())
				}
				return err
			}
		case packetAV := <-session.OutgoingPacketQueue:
			if !hlsEnabled {
				err := app.castMSEonly(streamID, *packetAV)
				if err != nil {
					log.Printf("[OutgoingPacketQueue] Can't execute casting for stream = %s [MSE only] due the error: %s\n", streamID, err.Error())
				}
			} else {
				err := app.cast(streamID, *packetAV)
				if err != nil {
					log.Printf("[OutgoingPacketQueue] Can't execute casting for stream = %s [both MSE and HLS] due the error: %s\n", streamID, err.Error())
				}
			}
		}
	}
}
