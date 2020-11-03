package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/LdDl/video-server/internal/hlserror"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var uuidRegExp = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}")

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
	if app.CorsConfig != nil {
		router.Use(cors.New(*app.CorsConfig))
	}
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
		k, err := uuid.Parse(uuidRegExp.FindString(file))
		if err == nil {
			code, err := hlserror.GetError(k)
			hlserror.SetError(k, 200, nil)
			if code != 200 {
				ctx.JSON(code, err.Error())
				return
			}
		}
		ctx.Header("Cache-Control", "no-cache")
		ctx.FileFromFS(file, http.Dir(app.HlsDirectory))
	}
}
