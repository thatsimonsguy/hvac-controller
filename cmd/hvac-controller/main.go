package main

import (
	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
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

	// @todo: start controller loop
	// @todo: start HTTP server

	select {} // block forever for now
}
