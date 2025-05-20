package controller

import (
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

type HeatSources struct {
	Primary   *model.HeatPump
	Secondary *model.HeatPump
	Tertiary  *model.Boiler
}

func RunBufferController(cfg *config.Config, state *state.SystemState, pollInterval time.Duration) {
	go func() {
		log.Info().Msg("Starting buffer tank controller")
		for {
			// build current source list
			sources := assignHeatSourceRoles(cfg, state)

			// get buffer tank temp
			sensor := state.SystemSensors["buffer_tank"]
			sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
			bufferTemp, err := gpio.ReadSensorTemp(sensorPath)
			if err != nil {
				log.Error().Err(err).Msg("Failed to read buffer tank temperature")
			}

			// read device active states for online devices

			time.Sleep(time.Duration(cfg.PollIntervalSeconds))
		}
	}()
}

func assignHeatSourceRoles(cfg *config.Config, state *state.SystemState) HeatSources {
	var newPrimary *model.HeatPump
	var newSecondary *model.HeatPump
	var newTertiary *model.Boiler

	mode := state.SystemMode
	now := time.Now()
	sources := GetHeatSources(state)

	// verify that we have some heat source we can use in our current mode
	offlineCool := !sources.Primary.Online && !sources.Secondary.Online && mode == model.ModeCooling
	offlineHeat := !sources.Primary.Online && !sources.Secondary.Online && !sources.Tertiary.Online
	if offlineCool || offlineHeat {
		panic("No eligible heat sources are online")
	}

	// happy path: both heat pumps online
	if sources.Primary.Online && sources.Secondary.Online {
		if now.Sub(sources.Primary.LastRotated) > time.Duration(cfg.RoleRotationMinutes)*time.Minute {
			log.Info().Msgf("Rotating heat pump primary from %s to %s", sources.Primary.Name, sources.Secondary.Name)
			sources.Primary.IsPrimary = false
			sources.Secondary.IsPrimary = true
			sources.Primary.LastRotated = now
			sources.Secondary.LastRotated = now
			newPrimary = sources.Secondary
			newSecondary = sources.Primary
		} else {
			newPrimary = sources.Primary
			newSecondary = sources.Secondary
		}
	}

	// one heat pump offline
	if sources.Primary.Online != sources.Secondary.Online {
		if sources.Primary.Online {
			newPrimary = sources.Primary
		} else {
			newPrimary = sources.Secondary
		}
		newSecondary = nil
	}

	// both heat pumps offline
	if !sources.Primary.Online && !sources.Secondary.Online {
		newPrimary = nil
		newSecondary = nil
	}

	if mode == model.ModeCooling || sources.Tertiary == nil || !sources.Tertiary.Online {
		newTertiary = nil
	} else {
		newTertiary = sources.Tertiary
	}

	return HeatSources{
		Primary:   newPrimary,
		Secondary: newSecondary,
		Tertiary:  newTertiary,
	}
}

func RunZoneController(zone model.Zone, state *state.SystemState, pollInterval time.Duration) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")
		for {
			// TODO: Read zone temp, compare to setpoint, activate/deactivate loop or air handler
			time.Sleep(pollInterval)
		}
	}()
}

func GetHeatSources(state *state.SystemState) HeatSources {
	var primary *model.HeatPump
	var secondary *model.HeatPump
	var tertiary *model.Boiler

	for i := range state.HeatPumps {
		hp := &state.HeatPumps[i]
		if hp.IsPrimary {
			primary = hp
		} else {
			secondary = hp
		}
	}
	if len(state.Boilers) > 0 {
		tertiary = &state.Boilers[0]
	}

	return HeatSources{
		Primary:   primary,
		Secondary: secondary,
		Tertiary:  tertiary,
	}
}
