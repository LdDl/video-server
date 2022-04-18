package videoserver

import (
	"github.com/deepch/vdk/av"
)

// viewer is just an wrapping alias to chan for av.Packet
type viewer struct {
	c chan av.Packet
}
