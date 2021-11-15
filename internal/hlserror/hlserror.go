package hlserror

import (
	"sync"

	"github.com/google/uuid"
)

// @todo: redudant files
var hem = hlserrorMap{status: make(map[uuid.UUID]hlserror)}

type hlserrorMap struct {
	sync.RWMutex
	status map[uuid.UUID]hlserror
}
type hlserror struct {
	code int
	err  error
}

func SetError(stream uuid.UUID, code int, err error) {
	hem.Lock()
	hem.status[stream] = hlserror{code, err}
	hem.Unlock()
}
func GetError(stream uuid.UUID) (code int, err error) {
	hem.RLock()
	herr, ok := hem.status[stream]
	hem.RUnlock()
	if ok {
		return herr.code, herr.err
	}
	return 200, nil
}
