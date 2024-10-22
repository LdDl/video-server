package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	videoserver "github.com/LdDl/video-server"
	"github.com/LdDl/video-server/configuration"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	cpuprofile          = flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile          = flag.String("memprofile", "", "write memory profile to `file`")
	conf                = flag.String("conf", "conf.json", "Path to configuration JSON-file")
	EVENT_CPU           = "cpu_profile"
	EVENT_MEMORY        = "memory_profile"
	EVENT_APP_START     = "app_start"
	EVENT_APP_STOP      = "app_stop"
	EVENT_APP_SIGNAL_OS = "app_signal_os"
)

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.DurationFieldUnit = time.Second
}

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Error().Err(err).Str("event", EVENT_CPU).Msg("Could not create file for CPU profiling")
			return
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Error().Err(err).Str("event", EVENT_CPU).Msg("Could not start CPU profiling")
			return
		}
		defer pprof.StopCPUProfile()
	}
	appCfg, err := configuration.PrepareConfiguration(conf)
	if err != nil {
		log.Error().Err(err).Str("scope", videoserver.SCOPE_CONFIGURATION).Msg("Could not prepare application configuration")
		return
	}

	app, err := videoserver.NewApplication(appCfg)
	if err != nil {
		log.Error().Err(err).Str("scope", videoserver.SCOPE_CONFIGURATION).Msg("Could not prepare application")
		return
	}

	if strings.ToLower(app.APICfg.Mode) == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Run streams
	go app.StartStreams()

	// Start "Video" server
	go app.StartVideoServer()

	// Start API server
	if appCfg.APICfg.Enabled {
		go app.StartAPIServer()
	}

	sigOUT := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sigOUT, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigOUT
		log.Info().Str("event", EVENT_APP_SIGNAL_OS).Any("signal", sig).Msg("Server has captured signal")
		exit <- true
	}()
	log.Info().Str("event", EVENT_APP_START).Msg("Server has been started (awaiting signal to exit)")
	<-exit
	log.Info().Str("event", EVENT_APP_STOP).Msg("Stopping video server")

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Error().Err(err).Str("event", EVENT_MEMORY).Msg("Could not create file for memory profiling")
			return
		}
		defer f.Close()
		// Explicit for garbage collection
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Error().Err(err).Str("event", EVENT_MEMORY).Msg("Could not write to file for memory profiling")
			return
		}
	}
}
