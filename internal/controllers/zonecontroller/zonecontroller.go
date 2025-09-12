package zonecontroller

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/datadog"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

const ZoneSpread float64 = 0.5
const HeatingSecondaryThreshold float64 = 3

type TemperatureService interface {
	GetTemperature(sensorID string) (float64, bool)
}

func RunZoneController(zone *model.Zone, dbConn *sql.DB, tempService TemperatureService) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")

		sensor, err := db.GetSensorByID(dbConn, zone.Sensor.ID)
		if err != nil {
			log.Error().Err(err).Str("sensor id", zone.Sensor.ID).Msg("Could not retrieve sensor")
		}

		// Sleep for 3 mins at first run, relatively safe assumed minOff
		jitter := time.Duration(rand.Intn(10000)) * time.Millisecond // stagger cycle activation for all async routines
		time.Sleep(3*time.Minute + jitter)

		for {
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)

			// Check if system is in override mode - if so, skip normal zone control
			overrideActive, err := db.GetSystemOverride(dbConn)
			if err != nil {
				log.Error().Err(err).Str("zone", zone.ID).Msg("Could not check override status")
				continue
			}
			if overrideActive {
				log.Debug().Str("zone", zone.ID).Msg("System override active - skipping zone control")
				continue
			}

			// Refresh zone from db
			zone, err = db.GetZoneByID(dbConn, zone.ID)
			if err != nil {
				log.Error().Err(err).Str("zone", zone.ID).Msg("Could not retrieve zone from db")
				continue
			}

			// Get temp
			zoneTemp, valid := tempService.GetTemperature(sensor.ID)
			if !valid {
				log.Warn().Str("zone", zone.ID).Msg("No valid temperature reading available for zone")
				continue
			}

			// Log out temp TODO: move this into o11y routine
			log.Info().Str("zone", zone.ID).Str("mode", string(zone.Mode)).Float64("temp", zoneTemp).Msg("Evaluating zone")
			datadog.Gauge("zone.temperature", zoneTemp, "component:sensor", fmt.Sprintf("zone:%s", zone.ID))

			// Get system mode
			sysMode, err := db.GetSystemMode(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not retrieve system mode from db")
			}

			// Get distribution devices

			handler, err := db.GetAirHandlerByID(dbConn, zone.ID)
			if err != nil {
				log.Error().Err(err).Str("zone", zone.ID).Msg("could not retrieve air handler for zone")
			}
			loop, err := db.GetRadiantLoopByID(dbConn, zone.ID)
			if err != nil {
				log.Error().Err(err).Str("zone", zone.ID).Msg("could not retrieve radiant loop for zone")
			}

			// Get toggleable statuses
			canToggleHandler := false
			canToggleLoop := false

			if handler != nil {
				canToggleHandler = device.CanToggle(&handler.Device, time.Now())
			}
			if loop != nil {
				canToggleLoop = device.CanToggle(&loop.Device, time.Now())
			}

			// Get active states for devices
			blowerActive := false
			pumpActive := false
			loopActive := false

			if handler != nil {
				blowerActive = gpio.CurrentlyActive(handler.Pin)
				pumpActive = gpio.CurrentlyActive(handler.CircPumpPin)
			}
			if loop != nil {
				loopActive = gpio.CurrentlyActive(loop.Pin)
			}

			threshold := getThreshold(zone, pumpActive, false)
			secondaryThreshold := getThreshold(zone, pumpActive, true)

			datadog.Gauge("zone.temperature", zoneTemp, "component:sensor", fmt.Sprintf("zone:%s", zone.ID))

			// Log out temp
			log.Debug().
				Str("zone", zone.Label).
				Float64("temp", zoneTemp).
				Float64("setpoint", zone.Setpoint).
				Float64("threshold", threshold).
				Str("mode", string(zone.Mode)).
				Msg("Zone temperature check")

			// Run evaluation logic
			switchMap, err := evaluateZoneActions(
				zone.ID,
				blowerActive,
				pumpActive,
				loopActive,
				handler,
				loop,
				canToggleHandler,
				canToggleLoop,
				zoneTemp,
				zone.Mode,
				sysMode,
				threshold,
				secondaryThreshold,
			)
			if err != nil {
				log.Error().Err(err).Str("zone", zone.ID).Msg("zone evaluation failure")

				log.Debug().Err(err).Str("zone", zone.ID).
					Bool("blower_active", blowerActive).
					Bool("pump_active", pumpActive).
					Bool("loop_active", loopActive).
					Bool("handler_is_nil", handler == nil).
					Bool("loop_is_nil", loop == nil).
					Bool("can_toggle_handler", canToggleHandler).
					Bool("can_toggle_loop", canToggleLoop).
					Float64("zone_temp", zoneTemp).
					Str("zone_mode", string(zone.Mode)).
					Str("system_mode", string(sysMode)).
					Float64("thresdhold", threshold).
					Float64("secondary_threshold", secondaryThreshold).Msg("debug: zone evaluation failure")

				// turn everything off (but respect recirculation override for blower)
				if handler != nil {
					device.DeactivateAirHandler(handler, dbConn)
					
					// Check if recirculation is active before deactivating blower
					recircActive, _, err := db.GetRecirculationStatus(dbConn)
					if err != nil {
						log.Error().Err(err).Msg("Failed to check recirculation status")
					}
					if !recircActive {
						device.DeactivateBlower(handler, dbConn)
					} else {
						log.Debug().Str("zone", zone.ID).Msg("Skipping blower deactivation - recirculation active")
					}
				}
				if loop != nil {
					device.DeactivateRadiantLoop(loop, dbConn)
				}
				continue
			}

			// no errors, so use our switchMap to turn things off and on
			if handler != nil {
				if switchMap["activate_blower"] {
					device.ActivateBlower(handler, dbConn)
				}
				if switchMap["deactivate_blower"] {
					// Check if recirculation is active before deactivating blower
					recircActive, _, err := db.GetRecirculationStatus(dbConn)
					if err != nil {
						log.Error().Err(err).Msg("Failed to check recirculation status")
					}
					if !recircActive {
						device.DeactivateBlower(handler, dbConn)
					} else {
						log.Debug().Str("zone", zone.ID).Msg("Skipping blower deactivation - recirculation active")
					}
				}
				if switchMap["activate_pump"] {
					device.ActivateAirHandler(handler, dbConn)
				}
				if switchMap["deactivate_pump"] {
					device.DeactivateAirHandler(handler, dbConn)
				}
			}

			if loop != nil {
				if switchMap["activate_loop"] {
					device.ActivateRadiantLoop(loop, dbConn)
				}
				if switchMap["deactivate_loop"] {
					device.DeactivateRadiantLoop(loop, dbConn)
				}
			}

		}
	}()
}

func evaluateZoneActions(
	zoneID string,
	blowerActive bool,
	pumpActive bool,
	loopActive bool,
	handler *model.AirHandler,
	loop *model.RadiantFloorLoop,
	canToggleHandler bool,
	canToggleLoop bool,
	temp float64,
	mode model.SystemMode,
	sysMode model.SystemMode,
	threshold float64,
	secondaryThreshold float64,
) (map[string]bool, error) {
	switchThings := map[string]bool{
		"activate_blower":   false,
		"activate_pump":     false,
		"activate_loop":     false,
		"deactivate_blower": false,
		"deactivate_pump":   false,
		"deactivate_loop":   false,
	}

	// turn everythuing off if zone is off
	if mode == model.ModeOff {
		if blowerActive && canToggleHandler {
			switchThings["deactivate_blower"] = true
		}
		if pumpActive && canToggleHandler {
			switchThings["deactivate_pump"] = true
		}
		if loopActive && canToggleLoop {
			switchThings["deactivate_loop"] = true
		}
		return switchThings, nil
	}

	// return an error and early out if the system and zone modes conflict
	if isOppositeMode(mode, sysMode) {
		return switchThings, fmt.Errorf("zone mode is opposite of system mode. zone: %s, zone mode: %s, system mode: %s", zoneID, mode, sysMode)
	}

	if handler == nil && loop == nil {
		return switchThings, fmt.Errorf("no distribution device associated with zone: %s", zoneID)
	}

	// Early out if we can't change the state
	skipHandler := true
	skipLoop := true
	if handler != nil && canToggleHandler {
		skipHandler = false
	}
	if loop != nil && canToggleLoop {
		skipLoop = false
	}
	if skipHandler && skipLoop {
		return switchThings, nil
	}

	// Make sure the blower is on if the zone is set to circulate
	if mode == model.ModeCirculate {

		// API should validate zone mode requests to block this
		if handler == nil {
			log.Error().
				Str("zone", zoneID).
				Msg("Zone set to circulate with no air handler")

			return switchThings, nil
		}

		if blowerActive {
			if pumpActive {
				// turn off pump if we just switched from heat/cool to circulate
				switchThings["deactivate_pump"] = true
			}
			return switchThings, nil // keep blower active, no switching
		}
		switchThings["activate_blower"] = true
		return switchThings, nil
	}

	shouldPrimary := shouldBeOn(temp, threshold, mode)
	shouldSecondary := shouldBeOn(temp, secondaryThreshold, mode)

	// Loop only
	if handler == nil && loop != nil {
		if mode == model.ModeHeating {
			if shouldPrimary && !loopActive {
				switchThings["activate_loop"] = true
			}
			if !shouldPrimary && loopActive {
				switchThings["deactivate_loop"] = true
			}
		}
		return switchThings, nil
	}

	// Handler only
	if loop == nil && handler != nil {
		if shouldPrimary && !pumpActive {
			switchThings["activate_blower"] = true
			switchThings["activate_pump"] = true
		}
		if !shouldPrimary && pumpActive {
			switchThings["deactivate_blower"] = true
			switchThings["deactivate_pump"] = true
		}
		return switchThings, nil
	}

	// Both handler and loop
	if handler != nil && loop != nil {
		if mode == model.ModeCooling {
			if shouldPrimary && !pumpActive {
				switchThings["activate_blower"] = true
				switchThings["activate_pump"] = true
			}
			if !shouldPrimary && pumpActive {
				switchThings["deactivate_blower"] = true
				switchThings["deactivate_pump"] = true
			}
			return switchThings, nil
		}

		// Heating mode logic
		if shouldPrimary && !loopActive {
			switchThings["activate_loop"] = true
		}
		if !shouldPrimary && loopActive {
			switchThings["deactivate_loop"] = true
		}

		if shouldSecondary && !pumpActive {
			switchThings["activate_blower"] = true
			switchThings["activate_pump"] = true
		}
		if !shouldSecondary && pumpActive {
			switchThings["deactivate_blower"] = true
			switchThings["deactivate_pump"] = true
		}
	}

	return switchThings, nil
}

func getThreshold(zone *model.Zone, active bool, secondary bool) float64 {
	log.Debug().
		Str("zone", zone.Label).
		Str("mode", string(zone.Mode)).
		Bool("active", active).
		Msg("Evaluating temperature threshold")

	var (
		base = zone.Setpoint

		heatOn  = base - ZoneSpread
		heatOff = base + ZoneSpread

		coolOn  = base + ZoneSpread
		coolOff = base - ZoneSpread

		backupHeatOn  = base - ZoneSpread - HeatingSecondaryThreshold
		backupHeatOff = base + ZoneSpread - HeatingSecondaryThreshold
	)

	if zone.Mode == model.ModeHeating {
		if active && secondary {
			return backupHeatOff
		}
		if active {
			return heatOff
		}
		if secondary {
			return backupHeatOn
		}
		return heatOn
	}
	if active {
		return coolOff
	}
	return coolOn
}

func isOppositeMode(a, b model.SystemMode) bool {
	return (a == model.ModeHeating && b == model.ModeCooling) ||
		(a == model.ModeCooling && b == model.ModeHeating)
}

func shouldBeOn(zt float64, threshold float64, mode model.SystemMode) bool {
	if mode == model.ModeHeating {
		return zt < threshold
	}
	return zt > threshold
}
