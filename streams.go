package videoserver

import (
	"log"
	"time"

	"github.com/google/uuid"
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
	supportedTypes := app.Streams.Streams[k].SupportedStreamTypes
	app.Streams.Unlock()

	hlsEnabled := typeExists("hls", supportedTypes)
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
		log.Printf("Stream must be re-establishment for '%s' by connecting to %s in next 5 seconds\n", streamID, url)
		time.Sleep(5 * time.Second)
	}
}

// typeExists checks if a type exists in a types list
func typeExists(typeName string, typesNames []string) bool {
	for i := range typesNames {
		if typesNames[i] == typeName {
			return true
		}
	}
	return false
}
