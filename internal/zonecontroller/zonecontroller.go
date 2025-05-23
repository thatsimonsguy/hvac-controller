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

func RunZoneController(zone model.Zone) {
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

			// Continue if we can't change the state
			skipHandler := true
			skipLoop := true

			if handler != nil && device.CanToggle(&handler.Device, time.Now()) {
				skipHandler = false
			}
			if loop != nil && device.CanToggle(&loop.Device, time.Now()) {
				skipLoop = false
			}

			if skipHandler && skipLoop {
				continue
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

			// Make sure the blower is on if the zone is set to circulate
			if zone.Mode == model.ModeCirculate {

				// API should validate zone mode requests to block this
				if handler == nil {
					log.Error().
						Str("zone", zone.ID).
						Msg("Zone set to circulate with no air handler")

					continue
				}

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

			// Control logic for zone with loop only
			if handler == nil && loop != nil {
				// Only operate radiant loops in heating mode
				if zone.Mode == model.ModeHeating {
					should := shouldBeOn(zoneTemp, threshold, zone.Mode)
					if should && !loopActive {
						device.ActivateRadiantLoop(loop)
					}
					if !should && loopActive {
						device.DeactivateRadiantLoop(loop)
					}
				}
				// Continue if in cooling or should == active
				continue
			}

			// Control logic for zone with handler only
			if loop == nil && handler != nil {
				should := shouldBeOn(zoneTemp, threshold, zone.Mode)
				if should && !pumpActive {
					device.ActivateAirHandler(handler)
				}
				if !should && pumpActive {
					device.DeactivateAirHandler(handler)
				}
				// Continue if should == active
				continue
			}

			// Control logic for zone with both handler and loop
			if handler != nil && loop != nil {

				// Ignore loop in cooling mode
				if zone.Mode == model.ModeCooling {
					should := shouldBeOn(zoneTemp, threshold, zone.Mode)
					if should && !pumpActive {
						device.ActivateAirHandler(handler)
					}
					if !should && pumpActive {
						device.DeactivateAirHandler(handler)
					}
					// Continue if should == active
					continue
				}

				// Heating mode
				// Loop is primary, preferred distributor
				if shouldBeOn(zoneTemp, threshold, zone.Mode) && !loopActive {
					device.ActivateRadiantLoop(loop)
				}
				if !shouldBeOn(zoneTemp, threshold, zone.Mode) && loopActive {
					device.DeactivateRadiantLoop(loop)
				}

				// Air Handler kicks in if loop is lagging
				if shouldBeOn(zoneTemp, secondaryThreshold, zone.Mode) && !pumpActive {
					device.ActivateAirHandler(handler)
				}
				if !shouldBeOn(zoneTemp, secondaryThreshold, zone.Mode) && pumpActive {
					device.DeactivateAirHandler(handler)
				}
			}
		}
	}()
}

func getThreshold(zone model.Zone, active bool, secondary bool) float64 {
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
