package videoserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/deepch/vdk/format/mp4f"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var keyFramesTimeout = 10 * time.Second

// wshandler is a websocket handler for user connection
func wshandler(wsUpgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request, app *Application) {
	streamIDSTR := r.FormValue("stream_id")
	log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg("MSE Connected")

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		closeWSwithError(conn, 1011, fmt.Sprintf("Failed to make websocket upgrade: %s\n", err.Error()))
		return
	}
	defer func() {
		log.Info().Str("remote_addr", r.RemoteAddr).Msg("Connection has been closed")
		conn.Close()
	}()

	streamID, err := uuid.Parse(streamIDSTR)
	if err != nil {
		closeWSwithError(conn, 1011, fmt.Sprintf("Can't parse UUID: '%s' due the error: %s\n", streamIDSTR, err.Error()))
		return
	}
	mseExists := app.existsWithType(streamID, STREAM_TYPE_MSE)
	log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Bool("mse_exists", mseExists).Msg("Validate stream type")
	if mseExists {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		cuuid, ch, err := app.addClient(streamID)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't add client for '%s' due the error: %s\n", streamID, err.Error()))
			return
		}
		defer app.clientDelete(streamID, cuuid)
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Client has been added")

		codecData, err := app.getCodec(streamID)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't add client '%s' due the error: %s\n", streamID, err.Error()))
			return
		}
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Msg("Validate codecs")

		if len(codecData) == 0 {
			closeWSwithError(conn, 1011, fmt.Sprintf("No codec information for stream %s\n", streamID))
			return
		}
		muxer := mp4f.NewMuxer(nil)
		err = muxer.WriteHeader(codecData)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't write header to muxer for %s: %s\n", conn.RemoteAddr().String(), err.Error()))
			return
		}
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Msg("Write header to muxer")

		meta, init := muxer.GetInit(codecData)
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Get meta information")

		err = conn.WriteMessage(websocket.BinaryMessage, append([]byte{9}, meta...))
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't write header to %s: %s\n", conn.RemoteAddr().String(), err.Error()))
			return
		}
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Send meta information")
		err = conn.WriteMessage(websocket.BinaryMessage, init)
		if err != nil {
			closeWSwithError(conn, 1011, fmt.Sprintf("Can't write message to %s: %s\n", conn.RemoteAddr().String(), err.Error()))
			return
		}
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Send initialization message")

		var start bool
		quitCh := make(chan bool)
		rxPingCh := make(chan bool)

		go func(q, p chan bool) {
			log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Start loop in goroutine")
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					q <- true
					closeWSwithError(conn, 1011, fmt.Sprintf("Read message error: %s\n", err.Error()))
					return
				}
				log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Read message in a loop")
				if msgType == websocket.TextMessage && len(data) > 0 && string(data) == "ping" {
					select {
					case p <- true:
						log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Message has been sent")
						// message sent
					default:
						// message dropped
						log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Message has been dropped")
					}
				}
			}
		}(quitCh, rxPingCh)

		noKeyFrames := time.NewTimer(keyFramesTimeout)

		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Start loop")

		for {
			select {
			case <-noKeyFrames.C:
				log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("No keyframes has been met")
				return
			case <-quitCh:
				log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Quit")
				return
			case <-rxPingCh:
				log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("'Ping' has been recieved")
				err := conn.WriteMessage(websocket.TextMessage, []byte("pong"))
				if err != nil {
					return
				}
			case pck := <-ch:
				// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Packet has been recieved from stream source")
				if pck.IsKeyFrame {
					log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Packet is a keyframe")
					noKeyFrames.Reset(keyFramesTimeout)
					start = true
				}
				if !start {
					// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Stream has not been started")
					continue
				}
				ready, buf, err := muxer.WritePacket(pck, false)
				if err != nil {
					log.Error().Err(err).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Can't write packet to the muxer")
					return
				}
				// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Bool("ready", ready).Int("buf_len", len(buf)).Msg("Write packet to the muxer")

				if ready {
					// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Bool("ready", ready).Int("buf_len", len(buf)).Msg("Muxer is ready to write another packet")
					err = conn.SetWriteDeadline(time.Now().Add(keyFramesTimeout))
					if err != nil {
						return
					}
					err := conn.WriteMessage(websocket.BinaryMessage, buf)
					if err != nil {
						closeWSwithError(conn, 1011, fmt.Sprintf("Can't write messsage due the error: %s\n", err.Error()))
						return
					}
					// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Bool("ready", ready).Int("buf_len", len(buf)).Msg("Write buffer to the client")
				}
			}
		}
	}
}

func prepareError(code int16, message string) []byte {
	buf := make([]byte, 0, 2+len(message))
	h, l := uint8(code>>8), uint8(code&0xff)
	buf = append(buf, h, l)
	buf = append(buf, []byte(message)...)
	return buf
}

func closeWSwithError(conn *websocket.Conn, code int16, message string) {
	conn.WriteControl(8, prepareError(code, message), time.Now().Add(10*time.Second))
}
