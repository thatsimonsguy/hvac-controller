package controller

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

type HeatSources struct {
	Primary   *model.HeatPump
	Secondary *model.HeatPump
	Tertiary  *model.Boiler
}

func RunBufferController(cfg *config.Config, state *state.SystemState) {
	go func() {
		log.Info().Msg("Starting buffer tank controller")
		for {
			// refresh current source list to handle rotations and maintenance drops
			sources := refreshSources(cfg, state)

			// get buffer tank temp
			sensor := state.SystemSensors["buffer_tank"]
			sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
			bufferTemp := gpio.ReadSensorTemp(sensorPath)

			// activate or deactivate heat sources if they should be and we can
			if sources.Primary != nil {
				active := gpio.CurrentlyActive(sources.Primary.Pin)
				togglePrimary := evaluateToggleSource("primary", bufferTemp, active, cfg, &sources.Primary.Device, state.SystemMode)

				if togglePrimary && active {
					log.Info().Str("device", sources.Primary.Name).Msg("Deactivating primary heat pump")
					device.DeactivateHeatPump(sources.Primary)
				}
				if togglePrimary && !active {
					log.Info().Str("device", sources.Primary.Name).Msg("Activating primary heat pump")
					device.ActivateHeatPump(sources.Primary)
				}
			}

			if sources.Secondary != nil {
				active := gpio.CurrentlyActive(sources.Secondary.Pin)
				togglePrimary := evaluateToggleSource("secondary", bufferTemp, active, cfg, &sources.Secondary.Device, state.SystemMode)

				if togglePrimary && active {
					log.Info().Str("device", sources.Primary.Name).Msg("Deactivating secondary heat pump")
					device.DeactivateHeatPump(sources.Secondary)
				}
				if togglePrimary && !active {
					log.Info().Str("device", sources.Primary.Name).Msg("Activating secondary heat pump")
					device.ActivateHeatPump(sources.Secondary)
				}
			}

			if sources.Tertiary != nil {
				active := gpio.CurrentlyActive(sources.Tertiary.Pin)
				togglePrimary := evaluateToggleSource("tertiary", bufferTemp, active, cfg, &sources.Tertiary.Device, state.SystemMode)

				if togglePrimary && active {
					log.Warn().Str("device", sources.Primary.Name).Msg("Deactivating boiler as tertiary heat source")
					device.DeactivateBoiler(sources.Tertiary)
				}
				if togglePrimary && !active {
					log.Warn().Str("device", sources.Primary.Name).Msg("Activating boiler as tertiary heat source")
					device.ActivateBoiler(sources.Tertiary)
				}
			}

			time.Sleep(time.Duration(cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}

func shouldBeOn(bt float64, threshold float64, mode model.SystemMode) bool {
	switch mode {
	case model.ModeHeating:
		return bt < threshold
	case model.ModeCooling:
		return bt > threshold
	default:
		return false
	}
}

func evaluateToggleSource(role string, bt float64, active bool, cfg *config.Config, d *model.Device, mode model.SystemMode) bool {
	threshold := getThreshold(role, cfg, mode)
	should := shouldBeOn(bt, threshold, mode)

	if should == active {
		// Already in the correct state, no toggle needed
		return false
	}

	return device.CanToggle(d, time.Now())
}

func getThreshold(role string, cfg *config.Config, mode model.SystemMode) float64 {
	if role != "primary" && role != "secondary" && role != "tertiary" {
		log.Fatal().Err(fmt.Errorf("invalid role definition: %s", role))
	}

	switch mode {
	case model.ModeHeating:
		switch role {
		case "primary":
			return cfg.HeatingThreshold
		case "secondary":
			return cfg.HeatingThreshold - cfg.SecondaryMargin
		case "tertiary":
			return cfg.HeatingThreshold - cfg.TertiaryMargin
		}
	case model.ModeCooling:
		switch role {
		case "primary":
			return cfg.CoolingThreshold
		case "secondary":
			return cfg.CoolingThreshold + cfg.SecondaryMargin
		case "tertiary":
			log.Fatal().Err(fmt.Errorf("invalid tertiary in cooling mode"))
		}
	}

	// Should never be reached
	log.Fatal().Msg("unreachable path in getThreshold")
	return 0.0
}

func refreshSources(cfg *config.Config, state *state.SystemState) HeatSources {
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
		log.Warn().Msg("No eligible heat sources are online.")
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

func RunZoneController(zone model.Zone, cfg *config.Config, state *state.SystemState) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")
		for {
			// TODO: Read zone temp, compare to setpoint, activate/deactivate loop or air handler
			time.Sleep(time.Duration(cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}

func GetHeatSources(state *state.SystemState) HeatSources {
	var primary *model.HeatPump
	var secondary *model.HeatPump
	var tertiary *model.Boiler
	var foundPrimary bool

	for i := range state.HeatPumps {
		hp := &state.HeatPumps[i]
		if hp.IsPrimary {
			if foundPrimary {
				log.Fatal().Msg("Multiple heat pumps marked as primary")
			}
			foundPrimary = true
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
