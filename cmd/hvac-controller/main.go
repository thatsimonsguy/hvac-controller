package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/controller"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/logging"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/store"
)

func main() {
	cfg := config.Load()
	logging.Init(cfg.LogLevel)

	log.Info().
		Str("state_file", cfg.StateFile).
		Msg("Starting HVAC controller")

	gpio.SetSafeMode(cfg.SafeMode)
	if cfg.SafeMode {
		log.Warn().Msg("SAFE MODE ENABLED — GPIO Set() is disabled system-wide")
	}

	if err := gpio.ValidateStartupPins(cfg); err != nil {
		log.Fatal().Err(err).Msg("Refusing to enable relay board due to unsafe pin states")
	}

	st := store.New(cfg.StateFile)

	state, err := st.Load()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load existing system state, starting with defaults")
		state = &model.SystemState{
			SystemMode: model.ModeOff,
			Zones:      []model.Zone{},
		}
	}

	log.Info().
		Str("mode", string(state.SystemMode)).
		Int("zones", len(state.Zones)).
		Msg("Loaded system state")

	ctrl := controller.New(cfg, state)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig
	log.Info().Msg("Shutdown signal received — exiting")

	// @todo: start HTTP server

	select {} // block forever for now
}
