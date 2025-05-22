package zonecontroller

import (
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func RunZoneController(zone model.Zone) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")
		for {
			// TODO: Read zone temp, compare to setpoint, activate/deactivate loop or air handler
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}
