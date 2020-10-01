package videoserver

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/morozka/vdk/av"
)

// StreamsMap map of *StreamConfiguration with mutex
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

// AppConfiguration Configuration parameters for application
type AppConfiguration struct {
	Server          ServerConfiguration
	Streams         StreamsMap
	HlsMsPerSegment int64
	HlsDirectory    string
	HlsWindowSize   uint
	HlsCapacity     uint
}

// ServerConfiguration Configuration parameters for server
type ServerConfiguration struct {
	HTTPPort string `json:"http_port"`
}

// StreamConfiguration Configuration parameters for stream
type StreamConfiguration struct {
	URL       string `json:"url"`
	Status    bool   `json:"status"`
	Codecs    []av.CodecData
	Clients   map[uuid.UUID]viewer
	HlsChanel chan av.Packet `json:"-"`
}

type viewer struct {
	c chan av.Packet
}

const (
	defaultHlsDir          = "./hls"
	defaultHlsMsPerSegment = 10000
	defaultHlsCapacity     = 10
	defaultHlsWindowSize   = 5
)

// ConfigurationArgs Configuration parameters for application from JSON-file
type ConfigurationArgs struct {
	Server          ServerConfiguration `json:"server"`
	Streams         []StreamArg         `json:"streams"`
	HlsMsPerSegment int64               `json:"hls_ms_per_segment"`
	HlsDirectory    string              `json:"hls_directory"`
	HlsWindowSize   uint                `json:"hls_window_size"`
	HlsCapacity     uint                `json:"hls_window_capacity"`
	StreamTypes     []string            `json:"stream_types"`
}

// StreamArg Infromation about stream's source
type StreamArg struct {
	GUID string `json:"guid"`
	URL  string `json:"url"`
}

// NewAppConfiguration Prepare configuration for application
func NewAppConfiguration(fname string) (*AppConfiguration, error) {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	jsonConf := ConfigurationArgs{}
	err = json.Unmarshal(data, &jsonConf)
	if err != nil {
		return nil, err
	}
	if jsonConf.HlsDirectory == "" {
		jsonConf.HlsDirectory = defaultHlsDir
	}
	if jsonConf.HlsMsPerSegment == 0 {
		jsonConf.HlsMsPerSegment = defaultHlsMsPerSegment
	}
	if jsonConf.HlsCapacity == 0 {
		jsonConf.HlsCapacity = defaultHlsCapacity
	}
	if jsonConf.HlsWindowSize == 0 {
		jsonConf.HlsWindowSize = defaultHlsWindowSize
	}
	if jsonConf.HlsWindowSize > jsonConf.HlsCapacity {
		jsonConf.HlsWindowSize = jsonConf.HlsCapacity
	}
	tmp := AppConfiguration{
		Server:          jsonConf.Server,
		Streams:         StreamsMap{Streams: make(map[uuid.UUID]*StreamConfiguration)},
		HlsMsPerSegment: jsonConf.HlsMsPerSegment,
		HlsDirectory:    jsonConf.HlsDirectory,
		HlsWindowSize:   jsonConf.HlsWindowSize,
		HlsCapacity:     jsonConf.HlsCapacity,
	}

	for i := range jsonConf.Streams {
		validUUID, err := uuid.Parse(jsonConf.Streams[i].GUID)
		if err != nil {
			log.Printf("Not valid UUID: %s\n", jsonConf.Streams[i].GUID)
			continue
		}
		tmp.Streams.Streams[validUUID] = &StreamConfiguration{
			URL:       jsonConf.Streams[i].URL,
			Clients:   make(map[uuid.UUID]viewer),
			HlsChanel: make(chan av.Packet, 100),
		}
	}
	return &tmp, nil
}

func (element *AppConfiguration) cast(id uuid.UUID, pck av.Packet) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	curStream, _ := element.Streams.Streams[id]
	curStream.HlsChanel <- pck
	for _, v := range curStream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *AppConfiguration) ext(streamID uuid.UUID) bool {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	_, ok := element.Streams.Streams[streamID]
	return ok
}

func (element *AppConfiguration) codecAdd(streamID uuid.UUID, codecs []av.CodecData) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	element.Streams.Streams[streamID].Codecs = codecs
}

func (element *AppConfiguration) codecGet(streamID uuid.UUID) []av.CodecData {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	curStream, _ := element.Streams.Streams[streamID]
	return curStream.Codecs
}

func (element *AppConfiguration) updateStatus(streamID uuid.UUID, status bool) {
	element.Streams.Lock()
	defer element.Streams.Unlock()
	t, _ := element.Streams.Streams[streamID]
	t.Status = status
	element.Streams.Streams[streamID] = t
}

func (element *AppConfiguration) clientAdd(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
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

func (element *AppConfiguration) clientDelete(streamID, clientID uuid.UUID) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	delete(element.Streams.Streams[streamID].Clients, clientID)
}

func (element *AppConfiguration) startHlsCast(streamID uuid.UUID, stopCast chan bool) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	go element.startHls(streamID, element.Streams.Streams[streamID].HlsChanel, stopCast)
}

func (element *AppConfiguration) list() (uuid.UUID, []uuid.UUID) {
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
