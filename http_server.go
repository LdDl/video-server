package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
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
	// @todo I guess we should make proper configuration...
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
	router.GET("/ws/:suuid", WebSocketWrapper(app, &wsUpgrader))
	router.GET("/hls/:file", HLSWrapper(app))
	s := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.Server.HTTPAddr, app.Server.VideoHTTPPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Can't run Video-server on port: %d\n", app.Server.VideoHTTPPort)
		return
	}
}

// StartAPIServer starts server with API functionality
func (app *Application) StartAPIServer() {
	router := gin.New()

	gin.SetMode(gin.ReleaseMode)
	pprof.Register(router)

	if app.CorsConfig != nil {
		router.Use(cors.New(*app.CorsConfig))
	}
	router.GET("/list", ListWrapper(app))
	router.GET("/status", StatusWrapper(app))
	router.POST("/enable_camera", EnableCamera(app))
	router.POST("/disable_camera", DisableCamera(app))

	s := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", app.Server.HTTPAddr, app.Server.APIHTTPPort),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("Can't run API-server on port: %d\n", app.Server.APIHTTPPort)
		return
	}
}

// ListWrapper returns list of streams
func ListWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		_, all := app.list()
		ctx.JSON(200, all)
	}
}

// StatusWrapper returns statuses for list of streams
func StatusWrapper(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		ctx.JSON(200, app)
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

// EnablePostData is a POST-body for API which enables to turn on/off specific streams
type EnablePostData struct {
	GUID        uuid.UUID `json:"guid"`
	URL         string    `json:"url"`
	StreamTypes []string  `json:"stream_types"`
}

// EnableCamera adds new stream if does not exist
func EnableCamera(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		var postData EnablePostData
		if err := ctx.ShouldBindJSON(&postData); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"Error": err.Error()})
			return
		}
		if exist := app.exists(postData.GUID); !exist {
			app.Streams.Lock()
			app.Streams.Streams[postData.GUID] = NewStreamConfiguration(postData.URL, postData.StreamTypes)
			app.Streams.Unlock()
			app.StartStream(postData.GUID)
		}
		ctx.JSON(200, app)
	}
}

// DisableCamera turns off stream for specific stream ID
func DisableCamera(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		var postData EnablePostData
		if err := ctx.ShouldBindJSON(&postData); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"Error": err.Error()})
			return
		}

		if exist := app.exists(postData.GUID); exist {
			app.Streams.Lock()
			delete(app.Streams.Streams, postData.GUID)
			app.Streams.Unlock()
		}
		ctx.JSON(200, app)
	}
}
