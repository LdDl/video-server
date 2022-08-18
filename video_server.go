package videoserver

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// @todo: eliminate this regexp and use the third party
var uuidRegExp = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}")

// StartVideoServer initializes "video" server and run it (MSE-websockets and HLS-static files)
func (app *Application) StartVideoServer() {
	router := gin.New()

	if strings.ToLower(app.APICfg.Mode) == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	pprof.Register(router)

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	if app.CorsConfig != nil {
		router.Use(cors.New(*app.CorsConfig))
	}
	router.GET("/ws/:stream_id", WebSocketWrapper(app, &wsUpgrader))
	router.GET("/hls/:file", HLSWrapper(app))

	url := fmt.Sprintf("%s:%d", app.VideoServerCfg.Host, app.VideoServerCfg.Port)
	s := &http.Server{
		Addr:         url,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := s.ListenAndServe()
	if err != nil {
		fmt.Printf("Can't start Video-server '%s' due the error: %s\n", url, err.Error())
		return
	}
}

// WebSocketWrapper returns WS handler
func WebSocketWrapper(app *Application, wsUpgrader *websocket.Upgrader) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		wshandler(wsUpgrader, ctx.Writer, ctx.Request, app)
	}
}

// HLSWrapper returns HLS handler (static files)
func HLSWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		file := ctx.Param("file")
		_, err := uuid.Parse(uuidRegExp.FindString(file))
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"Error": err.Error()})
			return
		}
		ctx.Header("Cache-Control", "no-cache")
		ctx.FileFromFS(file, http.Dir(app.HLS.Directory))
	}
}
