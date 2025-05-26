package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/buffercontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/zonecontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/datadog"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/logging"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
	"github.com/thatsimonsguy/hvac-controller/system/startup"
)

func main() {
	env.Cfg = config.Load()
	logging.Init(env.Cfg.LogLevel)

	if env.Cfg.EnableDatadog {
		datadog.InitMetrics()
	}

	// Initialize the DB
	db.InitConfig(env.Cfg)
	if err := db.InitializeIfMissing(); err != nil {
		shutdown.ShutdownWithError(err, "Failed to initialize database")
	}
	if err := db.ValidateDatabase(); err != nil {
		shutdown.ShutdownWithError(err, "Failed to validate database")
	}

	log.Info().
		Str("state_file", env.Cfg.StateFilePath).
		Msg("Starting HVAC controller")

	gpio.SetSafeMode(env.Cfg.SafeMode)
	if env.Cfg.SafeMode {
		log.Warn().Msg("SAFE MODE ENABLED — GPIO Set() is disabled system-wide")
	}

	state.Init(env.Cfg)
	var loadErr error
	env.SystemState, loadErr = state.LoadSystemState(env.Cfg.StateFilePath)
	if loadErr != nil {
		log.Warn().Err(loadErr).Msg("Failed to load existing system state, starting with defaults")
		// indicates first run
		env.SystemState = state.NewSystemStateFromConfig() // create state file from config
		state.SaveSystemState(env.Cfg.StateFilePath, env.SystemState)

		// write a pinctrl shell script to disk that sets initial pin states, run it now, and install it as a service
		// context: pin states can float or fluctuate during device boot, so we're setting them as early as possible to their off states via systemd service
		startup.WriteStartupScript()
		startup.RunStartupScript()
		startup.InstallStartupService()
	}

	log.Info().
		Str("mode", string(env.SystemState.SystemMode)).
		Int("zones", len(env.SystemState.Zones)).
		Msg("Loaded system state")

	if err := gpio.ValidateInitialPinStates(); err != nil {
		shutdown.ShutdownWithError(err, "Refusing to enable relay board due to unsafe pin states")
	}

	if !env.Cfg.SafeMode {
		gpio.Activate(env.SystemState.MainPowerPin)
	}

	for _, zone := range env.SystemState.Zones {
		zonecontroller.RunZoneController(&zone)
	}
	buffercontroller.RunBufferController()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig
	log.Info().Msg("Shutdown signal received — exiting")
	shutdown.Shutdown()

	// @todo: start HTTP server

	select {} // block forever for now
}
