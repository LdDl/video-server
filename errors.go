package videoserver

import (
	"fmt"
)

var (
	// ErrStreamNotFound When map of streams doesn't contain requested key
	ErrStreamNotFound     = fmt.Errorf("Stream not found for provided ID")
	ErrStreamHasNoVideo   = fmt.Errorf("Stream has no video")
	ErrStreamDistonnected = fmt.Errorf("Disconnected")
)
