package videoserver

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/morozka/vdk/av"
)

// AppConfiguration Configuration parameters for application
type AppConfiguration struct {
	mutex   sync.Mutex
	Server  ServerConfiguration                `json:"server"`
	Streams map[uuid.UUID]*StreamConfiguration `json:"streams"`

	HlsMsPerSegment int64  `json:"hlsMsPerSegment"`
	HlsDirectory    string `json:"hlsDirectory"`
	HlsWindowSize   uint   `json:"hlsWindowSize"`
	HlsCapacity     uint   `json:"hlsWindowCapacity"`
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
	HlsMsPerSegment int64               `json:"hlsMsPerSegment"`
	HlsDirectory    string              `json:"hlsDirectory"`
	HlsWindowSize   uint                `json:"hlsWindowSize"`
	HlsCapacity     uint                `json:"hlsWindowCapacity"`
}

// StreamArg Infromation about stream's source
type StreamArg struct {
	GUID string `json:"guid"`
	URL  string `json:"url"`
}

// NewAppConfiguration Prepare configuration for application
func NewAppConfiguration(fname string) (*AppConfiguration, error) {
	var tmp AppConfiguration
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}
	var jsonConf ConfigurationArgs
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

	tmp.Streams = make(map[uuid.UUID]*StreamConfiguration)
	tmp.Server = jsonConf.Server
	tmp.HlsMsPerSegment = jsonConf.HlsMsPerSegment
	tmp.HlsDirectory = jsonConf.HlsDirectory
	tmp.HlsWindowSize = jsonConf.HlsWindowSize
	tmp.HlsCapacity = jsonConf.HlsCapacity
	for i := range jsonConf.Streams {
		validUUID, err := uuid.Parse(jsonConf.Streams[i].GUID)
		if err != nil {
			log.Printf("Not valid UUID: %s\n", jsonConf.Streams[i].GUID)
			continue
		}
		tmp.Streams[validUUID] = &StreamConfiguration{
			URL:       jsonConf.Streams[i].URL,
			Clients:   make(map[uuid.UUID]viewer),
			HlsChanel: make(chan av.Packet, 100),
		}
	}
	return &tmp, nil
}

func (element *AppConfiguration) cast(id uuid.UUID, pck av.Packet) {
	element.Streams[id].HlsChanel <- pck
	for _, v := range element.Streams[id].Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *AppConfiguration) ext(streamID uuid.UUID) bool {
	_, ok := element.Streams[streamID]
	return ok
}

func (element *AppConfiguration) codecAdd(streamID uuid.UUID, codecs []av.CodecData) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	t := element.Streams[streamID]
	t.Codecs = codecs
	element.Streams[streamID] = t
}

func (element *AppConfiguration) codecGet(streamID uuid.UUID) []av.CodecData {
	return element.Streams[streamID].Codecs
}

func (element *AppConfiguration) updateStatus(streamID uuid.UUID, status bool) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	t := element.Streams[streamID]
	t.Status = status
	element.Streams[streamID] = t
}

func (element *AppConfiguration) clientAdd(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	clientID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	ch := make(chan av.Packet, 100)
	element.Streams[streamID].Clients[clientID] = viewer{c: ch}
	return clientID, ch, nil
}

func (element *AppConfiguration) clientDelete(streamID, clientID uuid.UUID) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	delete(element.Streams[streamID].Clients, clientID)
}

func (element *AppConfiguration) startHlsCast(streamID uuid.UUID, stopCast chan bool) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	go element.startHls(streamID, element.Streams[streamID].HlsChanel, stopCast)
}

func (element *AppConfiguration) list() (uuid.UUID, []uuid.UUID) {
	res := []uuid.UUID{}
	first := uuid.UUID{}
	for k := range element.Streams {
		if first.String() == "" {
			first = k
		}
		res = append(res, k)
	}
	return first, res
}
