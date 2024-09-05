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

var (
	keyFramesTimeout = 10 * time.Second
	deadlineTimeout  = 10 * time.Second
	controlTimeout   = 10 * time.Second
	SCOPE_WS_HANDLER = "ws_handler"
)

// wshandler is a websocket handler for user connection
func wshandler(wsUpgrader *websocket.Upgrader, w http.ResponseWriter, r *http.Request, app *Application) {
	streamIDSTR := r.FormValue("stream_id")
	log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg("MSE Connected")

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		errReason := "Can't call websocket upgrader"
		log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg(errReason)
		closeWSwithError(conn, 1011, errReason)
		return
	}
	defer func() {
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Msg("Connection has been closed")
		conn.Close()
	}()

	streamID, err := uuid.Parse(streamIDSTR)
	if err != nil {
		errReason := fmt.Sprintf("Not valid UUID: '%s'", streamIDSTR)
		log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg(errReason)
		closeWSwithError(conn, 1011, errReason)
		return
	}
	mseExists := app.existsWithType(streamID, STREAM_TYPE_MSE)
	log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Bool("mse_exists", mseExists).Msg("Validate stream type")
	if mseExists {
		err = conn.SetWriteDeadline(time.Now().Add(deadlineTimeout))
		if err != nil {
			errReason := "Can't set deadline"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("event", "ping").Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		cuuid, ch, err := app.addClient(streamID)
		if err != nil {
			errReason := "Can't add client to the queue"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		defer app.clientDelete(streamID, cuuid)
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Client has been added")

		codecData, err := app.getCodec(streamID)
		if err != nil {
			errReason := "Can't extract codec for stream"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Msg("Validate codecs")

		if len(codecData) == 0 {
			errReason := "No codec information"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		muxer := mp4f.NewMuxer(nil)
		err = muxer.WriteHeader(codecData)
		if err != nil {
			errReason := "Can't write codec information to the header"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Msg("Write header to muxer")

		meta, init := muxer.GetInit(codecData)
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Get meta information")

		err = conn.WriteMessage(websocket.BinaryMessage, append([]byte{9}, meta...))
		if err != nil {
			errReason := "Can't write meta information"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Send meta information")
		err = conn.WriteMessage(websocket.BinaryMessage, init)
		if err != nil {
			errReason := "Can't write initialization information"
			log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg(errReason)
			closeWSwithError(conn, 1011, errReason)
			return
		}
		log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg("Send initialization message")

		var start bool
		quitCh := make(chan bool)
		rxPingCh := make(chan bool)

		go func(q, p chan bool) {
			log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Start loop in goroutine")
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					q <- true
					errReason := "Can't read message"
					log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg(errReason)
					closeWSwithError(conn, 1011, errReason)
					return
				}
				log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Read message in a loop")
				if msgType == websocket.TextMessage && len(data) > 0 && string(data) == "ping" {
					select {
					case p <- true:
						log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Message has been sent")
						// message sent
					default:
						// message dropped
						log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Int("message_type", msgType).Int("data_len", len(data)).Msg("Message has been dropped")
					}
				}
			}
		}(quitCh, rxPingCh)

		noKeyFrames := time.NewTimer(keyFramesTimeout)

		log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Start loop")

		for {
			select {
			case <-noKeyFrames.C:
				log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("No keyframes has been met")
				return
			case <-quitCh:
				log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Quit")
				return
			case <-rxPingCh:
				log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("'Ping' has been recieved")
				err := conn.WriteMessage(websocket.TextMessage, []byte("pong"))
				if err != nil {
					errReason := "Can't write PONG message"
					log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("event", "ping").Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("codecs", codecData).Str("meta", meta).Any("init", init).Msg(errReason)
					closeWSwithError(conn, 1011, errReason)
					return
				}
			case pck := <-ch:
				// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Packet has been recieved from stream source")
				if pck.IsKeyFrame {
					log.Info().Str("scope", SCOPE_WS_HANDLER).Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Packet is a keyframe")
					noKeyFrames.Reset(keyFramesTimeout)
					start = true
				}
				if !start {
					// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Msg("Stream has not been started")
					continue
				}
				ready, buf, err := muxer.WritePacket(pck, false)
				if err != nil {
					errReason := "Can't write packet to the muxer"
					log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("event", "ping").Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("packet_len", len(pck.Data)).Msg(errReason)
					closeWSwithError(conn, 1011, errReason)
					return
				}
				// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Bool("ready", ready).Int("buf_len", len(buf)).Msg("Write packet to the muxer")

				if ready {
					// log.Info().Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Bool("ready", ready).Int("buf_len", len(buf)).Msg("Muxer is ready to write another packet")
					err = conn.SetWriteDeadline(time.Now().Add(deadlineTimeout))
					if err != nil {
						errReason := "Can't set new deadline"
						log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("event", "ping").Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("packet_len", len(pck.Data)).Bool("ready", ready).Int("buf_len", len(buf)).Msg(errReason)
						closeWSwithError(conn, 1011, errReason)
						return
					}
					err := conn.WriteMessage(websocket.BinaryMessage, buf)
					if err != nil {
						errReason := "Can't write buffered message"
						log.Error().Err(err).Str("scope", SCOPE_WS_HANDLER).Str("event", "ping").Str("remote_addr", r.RemoteAddr).Str("stream_id", streamIDSTR).Str("client_id", cuuid.String()).Any("packet_len", len(pck.Data)).Bool("ready", ready).Int("buf_len", len(buf)).Msg(errReason)
						closeWSwithError(conn, 1011, errReason)
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
	conn.WriteControl(8, prepareError(code, message), time.Now().Add(controlTimeout))
}
