package buffercontroller

import (
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/system/startup"
)

func SetSystemMode(dbConn *sql.DB, mode model.SystemMode) error {
	hps, err := db.GetHeatPumps(dbConn)
	if err != nil {
		return err
	}

	swapped := false
	for i := range hps {
		hp := hps[i]

		pumpActive := gpio.CurrentlyActive(hp.Pin) // if we need to shift the mode pin but the pump is active, we need to turn off the pump, wait the minoff, then switch the mode pin
		modeActive := gpio.CurrentlyActive(hp.ModePin)
		canToggle := device.CanToggle(&hp.Device, time.Now())
		online := hp.Online
		should := ShouldToggle(pumpActive,
			modeActive,
			mode,
			canToggle,
			online,
			hp.MinOn,
			func() { device.DeactivateHeatPump(&hp) },
			time.Sleep)

		if should && modeActive {
			gpio.Deactivate(hp.ModePin) // sys mode in heating, off, or circ and mode pin is on (cooling)
			log.Info().Str("heat pump", hp.Name).Msg("switching mode pin FROM cooling TO heating")
			swapped = true
		}
		if should && !modeActive {
			gpio.Activate(hp.ModePin) // sys mode is in cooling and mode pin is off
			log.Info().Str("heat pump", hp.Name).Msg("switching mode pin FROM heating TO cooling")
			swapped = true
		}
	}

	// rewrite the startup script to reflect the current state of the DB so validation will pass on reboot if there's a crash or power failure
	if swapped {
		if err := startup.WriteStartupScript(dbConn); err != nil {
			log.Error().Err(err).Msg("failed to rewrite pinsetter script")
			return err
		}
	}

	return nil // mode pins are now aligned with system mode
}

func ShouldToggle(pumpActive bool,
	modeActive bool,
	sysMode model.SystemMode,
	canToggle bool,
	online bool,
	minOn time.Duration,
	pumpDeactivate func(),
	sleepFunc func(time.Duration)) bool {

	log.Debug().Bool("pump_active", pumpActive).Bool("hp_mode_cooling", modeActive).Str("system_mode", string(sysMode)).
		Bool("can_toggle", canToggle).Bool("hp_online", online).Float64("min_on", minOn.Minutes())

	// evaluate whether the mode needs to change
	fromCooling := modeActive && sysMode != model.ModeCooling
	fromHeatingAndOnline := !modeActive && sysMode == model.ModeCooling && online // don't switch into cooling if the pump isn't online

	if !(fromCooling || fromHeatingAndOnline) {
		return false // early out if the heat pump mode is aligned with system mode
	}

	// pump should not be running when we toggle system mode from heating to cooling or vice versa
	if pumpActive {
		if !canToggle {
			sleepFunc(minOn)
		}
		pumpDeactivate()
		sleepFunc(30 * time.Second)
	}

	return true
}
