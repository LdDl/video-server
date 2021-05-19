package videoserver

import (
	"fmt"
)

var (
	// When map of streams doesn't contain requested key
	ErrStreamNotFound = fmt.Errorf("Stream not found for provided ID")
	// When stream has no video data
	ErrorNoVideoOnStream = fmt.Errorf("Stream doesn't have video")
)
