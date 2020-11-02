package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/morozka/vdk/format/mp4f"
)

func wshandler(wsUpgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request, app *Application) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		closeWSwithError(conn, 1011, fmt.Sprintf("Failed to make websocket upgrade: %s\n", err.Error()))
		log.Printf("Failed to make websocket upgrade: %s\n", err.Error())
		return
	}

	defer func() {
		err = conn.Close()
		if err != nil {
			// log.Printf("WS connection has been closed %s: %s\n", conn.RemoteAddr().String(), err.Error())
		}
		// log.Printf("WS connection has been terminated %s\n", conn.RemoteAddr().String())
	}()

	streamIDSTR := r.FormValue("suuid")
	streamID, err := uuid.Parse(streamIDSTR)
	if err != nil {
		closeWSwithError(conn, 1011, fmt.Sprintf("Can't parse UUID: '%s' due the error: %s\n", streamIDSTR, err.Error()))
		log.Printf("Can't parse UUID: '%s' due the error: %s\n", streamIDSTR, err.Error())
		return
	}

	if app.existsWithType(streamID, "mse") {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		cuuid, ch, err := app.clientAdd(streamID)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("  Can't add client for '%s' due the error: %s\n", streamID, err.Error()))
			log.Printf("Can't add client for '%s' due the error: %s\n", streamID, err.Error())
			return
		}
		defer app.clientDelete(streamID, cuuid)
		codecData, err := app.codecGet(streamID)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't add client '%s' due the error: %s\n", streamID, err.Error()))
			log.Printf("Can't add client '%s' due the error: %s\n", streamID, err.Error())
			return
		}
		if codecData == nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("No codec information for stream %s\n", streamID))
			log.Printf("No codec information for stream %s\n", streamID)
			return
		}
		muxer := mp4f.NewMuxer(nil)
		muxer.WriteHeader(codecData)
		meta, init := muxer.GetInit(codecData)
		err = conn.WriteMessage(websocket.BinaryMessage, append([]byte{9}, meta...))
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't write header to %s: %s\n", conn.RemoteAddr().String(), err.Error()))
			log.Printf("Can't write header to %s: %s\n", conn.RemoteAddr().String(), err.Error())
			return
		}
		err = conn.WriteMessage(websocket.BinaryMessage, init)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't write message to %s: %s\n", conn.RemoteAddr().String(), err.Error()))
			log.Printf("Can't write message to %s: %s\n", conn.RemoteAddr().String(), err.Error())
			return
		}
		var start bool
		quitCh := make(chan bool)
		go func(q chan bool) {
			_, _, err := conn.ReadMessage()
			if err != nil {
				q <- true
				closeWSwithError(conn, 1011, fmt.Sprintf("Read message error: %s\n", err.Error()))
				log.Printf("Read message error: %s\n", err.Error())
				return
			}
		}(quitCh)
		for {
			select {
			case <-quitCh:
				return
			case pck := <-ch:
				if pck.IsKeyFrame {
					start = true
				}
				if !start {
					continue
				}
				ready, buf, _ := muxer.WritePacket(pck, false)
				if ready {
					conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
					err := conn.WriteMessage(websocket.BinaryMessage, buf)
					if err != nil {
						closeWSwithError(conn, 1011, fmt.Sprintf("Can't write messsage due the error: %s\n", err.Error()))
						log.Printf("Can't write messsage due the error: %s\n", err.Error())
						return
					}
				}
			}
		}
	}
}

func prepareError(code int16, message string) []byte {
	buf := make([]byte, 0, 2+len(message))
	var h, l uint8 = uint8(code >> 8), uint8(code & 0xff)
	buf = append(buf, h, l)
	buf = append(buf, []byte(message)...)
	return buf
}
func closeWSwithError(conn *websocket.Conn, code int16, message string) {
	conn.WriteControl(8, prepareError(code, message), time.Now().Add(10*time.Second))
}
