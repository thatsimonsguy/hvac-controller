package controller

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
)

type HeatSources struct {
	Primary   *model.HeatPump
	Secondary *model.HeatPump
	Tertiary  *model.Boiler
}

func RunBufferController() {
	go func() {
		log.Info().Msg("Starting buffer tank controller")
		for {
			// refresh current source list to handle rotations and maintenance drops
			sources := refreshSources()

			// get buffer tank temp
			sensor := env.SystemState.SystemSensors["buffer_tank"]
			sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
			bufferTemp := gpio.ReadSensorTemp(sensorPath)

			// activate or deactivate heat sources if they should be and we can
			if sources.Primary != nil {
				active := gpio.CurrentlyActive(sources.Primary.Pin)
				togglePrimary := evaluateToggleSource("primary", bufferTemp, active, &sources.Primary.Device, env.SystemState.SystemMode)

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
				togglePrimary := evaluateToggleSource("secondary", bufferTemp, active, &sources.Secondary.Device, env.SystemState.SystemMode)

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
				togglePrimary := evaluateToggleSource("tertiary", bufferTemp, active, &sources.Tertiary.Device, env.SystemState.SystemMode)

				if togglePrimary && active {
					log.Warn().Str("device", sources.Primary.Name).Msg("Deactivating boiler as tertiary heat source")
					device.DeactivateBoiler(sources.Tertiary)
				}
				if togglePrimary && !active {
					log.Warn().Str("device", sources.Primary.Name).Msg("Activating boiler as tertiary heat source")
					device.ActivateBoiler(sources.Tertiary)
				}
			}

			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)
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

func evaluateToggleSource(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
	threshold := getThreshold(role, mode)
	should := shouldBeOn(bt, threshold, mode)

	if should == active {
		// Already in the correct state, no toggle needed
		return false
	}

	return device.CanToggle(d, time.Now())
}

func getThreshold(role string, mode model.SystemMode) float64 {
	if role != "primary" && role != "secondary" && role != "tertiary" {
		log.Fatal().Err(fmt.Errorf("invalid role definition: %s", role))
	}

	switch mode {
	case model.ModeHeating:
		switch role {
		case "primary":
			return env.Cfg.HeatingThreshold
		case "secondary":
			return env.Cfg.HeatingThreshold - env.Cfg.SecondaryMargin
		case "tertiary":
			return env.Cfg.HeatingThreshold - env.Cfg.TertiaryMargin
		}
	case model.ModeCooling:
		switch role {
		case "primary":
			return env.Cfg.CoolingThreshold
		case "secondary":
			return env.Cfg.CoolingThreshold + env.Cfg.SecondaryMargin
		case "tertiary":
			log.Fatal().Err(fmt.Errorf("invalid tertiary in cooling mode"))
		}
	}

	// Should never be reached
	log.Fatal().Msg("unreachable path in getThreshold")
	return 0.0
}

func refreshSources() HeatSources {
	var newPrimary *model.HeatPump
	var newSecondary *model.HeatPump
	var newTertiary *model.Boiler

	mode := env.SystemState.SystemMode
	now := time.Now()
	sources := GetHeatSources()

	// verify that we have some heat source we can use in our current mode
	offlineCool := !sources.Primary.Online && !sources.Secondary.Online && mode == model.ModeCooling
	offlineHeat := !sources.Primary.Online && !sources.Secondary.Online && !sources.Tertiary.Online
	if offlineCool || offlineHeat {
		log.Warn().Msg("No eligible heat sources are online.")
	}

	// happy path: both heat pumps online
	if sources.Primary.Online && sources.Secondary.Online {
		if now.Sub(sources.Primary.LastRotated) > time.Duration(env.Cfg.RoleRotationMinutes)*time.Minute {
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

func RunZoneController(zone model.Zone) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")
		for {
			// TODO: Read zone temp, compare to setpoint, activate/deactivate loop or air handler
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}

func GetHeatSources() HeatSources {
	var primary *model.HeatPump
	var secondary *model.HeatPump
	var tertiary *model.Boiler
	var foundPrimary bool

	for i := range env.SystemState.HeatPumps {
		hp := &env.SystemState.HeatPumps[i]
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
	if len(env.SystemState.Boilers) > 0 {
		tertiary = &env.SystemState.Boilers[0]
	}

	return HeatSources{
		Primary:   primary,
		Secondary: secondary,
		Tertiary:  tertiary,
	}
}
