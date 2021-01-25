package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	videoserver "github.com/LdDl/video-server"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile = flag.String("memprofile", "", "write memory profile to `file`")
	conf       = flag.String("conf", "conf.json", "Path to configuration JSON-file")
)

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Printf("Could not create file for CPU profiling: %s\n", err.Error())
			return
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Printf("Could not start CPU profiling: %s\n", err.Error())
			return
		}
		defer pprof.StopCPUProfile()
	}

	settings, err := videoserver.NewConfiguration(*conf)
	if err != nil {
		fmt.Printf("Can't prepare setting due the error: %s", err.Error())
		return
	}
	app, err := videoserver.NewApplication(settings)
	if err != nil {
		fmt.Printf("Can't prepare application due the error: %s", err.Error())
		return
	}

	go app.StartHTTPServer()
	go app.StartHTTPVideoServer()
	go app.StartStreams()

	sigOUT := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sigOUT, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigOUT
		log.Println("Server has captured signal:", sig)
		exit <- true
	}()
	log.Println("Server has been started (awaiting signal to exit)")
	<-exit
	log.Println("Stopping video_server")

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Printf("Could not create file for memory profiling: %s\n", err.Error())
			return
		}
		defer f.Close()
		// Explicit for garbage collection
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Printf("Could not write to file for memory profiling: %s\n", err.Error())
			return
		}
	}

}
