package videoserver

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// @todo: eliminate this regexp and use the third party
var uuidRegExp = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}")

// StartVideoServer initializes "video" server and run it (MSE-websockets and HLS-static files)
func (app *Application) StartVideoServer() {
	log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_PREPARE).Msg("Preparing to start WS Server")

	router := gin.New()

	pprof.Register(router)

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	if app.CorsConfig != nil {
		log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_CORS_ENABLE).
			Bool("cors_allow_all_origins", app.CorsConfig.AllowAllOrigins).
			Any("cors_allow_origins", app.CorsConfig.AllowOrigins).
			Any("cors_allow_methods", app.CorsConfig.AllowMethods).
			Bool("cors_allow_private_network", app.CorsConfig.AllowPrivateNetwork).
			Any("cors_allow_headers", app.CorsConfig.AllowHeaders).
			Bool("cors_allow_credentials", app.CorsConfig.AllowCredentials).
			Any("cors_expose_headers", app.CorsConfig.ExposeHeaders).
			Dur("cors_max_age", app.CorsConfig.MaxAge).
			Bool("cors_allow_wildcard", app.CorsConfig.AllowWildcard).
			Bool("cors_allow_browser_extensions", app.CorsConfig.AllowBrowserExtensions).
			Any("cors_custom_schemas", app.CorsConfig.CustomSchemas).
			Bool("cors_allow_websockets", app.CorsConfig.AllowWebSockets).
			Bool("cors_allow_files", app.CorsConfig.AllowFiles).
			Int("cors_allow_option_status_code", app.CorsConfig.OptionsResponseStatusCode).
			Msg("CORS are enabled")
		router.Use(cors.New(*app.CorsConfig))
	}
	router.GET("/ws/:stream_id", WebSocketWrapper(&app.Streams, &wsUpgrader, app.VideoServerCfg.Verbose))
	router.GET("/hls/:file", HLSWrapper(&app.HLS, app.VideoServerCfg.Verbose))

	url := fmt.Sprintf("%s:%d", app.VideoServerCfg.Host, app.VideoServerCfg.Port)
	s := &http.Server{
		Addr:         url,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	if app.VideoServerCfg.Verbose > VERBOSE_NONE {
		log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_START).Str("url", url).Msg("Start microservice for WS server")
	}
	err := s.ListenAndServe()
	if err != nil {
		log.Error().Err(err).Str("scope", "video_server").Str("event", "video_server_start").Str("url", url).Msg("Can't start video server routers")
		return
	}
}

// WebSocketWrapper returns WS handler
func WebSocketWrapper(streamsStorage *StreamsStorage, wsUpgrader *websocket.Upgrader, verboseLevel VerboseLevel) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_REQUEST).Str("method", ctx.Request.Method).Str("route", ctx.Request.URL.Path).Str("remote", ctx.Request.RemoteAddr).Msg("Try to call ws upgrader")
		}
		wshandler(wsUpgrader, ctx.Writer, ctx.Request, streamsStorage, verboseLevel)
	}
}

// HLSWrapper returns HLS handler (static files)
func HLSWrapper(hlsConf *HLSInfo, verboseLevel VerboseLevel) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_REQUEST).Str("method", ctx.Request.Method).Str("route", ctx.Request.URL.Path).Str("remote", ctx.Request.RemoteAddr).Str("hls_dir", hlsConf.Directory).Msg("Call HLS")
		}
		file := ctx.Param("file")
		_, err := uuid.Parse(uuidRegExp.FindString(file))
		if err != nil {
			errReason := "Not valid UUId"
			if verboseLevel > VERBOSE_NONE {
				log.Error().Err(err).Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_REQUEST).Str("method", ctx.Request.Method).Str("route", ctx.Request.URL.Path).Str("remote", ctx.Request.RemoteAddr).Str("hls_dir", hlsConf.Directory).Msg(errReason)
			}
			ctx.JSON(http.StatusBadRequest, gin.H{"Error": err.Error()})
			return
		}
		ctx.Header("Cache-Control", "no-cache")
		if verboseLevel > VERBOSE_SIMPLE {
			log.Info().Str("scope", SCOPE_WS_SERVER).Str("event", EVENT_WS_REQUEST).Str("method", ctx.Request.Method).Str("route", ctx.Request.URL.Path).Str("remote", ctx.Request.RemoteAddr).Str("hls_dir", hlsConf.Directory).Msg("Send file")
		}
		ctx.FileFromFS(file, http.Dir(hlsConf.Directory))
	}
}
