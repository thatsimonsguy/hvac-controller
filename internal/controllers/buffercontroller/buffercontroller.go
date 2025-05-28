package buffercontroller

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/datadog"
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

type HeatSourcesProvider interface {
	GetHeatSources(dbConn *sql.DB) HeatSources
}

type SourceRefresher struct {
	Provider HeatSourcesProvider
}

type RealProvider struct{}

func (RealProvider) GetHeatSources(dbConn *sql.DB) HeatSources {
	return GetHeatSources(dbConn)
}

func RunBufferController(dbConn *sql.DB) {
	go func() {
		log.Info().Msg("Starting buffer tank controller")

		sensor, err := db.GetSensorByID(dbConn, "buffer_tank")
		if err != nil {
			log.Error().Err(err).Str("sensor id", "buffer_tank").Msg("Could not retrieve sensor")
		}

		// Create SourceRefresher with the real provider
		refresher := SourceRefresher{Provider: RealProvider{}}

		// Sleep once at startup to honor min-off duration
		sleepDuration := time.Duration(env.Cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOff) * time.Minute
		log.Info().Dur("sleep", sleepDuration).Msg("Initial delay to avoid startup flapping")
		time.Sleep(sleepDuration)

		for {
			// refresh current source list to handle rotations and maintenance drops
			sources := refresher.RefreshSources(dbConn)

			// get buffer tank temp
			sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
			bufferTemp := gpio.ReadSensorTempWithRetries(sensorPath, 5)

			datadog.Gauge("buffer_tank.temperature", bufferTemp, "component:sensor")

			// get system mode
			mode, err := db.GetSystemMode(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could nor retrieve system mode from db")
			}

			log.Info().
				Str("mode", string(mode)).
				Float64("buffer_temp", bufferTemp).
				Msg("Evaluating buffer tank and heat sources")

			// activate or deactivate heat sources if they should be and we can
			if sources.Primary != nil {
				EvaluateAndToggle(
					"primary",
					sources.Primary.Device,
					gpio.CurrentlyActive(sources.Primary.Pin),
					bufferTemp,
					mode,
					func() { device.ActivateHeatPump(sources.Primary) },
					func() { device.DeactivateHeatPump(sources.Primary) },
				)
			}

			if sources.Secondary != nil {
				EvaluateAndToggle(
					"secondary",
					sources.Secondary.Device,
					gpio.CurrentlyActive(sources.Secondary.Pin),
					bufferTemp,
					mode,
					func() { device.ActivateHeatPump(sources.Secondary) },
					func() { device.DeactivateHeatPump(sources.Secondary) },
				)
			}

			if sources.Tertiary != nil {
				EvaluateAndToggle(
					"tertiary",
					sources.Tertiary.Device,
					gpio.CurrentlyActive(sources.Tertiary.Pin),
					bufferTemp,
					mode,
					func() { device.ActivateBoiler(sources.Tertiary) },
					func() { device.DeactivateBoiler(sources.Tertiary) },
				)
			}

			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)
		}
	}()
}

func EvaluateAndToggle(
	role string,
	source model.Device,
	active bool,
	bufferTemp float64,
	mode model.SystemMode,
	activate func(),
	deactivate func(),
) {
	shouldToggle := EvaluateToggleSource(role, bufferTemp, active, &source, mode)

	if shouldToggle && active {
		log.Info().Str("device", source.Name).Msgf("Deactivating %s", role)
		deactivate()
	}
	if shouldToggle && !active {
		log.Info().Str("device", source.Name).Msgf("Activating %s", role)
		activate()
	}
}

func ShouldBeOn(bt float64, threshold float64, mode model.SystemMode) bool {
	switch mode {
	case model.ModeHeating:
		return bt < threshold
	case model.ModeCooling:
		return bt > threshold
	default:
		return false
	}
}

var EvaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
	threshold := GetThreshold(role, mode, active)
	should := ShouldBeOn(bt, threshold, mode)

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

func GetThreshold(role string, mode model.SystemMode, active bool) float64 {
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
	default: // return value does not matter for off or circulate
		return 0.0
	}

	// Should never be reached
	shutdown.ShutdownWithError(fmt.Errorf("unreachable path in getThreshold"), "threshold logical path error")
	return 0.0
}

func (r *SourceRefresher) RefreshSources(dbConn *sql.DB) HeatSources {
	var newPrimary *model.HeatPump
	var newSecondary *model.HeatPump
	var newTertiary *model.Boiler

	mode, err := db.GetSystemMode(dbConn)
	if err != nil {
		shutdown.ShutdownWithError(err, "Could not get system mode")
	}

	now := time.Now()
	sources := r.Provider.GetHeatSources(dbConn)

	offlineCool := !sources.Primary.Online && !sources.Secondary.Online && mode == model.ModeCooling
	offlineHeat := !sources.Primary.Online && !sources.Secondary.Online && !sources.Tertiary.Online
	if offlineCool || offlineHeat {
		log.Warn().Msg("No eligible heat sources are online.")
	}

	if sources.Primary.Online && sources.Secondary.Online {
		if now.Sub(sources.Primary.LastRotated) > time.Duration(env.Cfg.RoleRotationMinutes)*time.Minute {
			log.Info().Msgf("Rotating heat pump primary from %s to %s", sources.Primary.Name, sources.Secondary.Name)
			newPrimary = sources.Secondary
			newSecondary = sources.Primary

			err = db.SwapPrimaryHeatPump(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not swap primary and secondary heatpumps")
			}
		} else {
			newPrimary = sources.Primary
			newSecondary = sources.Secondary
		}
	}

	if sources.Primary.Online != sources.Secondary.Online {
		if sources.Primary.Online {
			newPrimary = sources.Primary
		} else {
			newPrimary = sources.Secondary
		}
		newSecondary = nil
	}

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

var GetHeatSources = func(dbConn *sql.DB) HeatSources {
	var primary *model.HeatPump
	var secondary *model.HeatPump
	var tertiary *model.Boiler
	var foundPrimary bool

	hps, err := db.GetHeatPumps(dbConn)
	if err != nil {
		shutdown.ShutdownWithError(err, "Could not retrieve heat pumps from db")
	}

	// Identify primary and secondary heat pumps and check for duplication
	for i := range hps {
		hp := hps[i]
		if hp.IsPrimary {
			if foundPrimary {
				shutdown.ShutdownWithError(fmt.Errorf("multiple heat pumps marked as primary"), "state validation error")
			}
			foundPrimary = true
			primary = &hp
		} else {
			secondary = &hp
		}
	}
	if !foundPrimary {
		shutdown.ShutdownWithError(fmt.Errorf("no heat pumps marked as primary"), "state validation error")
	}

	boilers, err := db.GetBoilers(dbConn)
	if err != nil {
		shutdown.ShutdownWithError(err, "Could not retrieve boilers from db")
	}

	if len(boilers) > 0 && boilers[0].Online {
		tertiary = &boilers[0]
	}

	return HeatSources{
		Primary:   primary,
		Secondary: secondary,
		Tertiary:  tertiary,
	}
}
