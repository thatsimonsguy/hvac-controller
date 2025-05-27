package zonecontroller

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/datadog"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

const ZoneSpread float64 = 0.5
const HeatingSecondaryThreshold float64 = 3

func RunZoneController(zone *model.Zone) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")

		// Sleep for handler.minOff at first run
		if handler := getAirHandler(zone.ID); handler != nil {
			time.Sleep(handler.MinOff + time.Duration(rand.Intn(1500))*time.Millisecond)
		}

		for {
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)

			// Early out if zone if off
			if zone.Mode == model.ModeOff {
				continue
			}

			// Log an error and early out if the system and zone modes conflict
			if isOppositeMode(zone.Mode, env.SystemState.SystemMode) {
				log.Error().
					Str("zone", zone.ID).
					Str("zone_mode", string(zone.Mode)).
					Str("system_mode", string(env.SystemState.SystemMode)).
					Msg("Zone mode is opposite of system mode — skipping control cycle")
				continue
			}

			// Get distribution devices
			handler := getAirHandler(zone.ID)
			loop := getRadiantLoop(zone.ID)
			if handler == nil && loop == nil {
				log.Error().Str("zone_id", zone.ID).Msg("No distribution device associated with zone")
				continue
			}

			handlerNil := handler == nil
			loopNil := loop == nil

			// Get toggleable statuses
			canToggleHandler := false
			canToggleLoop := false

			if !handlerNil {
				canToggleHandler = device.CanToggle(&handler.Device, time.Now())
			}
			if !loopNil {
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

			// Get temps
			sensorPath := filepath.Join("/sys/bus/w1/devices", zone.Sensor.Bus)
			zoneTemp := gpio.ReadSensorTempWithRetries(sensorPath, 5)
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
			switchMap := evaluateZoneActions(
				zone.ID,
				handler == nil,
				blowerActive,
				pumpActive,
				loop == nil,
				loopActive,
				canToggleHandler,
				canToggleLoop,
				zoneTemp,
				zone.Mode,
				threshold,
				secondaryThreshold,
			)

			if handler != nil {
				if switchMap["activate_blower"] {
					device.ActivateBlower(handler)
				}
				if switchMap["deactivate_blower"] {
					device.DeactivateBlower(handler)
				}
				if switchMap["activate_pump"] {
					device.ActivateAirHandler(handler)
				}
				if switchMap["deactivate_pump"] {
					device.DeactivateAirHandler(handler)
				}
			}

			if loop != nil {
				if switchMap["activate_loop"] {
					device.ActivateRadiantLoop(loop)
				}
				if switchMap["deactivate_loop"] {
					device.DeactivateRadiantLoop(loop)
				}
			}

		}
	}()
}

func evaluateZoneActions(
	zoneID string,
	handlerNil bool,
	blowerActive bool,
	pumpActive bool,
	loopNil bool,
	loopActive bool,
	canToggleHandler bool,
	canToggleLoop bool,
	temp float64,
	mode model.SystemMode,
	threshold float64,
	secondaryThreshold float64,
) map[string]bool {
	switchThings := map[string]bool{
		"activate_blower":   false,
		"activate_pump":     false,
		"activate_loop":     false,
		"deactivate_blower": false,
		"deactivate_pump":   false,
		"deactivate_loop":   false,
	}

	shouldPrimary := shouldBeOn(temp, threshold, mode)
	shouldSecondary := shouldBeOn(temp, secondaryThreshold, mode)

	// Early out if we can't change the state
	skipHandler := true
	skipLoop := true

	if !handlerNil && canToggleHandler {
		skipHandler = false
	}
	if !loopNil && canToggleLoop {
		skipLoop = false
	}

	if skipHandler && skipLoop {
		return switchThings
	}

	// Make sure the blower is on if the zone is set to circulate
	if mode == model.ModeCirculate {

		// API should validate zone mode requests to block this
		if handlerNil {
			log.Error().
				Str("zone", zoneID).
				Msg("Zone set to circulate with no air handler")

			return switchThings
		}

		if blowerActive {
			if pumpActive {
				// turn off pump if we just switched from heat/cool to circulate
				switchThings["deactivate_pump"] = true
			}
		}
		switchThings["activate_blower"] = true
		return switchThings
	}

	// Loop only
	if handlerNil && !loopNil {
		if mode == model.ModeHeating {
			if shouldPrimary && !loopActive {
				switchThings["activate_loop"] = true
			}
			if !shouldPrimary && loopActive {
				switchThings["deactivate_loop"] = true
			}
		}
		return switchThings
	}

	// Handler only
	if loopNil && !handlerNil {
		if shouldPrimary && !pumpActive {
			switchThings["activate_blower"] = true
			switchThings["activate_pump"] = true
		}
		if !shouldPrimary && pumpActive {
			switchThings["deactivate_blower"] = true
			switchThings["deactivate_pump"] = true
		}
		return switchThings
	}

	// Both handler and loop
	if !handlerNil && !loopNil {
		if mode == model.ModeCooling {
			if shouldPrimary && !pumpActive {
				switchThings["activate_blower"] = true
				switchThings["activate_pump"] = true
			}
			if !shouldPrimary && pumpActive {
				switchThings["deactivate_blower"] = true
				switchThings["deactivate_pump"] = true
			}
			return switchThings
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

	return switchThings
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

func getAirHandler(zoneID string) *model.AirHandler {
	for i, ah := range env.SystemState.AirHandlers {
		if ah.Zone != nil && ah.Zone.ID == zoneID {
			return &env.SystemState.AirHandlers[i]
		}
	}
	return nil
}

func getRadiantLoop(zoneID string) *model.RadiantFloorLoop {
	for i, rl := range env.SystemState.RadiantLoops {
		if rl.Zone != nil && rl.Zone.ID == zoneID {
			return &env.SystemState.RadiantLoops[i]
		}
	}
	return nil
}

func shouldBeOn(zt float64, threshold float64, mode model.SystemMode) bool {
	if mode == model.ModeHeating {
		return zt < threshold
	}
	return zt > threshold
}
