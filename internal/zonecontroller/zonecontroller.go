package zonecontroller

import (
	"fmt"
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

func RunZoneController(zone model.Zone) {
	go func() {
		log.Info().Str("zone", zone.ID).Msg("Starting zone controller")

		// Sleep for handler.minOff at first run
		if handler := getAirHandler(zone.ID); handler != nil {
			time.Sleep(handler.MinOff)
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

			// Get the handler
			handler := getAirHandler(zone.ID)
			if handler == nil {
				log.Error().Str("zone_id", zone.ID).Msg("No air handler associated with zone")
				continue
			}

			// Continue if we can't change the state
			if !device.CanToggle(&handler.Device, time.Now()) {
				continue
			}

			// Get active states for blower and pump
			blowerActive := gpio.CurrentlyActive(handler.Pin)
			pumpActive := gpio.CurrentlyActive(handler.CircPumpPin)

			// Make sure the blower is on if the zone is set to circulate
			if zone.Mode == model.ModeCirculate {
				if blowerActive {
					if pumpActive {
						// turn off pump if we just switched from heat/cool to circulate
						gpio.Deactivate(handler.CircPumpPin)
						continue
					}
					continue
				}
				gpio.Activate(handler.Pin)
				continue
			}

			// Evaluate heating and cooling modes
			sensorPath := filepath.Join("/sys/bus/w1/devices", zone.Sensor.Bus)
			zoneTemp := gpio.ReadSensorTemp(sensorPath)
			threshold := getThreshold(zone, pumpActive)

			datadog.Gauge("zone.temperature", zoneTemp, "component:sensor", fmt.Sprintf("zone:%s", zone.ID))

			// Log out temp
			log.Debug().
				Str("zone", zone.Label).
				Float64("temp", zoneTemp).
				Float64("setpoint", zone.Setpoint).
				Float64("threshold", threshold).
				Str("mode", string(zone.Mode)).
				Msg("Zone temperature check")

			// Activate/deactivate air handler or leave on/off
			if shouldBeOn(zoneTemp, threshold, zone.Mode) {
				if pumpActive {
					continue
				}
				device.ActivateAirHandler(handler)
				continue
			}
			if pumpActive {
				device.DeactivateAirHandler(handler)
			}
		}
	}()
}

func getThreshold(zone model.Zone, active bool) float64 {
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
	)

	if zone.Mode == model.ModeHeating {
		if active {
			return heatOff
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

func shouldBeOn(zt float64, threshold float64, mode model.SystemMode) bool {
	if mode == model.ModeHeating {
		return zt < threshold
	}
	return zt > threshold
}
