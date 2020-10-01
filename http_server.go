package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/morozka/vdk/format/mp4f"
)

// StartHTTPServer Initialize http server and run it
func (app *Application) StartHTTPServer() {
	router := gin.New()

	gin.SetMode(gin.ReleaseMode)
	pprof.Register(router)
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	router.Use(cors.New(config))
	router.GET("/list", ListWrapper(app))
	router.GET("/status", StatusWrapper(app))
	router.GET("/ws/:suuid", WebSocketWrapper(app))
	router.GET("/hls/:file", HLSWrapper(app))

	s := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.Server.HTTPAddr, app.Server.HTTPPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Can't run HTTP-server on port: %d\n", app.Server.HTTPPort)
		return
	}
}

// ListWrapper Returns list of streams
func ListWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		_, all := app.list()
		ctx.JSON(200, all)
	}
}

// StatusWrapper Returns statuses for list of streams
func StatusWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		ctx.JSON(200, app)
	}
}

// WebSocketWrapper Returns WS handler
func WebSocketWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		wshandler(ctx.Writer, ctx.Request, app)
	}
}

// HLSWrapper Returns HLS handler (static files)
func HLSWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		file := ctx.Param("file")
		ctx.Header("Cache-Control", "no-cache")
		ctx.FileFromFS(file, http.Dir(app.HlsDirectory))
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wshandler(w http.ResponseWriter, r *http.Request, app *Application) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
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
		log.Printf("Can't parse UUID: '%s' due the error: %s\n", streamIDSTR, err.Error())
		return
	}
	// log.Println("Request", streamID)
	if app.ext(streamID) {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		cuuid, ch, err := app.clientAdd(streamID)
		if err != nil {
			log.Printf("Can't add client for '%s' due the error: %s\n", streamID, err.Error())
			return
		}
		defer app.clientDelete(streamID, cuuid)
		codecData, err := app.codecGet(streamID)
		if err != nil {
			log.Printf("Can't add client '%s' due the error: %s\n", streamID, err.Error())
			return
		}
		if codecData == nil {
			log.Printf("No codec information for stream %s\n", streamID)
			return
		}
		muxer := mp4f.NewMuxer(nil)
		muxer.WriteHeader(codecData)
		meta, init := muxer.GetInit(codecData)
		err = conn.WriteMessage(websocket.BinaryMessage, append([]byte{9}, meta...))
		if err != nil {
			log.Printf("Can't write header to %s: %s\n", conn.RemoteAddr().String(), err.Error())
			return
		}
		err = conn.WriteMessage(websocket.BinaryMessage, init)
		if err != nil {
			log.Printf("Can't write message to %s: %s\n", conn.RemoteAddr().String(), err.Error())
			return
		}
		var start bool
		quitCh := make(chan bool)
		go func(q chan bool) {
			_, _, err := conn.ReadMessage()
			if err != nil {
				q <- true
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
						log.Printf("Can't write messsage due the error: %s\n", err.Error())
						return
					}
				}
			}
		}
	}
}
