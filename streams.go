package videoserver

import (
	"log"
	"time"

	"github.com/google/uuid"
)

const (
	restartStreamDuration = 5 * time.Second
)

// StartStreams starts all video streams
func (app *Application) StartStreams() {
	streamsIDs := app.Streams.getKeys()
	for _, k := range streamsIDs {
		app.StartStream(k)
	}
}

// StartStream starts single video stream
func (app *Application) StartStream(k uuid.UUID) {
	app.Streams.Lock()
	url := app.Streams.Streams[k].URL
	supportedTypes := app.Streams.Streams[k].SupportedOutputTypes
	app.Streams.Unlock()

	hlsEnabled := typeExists(STREAM_TYPE_HLS, supportedTypes)
	go app.startLoop(k, url, hlsEnabled)
}

// startLoop starts stream loop with dialing to certain RTSP
func (app *Application) startLoop(streamID uuid.UUID, url string, hlsEnabled bool) {
	for {
		log.Printf("Stream must be establishment for '%s' by connecting to %s", streamID, url)
		err := app.runStream(streamID, url, hlsEnabled)
		if err != nil {
			log.Printf("Error occured for stream %s on URL '%s': %s", streamID, url, err.Error())
		}
		log.Printf("Stream must be re-establishment for '%s' by connecting to %s in %s\n", streamID, url, restartStreamDuration)
		time.Sleep(restartStreamDuration)
	}
}

// typeExists checks if a type exists in a types list
func typeExists(typ StreamType, types []StreamType) bool {
	for i := range types {
		if types[i] == typ {
			return true
		}
	}
	return false
}
