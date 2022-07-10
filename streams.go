package videoserver

import (
	"context"
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
	go app.RunStream(context.Background(), k)
}

func (app *Application) RunStream(ctx context.Context, k uuid.UUID) {
	url, supportedTypes := app.Streams.GetStream(k)
	hlsEnabled := typeExists("hls", supportedTypes)

	app.startLoop(ctx, k, url, hlsEnabled)
}

// startLoop starts stream loop with dialing to certain RTSP
func (app *Application) startLoop(ctx context.Context, streamID uuid.UUID, url string, hlsEnabled bool) {
	select {
	case <-ctx.Done():
		return
	default:
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
func typeExists(typeName string, typesNames []string) bool {
	for i := range typesNames {
		if typesNames[i] == typeName {
			return true
		}
	}
	return false
}
