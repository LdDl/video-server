package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// StartHTTPServer Initialize http server and run it
func (app *Application) StartHTTPServer() {
	router := gin.New()

	gin.SetMode(gin.ReleaseMode)
	pprof.Register(router)

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	router.Use(cors.New(app.CorsConfig))
	router.GET("/list", ListWrapper(app))
	router.GET("/status", StatusWrapper(app))
	router.GET("/ws/:suuid", WebSocketWrapper(app, &wsUpgrader))
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
func WebSocketWrapper(app *Application, wsUpgrader *websocket.Upgrader) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		wshandler(wsUpgrader, ctx.Writer, ctx.Request, app)
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
