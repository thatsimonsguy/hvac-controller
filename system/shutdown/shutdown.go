package shutdown

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/pinctrl"
)

func Shutdown() {
	if !env.Cfg.SafeMode {
		if env.Cfg.MainPowerActiveHigh {
			pinctrl.SetPin(env.Cfg.MainPowerGPIO, "op", "pn", "dl")
		} else {
			pinctrl.SetPin(env.Cfg.MainPowerGPIO, "op", "pn", "dh")
		}
		log.Info().Msg("Main power relay deactivated")
		os.Exit(0)
	}
}

func ShutdownWithError(err error, msg string) {
	log.Error().Err(err).Msg(msg)
	Shutdown()
}
