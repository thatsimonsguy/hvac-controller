package main

import (
	"database/sql"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/api"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/buffercontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/failsafecontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/recirculationcontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/zonecontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/datadog"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/notifications"
	"github.com/thatsimonsguy/hvac-controller/internal/temperature"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/logging"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
	"github.com/thatsimonsguy/hvac-controller/system/startup"
)

func main() {
	env.Cfg = config.Load()
	logging.Init(env.Cfg.LogLevel)

	if env.Cfg.EnableDatadog {
		datadog.InitMetrics()
	}

	// Initialize notifications
	notifications.Init()

	// Initialize the DB
	db.InitConfig(env.Cfg)
	firstRun, err := db.InitializeIfMissing()
	if err != nil {
		shutdown.ShutdownWithError(err, "Failed to initialize database")
	}
	if err := db.ValidateDatabase(); err != nil {
		shutdown.ShutdownWithError(err, "Failed to validate database")
	}

	// Create DB connection
	dbConn, err := sql.Open("sqlite3", env.Cfg.DBPath)
	if err != nil {
		shutdown.ShutdownWithError(err, "Failed to connect to database")
	}
	defer dbConn.Close()

	log.Info().Msg("Starting HVAC controller")

	// Ensure services are properly installed and enabled on every run
	if err := startup.EnsureServicesReady(dbConn); err != nil {
		shutdown.ShutdownWithError(err, "Failed to ensure services are ready")
	}

	if firstRun {
		// Run the startup script now to set initial pin states
		// context: pin states can float or fluctuate during device boot, so we're setting them as early as possible to their off states
		if err := startup.RunStartupScript(); err != nil {
			shutdown.ShutdownWithError(err, "Failed to run startup script")
		}
	}

	if err := gpio.ValidateInitialPinStates(dbConn); err != nil {
		shutdown.ShutdownWithError(err, "Failed to initialize pin states")
	}

	mainPowerPin, err := db.GetMainPowerPin(dbConn)
	if err != nil {
		shutdown.ShutdownWithError(err, "could not retrieve main power pin from db")
	}
	gpio.Activate(mainPowerPin) // Turn on the relay board

	// Start centralized temperature reading service
	tempService := temperature.NewService(dbConn, env.Cfg.PollIntervalSeconds)
	tempService.Start()

	zones, err := db.GetAllZones(dbConn)
	if err != nil {
		shutdown.ShutdownWithError(err, "could not get zones from db")
	}

	for _, zone := range zones {
		zonecontroller.RunZoneController(&zone, dbConn, tempService)
	}
	
	// Stagger controller startups to avoid CPU spikes and race conditions
	time.Sleep(3 * time.Second)
	buffercontroller.RunBufferController(dbConn, tempService)
	
	time.Sleep(3 * time.Second)
	recirculationcontroller.RunRecirculationController(dbConn)
	
	time.Sleep(3 * time.Second)
	failsafecontroller.RunFailsafeController(dbConn, tempService)

	// Start REST API server
	apiServer := api.NewServer(dbConn, tempService, env.Cfg)
	go func() {
		if err := apiServer.Start(8080); err != nil {
			log.Error().Err(err).Msg("REST API server failed to start")
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig
	log.Info().Msg("Shutdown signal received — exiting")
	shutdown.Shutdown()
}
