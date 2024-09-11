package videoserver

import (
	"fmt"
)

var (
	ErrStreamNotFound         = fmt.Errorf("stream not found for provided ID")
	ErrStreamHasNoVideo       = fmt.Errorf("stream has no video")
	ErrStreamDisconnected     = fmt.Errorf("disconnected")
	ErrStreamTypeNotExists    = fmt.Errorf("stream type does not exists")
	ErrStreamTypeNotSupported = fmt.Errorf("stream type is not supported")
	ErrNotSupportedStorage    = fmt.Errorf("not supported storage")
)
