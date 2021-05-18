package videoserver

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/LdDl/video-server/internal/hlserror"
	"github.com/deepch/vdk/av"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var uuidRegExp = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}")

// StartVideoServer Initialize "video" server and run it (MSE-websockets and HLS-static files)
func (app *Application) StartVideoServer() {
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

// StartAPIServer Start separated server with API functionality
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

// EnablePostData ...
type EnablePostData struct {
	GUID        uuid.UUID `json:"guid"`
	URL         string    `json:"url"`
	StreamTypes []string  `json:"stream_types"`
}

// EnableCamera add new stream if does not exist
func EnableCamera(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		var postData EnablePostData
		if err := ctx.ShouldBindJSON(&postData); err != nil {
			ctx.JSON(http.StatusBadRequest, err)
			return
		}

		if exist := app.exists(postData.GUID); !exist {
			//add new
			app.Streams.Lock()
			app.Streams.Streams[postData.GUID] = &StreamConfiguration{
				URL:                  postData.URL,
				Clients:              make(map[uuid.UUID]viewer),
				hlsChanel:            make(chan av.Packet, 100),
				SupportedStreamTypes: postData.StreamTypes,
			}
			app.Streams.Unlock()
			app.StartStream(postData.GUID)
		}
		ctx.JSON(200, app)
	}
}

// DisableCamera switch working camera
func DisableCamera(app *Application) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		var postData EnablePostData
		if err := ctx.ShouldBindJSON(&postData); err != nil {
			ctx.JSON(http.StatusBadRequest, err)
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
