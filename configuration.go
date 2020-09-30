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

//GetStream - returns stream from the map by the uuid
func (sm *StreamsMap) GetStream(uuid uuid.UUID) (*StreamConfiguration, bool) {
	sm.Lock()
	stream, ok := sm.Streams[uuid]
	sm.Unlock()
	return stream, ok
}

//SetStream - sets the scfg to map by uuid
func (sm *StreamsMap) SetStream(uuid uuid.UUID, scfg *StreamConfiguration) {
	sm.Lock()
	sm.Streams[uuid] = scfg
	sm.Unlock()
}

//DeleteStream - removes stream by its uuid
func (sm *StreamsMap) DeleteStream(uuid uuid.UUID) {
	sm.Lock()
	delete(sm.Streams, uuid)
	sm.Unlock()
}

//GetKeys - returns snap of all keys
func (sm *StreamsMap) GetKeys() []uuid.UUID {
	sm.Lock()
	defer sm.Unlock()
	return []uuid.UUID{}
}

// AppConfiguration Configuration parameters for application
type AppConfiguration struct {
	mutex  sync.Mutex
	Server ServerConfiguration `json:"server"`
	//Streams map[uuid.UUID]*StreamConfiguration `json:"streams"`
	Streams         StreamsMap `json:"streams"`
	HlsMsPerSegment int64      `json:"hlsMsPerSegment"`
	HlsDirectory    string     `json:"hlsDirectory"`
	HlsWindowSize   uint       `json:"hlsWindowSize"`
	HlsCapacity     uint       `json:"hlsWindowCapacity"`
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

	tmp.Streams = StreamsMap{Streams: make(map[uuid.UUID]*StreamConfiguration)}
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
		tmp.Streams.SetStream(validUUID, &StreamConfiguration{
			URL:       jsonConf.Streams[i].URL,
			Clients:   make(map[uuid.UUID]viewer),
			HlsChanel: make(chan av.Packet, 100),
		})
	}
	return &tmp, nil
}

func (element *AppConfiguration) cast(id uuid.UUID, pck av.Packet) {
	curStream, _ := element.Streams.GetStream(id)
	curStream.HlsChanel <- pck
	for _, v := range curStream.Clients {
		if len(v.c) < cap(v.c) {
			v.c <- pck
		}
	}
}

func (element *AppConfiguration) ext(streamID uuid.UUID) bool {
	_, ok := element.Streams.GetStream(streamID)
	return ok
}

func (element *AppConfiguration) codecAdd(streamID uuid.UUID, codecs []av.CodecData) {
	defer element.Streams.Unlock()
	element.Streams.Lock()
	t, _ := element.Streams.GetStream(streamID)
	t.Codecs = codecs
	element.Streams.SetStream(streamID, t)
}

func (element *AppConfiguration) codecGet(streamID uuid.UUID) []av.CodecData {
	curStream, _ := element.Streams.GetStream(streamID)
	return curStream.Codecs
}

func (element *AppConfiguration) updateStatus(streamID uuid.UUID, status bool) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	t, _ := element.Streams.GetStream(streamID)
	t.Status = status
	element.Streams.SetStream(streamID, t)
}

func (element *AppConfiguration) clientAdd(streamID uuid.UUID) (uuid.UUID, chan av.Packet, error) {
	defer element.mutex.Unlock()
	element.mutex.Lock()
	clientID, err := uuid.NewUUID()
	if err != nil {
		return uuid.UUID{}, nil, err
	}
	ch := make(chan av.Packet, 100)
	curStream, _ := element.Streams.GetStream(streamID)
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
