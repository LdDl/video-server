package videoserver

import (
	"github.com/deepch/vdk/av"
)

type viewer struct {
	c chan av.Packet
}
