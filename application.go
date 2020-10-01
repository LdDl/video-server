package videoserver

import (
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/morozka/vdk/av"
)

// Application Configuration parameters for application
type Application struct {
	Server          *ServerInfo `json:"server"`
	Streams         StreamsMap  `json:"streams"`
	HlsMsPerSegment int64       `json:"hls_ms_per_segment"`
	HlsDirectory    string      `json:"hls_directory"`
	HlsWindowSize   uint        `json:"hls_window_size"`
	HlsCapacity     uint        `json:"hls_window_capacity"`
}

// ServerInfo Information about server
type ServerInfo struct {
	HTTPAddr string `json:"http_addr"`
	HTTPPort int    `json:"http_port"`
}

// StreamsMap Map wrapper for map[uuid.UUID]*StreamConfiguration with mutex for concurrent usage
type StreamsMap struct {
	sync.Mutex
	Streams map[uuid.UUID]*StreamConfiguration
}

func (sm *StreamsMap) getKeys() []uuid.UUID {
	sm.Lock()
	defer sm.Unlock()
	keys := make([]uuid.UUID, 0, len(sm.Streams))
	for k := range sm.Streams {
		keys = append(keys, k)
	}
	return keys
}

// StreamConfiguration Configuration parameters for stream
type StreamConfiguration struct {
	URL       string `json:"url"`
	Status    bool   `json:"status"`
	Codecs    []av.CodecData
	Clients   map[uuid.UUID]viewer
	hlsChanel chan av.Packet
}

type viewer struct {
	c chan av.Packet
}

// NewApplication Prepare configuration for application
func NewApplication(cfg *ConfigurationArgs) (*Application, error) {
	tmp := Application{
		Server: &ServerInfo{
			HTTPAddr: cfg.Server.HTTPAddr,
			HTTPPort: cfg.Server.HTTPPort,
		},
		Streams:         StreamsMap{Streams: make(map[uuid.UUID]*StreamConfiguration)},
		HlsMsPerSegment: cfg.HlsMsPerSegment,
		HlsDirectory:    cfg.HlsDirectory,
		HlsWindowSize:   cfg.HlsWindowSize,
		HlsCapacity:     cfg.HlsCapacity,
	}
	for i := range cfg.Streams {
		validUUID, err := uuid.Parse(cfg.Streams[i].GUID)
		if err != nil {
			log.Printf("Not valid UUID: %s\n", cfg.Streams[i].GUID)
			continue
		}
		tmp.Streams.Streams[validUUID] = &StreamConfiguration{
			URL:       cfg.Streams[i].URL,
			Clients:   make(map[uuid.UUID]viewer),
			hlsChanel: make(chan av.Packet, 100),
		}
	}
	return &tmp, nil
}

func (element *Application) cast(id uuid.UUID, pck av.Packet) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	curStream, _ := element.Streams.Streams[id]
	curStream.hlsChanel <- pck
	for _, v := range curStream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *Application) ext(streamID uuid.UUID) bool {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	_, ok := element.Streams.Streams[streamID]
	return ok
}

func (element *Application) codecAdd(streamID uuid.UUID, codecs []av.CodecData) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	element.Streams.Streams[streamID].Codecs = codecs
}

func (element *Application) codecGet(streamID uuid.UUID) []av.CodecData {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	curStream, _ := element.Streams.Streams[streamID]
	return curStream.Codecs
}

func (element *Application) updateStatus(streamID uuid.UUID, status bool) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	t, _ := element.Streams.Streams[streamID]
	t.Status = status
	element.Streams.Streams[streamID] = t
}

func (element *Application) clientAdd(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	clientID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	ch := make(chan av.Packet, 100)
	curStream, _ := element.Streams.Streams[streamID]
	curStream.Clients[clientID] = viewer{c: ch}
	return clientID, ch, nil
}

func (element *Application) clientDelete(streamID, clientID uuid.UUID) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	delete(element.Streams.Streams[streamID].Clients, clientID)
}

func (element *Application) startHlsCast(streamID uuid.UUID, stopCast chan bool) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	go element.startHls(streamID, element.Streams.Streams[streamID].hlsChanel, stopCast)
}

func (element *Application) list() (uuid.UUID, []uuid.UUID) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	res := []uuid.UUID{}
	first := uuid.UUID{}
	for k := range element.Streams.Streams {
		if first.String() == "" {
			first = k
		}
		res = append(res, k)
	}
	return first, res
}
