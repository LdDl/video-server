package videoserver

import (
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/morozka/vdk/format/rtsp"
)

// StartStreams Start video streams
func StartStreams(cfg *AppConfiguration) {
	for k, v := range cfg.Streams.Streams {
		go func(name uuid.UUID, url string) {
			for {
				log.Printf("Stream must be establishment for '%s' by connecting to %s\n", name, url)
				rtsp.DebugRtsp = false
				session, err := rtsp.Dial(url)
				if err != nil {
					log.Printf("rtsp.Dial error for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
				session.RtpKeepAliveTimeout = time.Duration(10 * time.Second)
				codec, err := session.Streams()
				if err != nil {
					log.Printf("Can't get sessions for %s (%s): %s\n", name, url, err.Error())
					time.Sleep(60 * time.Second)
					continue
				}
				cfg.codecAdd(name, codec)
				cfg.updateStatus(name, true)
				stopHlsCast := make(chan bool, 1)
				cfg.startHlsCast(name, stopHlsCast)
				for {
					pkt, err := session.ReadPacket()
					if err != nil {
						log.Printf("Can't reade session's packet %s (%s): %s\n", name, url, err.Error())
						stopHlsCast <- true
						break
					}
					cfg.cast(name, pkt)
				}
				session.Close()
				cfg.updateStatus(name, false)
				log.Printf("Stream must be re-establishment for '%s' by connecting to %s in next 5 seconds\n", name, url)
				time.Sleep(5 * time.Second)
			}
		}(k, v.URL)
	}
}
