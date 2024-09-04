package videoserver

import (
	"fmt"
)

var (
	ErrStreamNotFound     = fmt.Errorf("stream not found for provided ID")
	ErrStreamHasNoVideo   = fmt.Errorf("stream has no video")
	ErrStreamDisconnected = fmt.Errorf("disconnected")
)
