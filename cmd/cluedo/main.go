package main

import (
	"flag"
	"math/rand"
	"os"
	"time"

	"cluedo-toolbox/internal/cli"
	"cluedo-toolbox/internal/config"

	"github.com/sirupsen/logrus"
)

func main() {
	// 1. Parse command-line flags
	logLevel := flag.String("loglevel", "info", "Set logging level (debug, info, warn, error)")
	flag.Parse()

	// 2. Set up top-level dependencies (Logger)
	log := logrus.New()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, ForceColors: true})

	// 3. Load game configuration
	gameConfig, err := config.Load("default_config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 4. Create the CLI, injecting the logger
	ui := cli.NewCLI(log)

	// 5. Run the application
	// We pass the args and a new random source for this run.
	randSource := rand.New(rand.NewSource(time.Now().UnixNano()))
	if err := ui.Run(flag.Args(), gameConfig, randSource); err != nil {
		log.Errorf("Application exited with error: %v", err)
		os.Exit(1)
	}
}
