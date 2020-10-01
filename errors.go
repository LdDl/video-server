package videoserver

import (
	"fmt"
)

var (
	// ErrStreamNotFound When map of streams doesn't contain requested key
	ErrStreamNotFound = fmt.Errorf("Stream not found for provided ID")
)
