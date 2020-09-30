package videoserver

import (
	"log"
	"net/http"
	"time"

	"github.com/morozka/vdk/format/mp4f"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// StartHTTPServer Initialize http server and run it
func StartHTTPServer(cfg *AppConfiguration) {
	router := gin.New()

	gin.SetMode(gin.ReleaseMode)
	pprof.Register(router)
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	router.Use(cors.New(config))

	router.GET("/list", func(ctx *gin.Context) {
		_, all := cfg.list()
		ctx.JSON(200, all)
	})
	router.GET("/status", func(ctx *gin.Context) {
		ctx.JSON(200, cfg)
	})
	router.GET("/ws/:suuid", func(ctx *gin.Context) {
		wshandler(ctx.Writer, ctx.Request, cfg)
	})
	router.GET("/hls/:file", func(ctx *gin.Context) {
		file := ctx.Param("file")
		ctx.Header("Cache-Control", "no-cache")
		ctx.FileFromFS(file, http.Dir("./hls"))
	})

	s := &http.Server{
		Addr:         cfg.Server.HTTPPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Can't run HTTP-server on port: %s\n", cfg.Server.HTTPPort)
		return
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wshandler(w http.ResponseWriter, r *http.Request, cfg *AppConfiguration) {
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
	if cfg.ext(streamID) {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		cuuid, ch, err := cfg.clientAdd(streamID)
		if err != nil {
			log.Printf("Can't add client for '%s' due the error: %s\n", streamID, err.Error())
			return
		}
		defer cfg.clientDelete(streamID, cuuid)
		codecs := cfg.codecGet(streamID)
		if codecs == nil {
			log.Printf("No codec information for stream %s\n", streamID)
			return
		}
		muxer := mp4f.NewMuxer(nil)
		muxer.WriteHeader(codecs)
		meta, init := muxer.GetInit(codecs)
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
