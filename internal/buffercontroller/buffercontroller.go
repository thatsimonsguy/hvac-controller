package buffercontroller

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

type HeatSources struct {
	Primary   *model.HeatPump
	Secondary *model.HeatPump
	Tertiary  *model.Boiler
}

func RunBufferController() {
	go func() {
		log.Info().Msg("Starting buffer tank controller")

		// Sleep once at startup to honor min-off duration
		sleepDuration := time.Duration(env.Cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOff) * time.Minute
		log.Info().Dur("sleep", sleepDuration).Msg("Initial delay to avoid startup flapping")
		time.Sleep(sleepDuration)

		for {
			// refresh current source list to handle rotations and maintenance drops
			sources := refreshSources()

			// get buffer tank temp
			sensor := env.SystemState.SystemSensors["buffer_tank"]
			sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
			bufferTemp := gpio.ReadSensorTemp(sensorPath)
			log.Info().
				Str("mode", string(env.SystemState.SystemMode)).
				Float64("buffer_temp", bufferTemp).
				Msg("Evaluating buffer tank and heat sources")

			// activate or deactivate heat sources if they should be and we can
			if sources.Primary != nil {
				evaluateAndToggle(
					"primary",
					sources.Primary.Device,
					gpio.CurrentlyActive(sources.Primary.Pin),
					bufferTemp,
					env.SystemState.SystemMode,
					func() { device.ActivateHeatPump(sources.Primary) },
					func() { device.DeactivateHeatPump(sources.Primary) },
				)
			}

			if sources.Secondary != nil {
				evaluateAndToggle(
					"secondary",
					sources.Secondary.Device,
					gpio.CurrentlyActive(sources.Secondary.Pin),
					bufferTemp,
					env.SystemState.SystemMode,
					func() { device.ActivateHeatPump(sources.Secondary) },
					func() { device.DeactivateHeatPump(sources.Secondary) },
				)
			}

			if sources.Tertiary != nil {
				evaluateAndToggle(
					"tertiary",
					sources.Tertiary.Device,
					gpio.CurrentlyActive(sources.Tertiary.Pin),
					bufferTemp,
					env.SystemState.SystemMode,
					func() { device.ActivateBoiler(sources.Tertiary) },
					func() { device.DeactivateBoiler(sources.Tertiary) },
				)
			}

			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}

func evaluateAndToggle(
	role string,
	source model.Device,
	active bool,
	bufferTemp float64,
	mode model.SystemMode,
	activate func(),
	deactivate func(),
) {
	if mode == model.ModeCirculate || mode == model.ModeOff {
		return // early out for modes that don't use the buffer tank
	}

	shouldToggle := evaluateToggleSource(role, bufferTemp, active, &source, mode)

	if shouldToggle && active {
		log.Info().Str("device", source.Name).Msgf("Deactivating %s", role)
		deactivate()
	}
	if shouldToggle && !active {
		log.Info().Str("device", source.Name).Msgf("Activating %s", role)
		activate()
	}
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

var evaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
	threshold := getThreshold(role, mode, active)
	should := shouldBeOn(bt, threshold, mode)

	log.Debug().
		Str("role", role).
		Float64("buffer_temp", bt).
		Float64("threshold", threshold).
		Str("mode", string(mode)).
		Bool("currently_active", active).
		Bool("should_be_on", should).
		Msg("Evaluating toggle condition")

	if should == active {
		return false
	}

	return device.CanToggle(d, time.Now())
}

func getThreshold(role string, mode model.SystemMode, active bool) float64 {
	log.Debug().
		Str("role", role).
		Str("mode", string(mode)).
		Bool("active", active).
		Msg("Evaluating temperature threshold")

	if role != "primary" && role != "secondary" && role != "tertiary" {
		shutdown.ShutdownWithError(fmt.Errorf("invalid role definition: %s", role), "error setting temperature thresholds")
	}

	var (
		// activation and deactivation thresholds are overlapped by spread to prevent flapping
		baseHeat = env.Cfg.HeatingThreshold
		baseCool = env.Cfg.CoolingThreshold

		primaryHeatOn  = baseHeat
		primaryHeatOff = baseHeat + env.Cfg.Spread

		secondaryHeatOn  = baseHeat - env.Cfg.SecondaryMargin
		secondaryHeatOff = secondaryHeatOn + env.Cfg.Spread

		tertiaryHeatOn  = baseHeat - env.Cfg.TertiaryMargin
		tertiaryHeatOff = tertiaryHeatOn + env.Cfg.Spread

		primaryCoolOn  = baseCool
		primaryCoolOff = baseCool - env.Cfg.Spread

		secondaryCoolOn  = baseCool + env.Cfg.SecondaryMargin
		secondaryCoolOff = secondaryCoolOn - env.Cfg.Spread
	)

	switch mode {
	case model.ModeHeating:
		switch role {
		case "primary":
			if active {
				return primaryHeatOff
			}
			return primaryHeatOn
		case "secondary":
			if active {
				return secondaryHeatOff
			}
			return secondaryHeatOn
		case "tertiary":
			if active {
				return tertiaryHeatOff
			}
			return tertiaryHeatOn
		}
	case model.ModeCooling:
		switch role {
		case "primary":
			if active {
				return primaryCoolOff
			}
			return primaryCoolOn
		case "secondary":
			if active {
				return secondaryCoolOff
			}
			return secondaryCoolOn
		case "tertiary":
			shutdown.ShutdownWithError(fmt.Errorf("invalid tertiary in cooling mode"), "heat source list composition failure")
		}
	}

	// Should never be reached
	shutdown.ShutdownWithError(fmt.Errorf("unreachable path in getThreshold"), "threshold logical path error")
	return 0.0
}

func refreshSources() HeatSources {
	var newPrimary *model.HeatPump
	var newSecondary *model.HeatPump
	var newTertiary *model.Boiler

	mode := env.SystemState.SystemMode
	now := time.Now()
	sources := getHeatSources()

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

func getHeatSources() HeatSources {
	var primary *model.HeatPump
	var secondary *model.HeatPump
	var tertiary *model.Boiler
	var foundPrimary bool

	for i := range env.SystemState.HeatPumps {
		hp := &env.SystemState.HeatPumps[i]
		if hp.IsPrimary {
			if foundPrimary {
				shutdown.ShutdownWithError(fmt.Errorf("multiple heat pumps marked as primary"), "statefile validation error")
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
