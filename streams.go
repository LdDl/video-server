package videoserver

import (
	"fmt"
	"log"
	"time"

	"github.com/LdDl/video-server/internal/hlserror"
	"github.com/deepch/vdk/format/rtsp"
	"github.com/google/uuid"
)

// StartStreams Start video streams
func (app *Application) StartStreams() {

	for _, k := range app.Streams.getKeys() {
		app.Streams.Lock()
		url := app.Streams.Streams[k].URL
		supportedTypes := app.Streams.Streams[k].SupportedStreamTypes
		app.Streams.Unlock()

		hlsEnabled := typeExists("hls", supportedTypes)

		go func(name uuid.UUID, hlsEnabled bool, url string) {
			for {
				log.Printf("Stream must be establishment for '%s' by connecting to %s", name, url)
				rtsp.DebugRtsp = false
				session, err := rtsp.Dial(url)
				if err != nil {
					hlserror.SetError(name, 502, fmt.Errorf("rtsp.Dial error for %s (%s): %s", name, url, err.Error()))
					log.Printf("rtsp.Dial error for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
				session.RtpKeepAliveTimeout = time.Duration(10 * time.Second)
				codec, err := session.Streams()
				if err != nil {
					hlserror.SetError(name, 520, fmt.Errorf("Can't get sessions for %s (%s): %s", name, url, err.Error()))
					log.Printf("Can't get sessions for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
				app.codecAdd(name, codec)
				err = app.updateStatus(name, true)
				if err != nil {
					log.Printf("Can't update status 'true' for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}

				if hlsEnabled {
					stopHlsCast := make(chan bool, 1)
					app.startHlsCast(name, stopHlsCast)
					for {
						pkt, err := session.ReadPacket()
						if err != nil {
							hlserror.SetError(name, 500, fmt.Errorf("Can't read session's packet %s (%s): %s", name, url, err.Error()))
							log.Printf("Can't read session's packet %s (%s): %s\n", name, url, err.Error())
							stopHlsCast <- true
							break
						}
						err = app.cast(name, pkt)
						if err != nil {
							hlserror.SetError(name, 500, fmt.Errorf("Can't cast packet %s (%s): %s", name, url, err.Error()))
							log.Printf("Can't cast packet %s (%s): %s\n", name, url, err.Error())
							stopHlsCast <- true
							break
						}
					}
				} else {
					for {
						pkt, err := session.ReadPacket()
						if err != nil {
							log.Printf("Can't read session's packet %s (%s): %s\n", name, url, err.Error())
							break
						}
						err = app.castMSE(name, pkt)
						if err != nil {
							log.Printf("Can't cast packet %s (%s): %s\n", name, url, err.Error())
							break
						}
					}
				}

				session.Close()
				err = app.updateStatus(name, false)
				if err != nil {
					log.Printf("Can't update status 'false' for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
				log.Printf("Stream must be re-establishment for '%s' by connecting to %s in next 5 seconds\n", name, url)
				time.Sleep(5 * time.Second)
			}
		}(k, hlsEnabled, url)
	}
}

func typeExists(typeName string, typesNames []string) bool {
	for i := range typesNames {
		if typesNames[i] == typeName {
			return true
		}
	}
	return false
}

// StartStream Start video stream
func (app *Application) StartStream(k uuid.UUID) {
	app.Streams.Lock()
	url := app.Streams.Streams[k].URL
	supportedTypes := app.Streams.Streams[k].SupportedStreamTypes
	app.Streams.Unlock()

	hlsEnabled := typeExists("hls", supportedTypes)

	go func(name uuid.UUID, hlsEnabled bool, url string) {
		for {
			log.Printf("Stream must be establishment for '%s' by connecting to %s", name, url)
			rtsp.DebugRtsp = false
			session, err := rtsp.Dial(url)
			if err != nil {
				hlserror.SetError(name, 502, fmt.Errorf("rtsp.Dial error for %s (%s): %s", name, url, err.Error()))
				log.Printf("rtsp.Dial error for %s (%s): %s\n", name, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}
			session.RtpKeepAliveTimeout = time.Duration(10 * time.Second)
			codec, err := session.Streams()
			if err != nil {
				hlserror.SetError(name, 520, fmt.Errorf("Can't get sessions for %s (%s): %s", name, url, err.Error()))
				log.Printf("Can't get sessions for %s (%s): %s\n", name, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}
			app.codecAdd(name, codec)
			err = app.updateStatus(name, true)
			if err != nil {
				log.Printf("Can't update status 'true' for %s (%s): %s\n", name, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}

			if hlsEnabled {
				stopHlsCast := make(chan bool, 1)
				app.startHlsCast(name, stopHlsCast)
				for {
					pkt, err := session.ReadPacket()
					if err != nil {
						hlserror.SetError(name, 500, fmt.Errorf("Can't read session's packet %s (%s): %s", name, url, err.Error()))
						log.Printf("Can't read session's packet %s (%s): %s\n", name, url, err.Error())
						stopHlsCast <- true
						break
					}
					err = app.cast(name, pkt)
					if err != nil {
						hlserror.SetError(name, 500, fmt.Errorf("Can't cast packet %s (%s): %s", name, url, err.Error()))
						log.Printf("Can't cast packet %s (%s): %s\n", name, url, err.Error())
						stopHlsCast <- true
						break
					}
				}
			} else {
				for {
					pkt, err := session.ReadPacket()
					if err != nil {
						log.Printf("Can't read session's packet %s (%s): %s\n", name, url, err.Error())
						break
					}
					err = app.castMSE(name, pkt)
					if err != nil {
						log.Printf("Can't cast packet %s (%s): %s\n", name, url, err.Error())
						break
					}
				}
			}

			session.Close()
			err = app.updateStatus(name, false)
			if err != nil {
				log.Printf("Can't update status 'false' for %s (%s): %s\n", name, url, err.Error())
				time.Sleep(60 * time.Second)
				continue
			}
			log.Printf("Stream must be re-establishment for '%s' by connecting to %s in next 5 seconds\n", name, url)
			time.Sleep(5 * time.Second)
		}
	}(k, hlsEnabled, url)
}
