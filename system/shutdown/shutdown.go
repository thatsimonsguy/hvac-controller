package shutdown

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

func Shutdown(state *state.SystemState, cfg *config.Config) {
	if !cfg.SafeMode {
		gpio.Deactivate(state.MainPowerPin)
		log.Info().Msg("Main power relay deactivated")
		os.Exit(0)
	}
}

func ShutdownWithError(state *state.SystemState, cfg *config.Config, err error, msg string) {
	log.Error().Err(err).Msg(msg)
	Shutdown(state, cfg)
}
