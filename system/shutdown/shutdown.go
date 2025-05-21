package shutdown

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
)

func Shutdown() {
	if !env.Cfg.SafeMode {
		gpio.Deactivate(env.SystemState.MainPowerPin)
		log.Info().Msg("Main power relay deactivated")
		os.Exit(0)
	}
}

func ShutdownWithError(err error, msg string) {
	log.Error().Err(err).Msg(msg)
	Shutdown()
}
