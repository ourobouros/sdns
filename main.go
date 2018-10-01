package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/semihalev/log"
)

var (
	configPath  string
	forceUpdate bool
	blockCache  = &MemoryBlockCache{Backend: make(map[string]bool)}
)

func init() {
	flag.StringVar(&configPath, "config", "sdns.toml", "location of the config file, if not found it will be generated (default sdns.toml)")
	flag.BoolVar(&forceUpdate, "update", false, "force an update of the blocklist database")
}

func main() {
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	if err := LoadConfig(configPath); err != nil {
		log.Crit("Config loading failed", "error", err.Error())
	}

	lvl, err := log.LvlFromString(Config.LogLevel)
	if err != nil {
		log.Crit("Log verbosity level unknown")
	}

	log.Root().SetHandler(log.LvlFilterHandler(lvl, log.StdoutHandler))

	log.Info("Starting sdns...", "version", BuildVersion)

	// delay updating the blocklists, cache until the server starts and can serve requests as the local resolver
	timer := time.NewTimer(time.Second * 1)
	go func() {
		<-timer.C
		if _, err := os.Stat("lists"); os.IsNotExist(err) || forceUpdate {
			if err := updateBlocklist(); err != nil {
				log.Crit("Update block cache failed", "error", err.Error())
			}
		}
		if err := UpdateBlockCache(); err != nil {
			log.Crit("Update block cache failed", "error", err.Error())
		}
	}()

	server := &Server{
		host:     Config.Bind,
		rTimeout: 5 * time.Second,
		wTimeout: 5 * time.Second,
	}

	server.Run()

	if err := StartAPIServer(); err != nil {
		log.Crit("Start API server failed", "error", err.Error())
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	<-c

	log.Info("Stopping sdns...")
}