package failsafecontroller

import (
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var ignoredZones = []string{
	"garage",
}

func isZoneIgnored(zoneID string) bool {
	for _, ignored := range ignoredZones {
		if zoneID == ignored {
			return true
		}
	}
	return false
}

type ZoneState struct {
	Zone        model.Zone
	Temperature float64
	Handler     *model.AirHandler
	Loop        *model.RadiantFloorLoop
}

type FailsafeAction struct {
	SetOverride     bool
	ClearOverride   bool
	OverrideMode    model.SystemMode
	TriggerZone     string
	TriggerTemp     float64
	ActivateZones   map[string]model.SystemMode // zoneID -> mode
	DeactivateZones []string
}

type TemperatureService interface {
	GetTemperature(sensorID string) (float64, bool)
}

func RunFailsafeController(dbConn *sql.DB, tempService TemperatureService) {
	go func() {
		log.Info().Msg("Starting failsafe controller")

		time.Sleep(2 * time.Minute)

		for {
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)

			log.Info().Msg("Failsafe controller running evaluation cycle")

			// Gather all current state
			zones, err := db.GetAllZones(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not retrieve zones from db")
				continue
			}

			overrideActive, err := db.GetSystemOverride(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not check override status")
				continue
			}

			// Read all zone temperatures and device states
			zoneStates := gatherZoneStates(dbConn, zones, tempService)

			// Determine what actions need to be taken
			action := evaluateFailsafeActions(zoneStates, overrideActive, env.Cfg.SystemOverrideMinTemp, env.Cfg.SystemOverrideMaxTemp, env.Cfg.Spread)

			// Execute the determined actions
			executeFailsafeActions(dbConn, action)
		}
	}()
}

func gatherZoneStates(dbConn *sql.DB, zones []model.Zone, tempService TemperatureService) []ZoneState {
	var zoneStates []ZoneState

	for _, zone := range zones {
		sensor, err := db.GetSensorByID(dbConn, zone.Sensor.ID)
		if err != nil {
			log.Error().Err(err).Str("zone", zone.ID).Msg("Could not retrieve sensor for zone")
			continue
		}

		zoneTemp, valid := tempService.GetTemperature(sensor.ID)
		if !valid {
			log.Warn().Str("zone", zone.ID).Msg("No valid temperature reading available for zone")
			continue
		}

		handler, _ := db.GetAirHandlerByID(dbConn, zone.ID)
		loop, _ := db.GetRadiantLoopByID(dbConn, zone.ID)

		zoneStates = append(zoneStates, ZoneState{
			Zone:        zone,
			Temperature: zoneTemp,
			Handler:     handler,
			Loop:        loop,
		})
	}

	return zoneStates
}

func evaluateFailsafeActions(zoneStates []ZoneState, overrideActive bool, minTemp, maxTemp, spread float64) FailsafeAction {
	action := FailsafeAction{
		ActivateZones:   make(map[string]model.SystemMode),
		DeactivateZones: []string{},
	}


	// Check if any zone is outside safe range
	var needsOverride bool
	var requiredMode model.SystemMode
	var triggerZone string
	var triggerTemp float64

	for _, zoneState := range zoneStates {
		// Skip ignored zones
		if isZoneIgnored(zoneState.Zone.ID) {
			log.Debug().
				Str("zone", zoneState.Zone.ID).
				Msg("Skipping ignored zone for failsafe evaluation")
			continue
		}

		deltaFromMin := zoneState.Temperature - minTemp
		deltaFromMax := zoneState.Temperature - maxTemp

		log.Info().
			Str("zone", zoneState.Zone.ID).
			Float64("temp", zoneState.Temperature).
			Float64("min_threshold", minTemp).
			Float64("max_threshold", maxTemp).
			Float32("delta_from_min", float32(deltaFromMin)).
			Float32("delta_from_max", float32(deltaFromMax)).
			Bool("override_active", overrideActive).
			Msg("Evaluating zone for failsafe conditions")

		if zoneState.Temperature < minTemp {
			needsOverride = true
			requiredMode = model.ModeHeating
			triggerZone = zoneState.Zone.ID
			triggerTemp = zoneState.Temperature
			log.Warn().
				Str("zone", zoneState.Zone.ID).
				Float64("temp", zoneState.Temperature).
				Float64("min_threshold", minTemp).
				Msg("Zone temperature below safety minimum - failsafe heating required")
			break
		}

		if zoneState.Temperature > maxTemp {
			needsOverride = true
			requiredMode = model.ModeCooling
			triggerZone = zoneState.Zone.ID
			triggerTemp = zoneState.Temperature
			log.Warn().
				Str("zone", zoneState.Zone.ID).
				Float64("temp", zoneState.Temperature).
				Float64("max_threshold", maxTemp).
				Msg("Zone temperature above safety maximum - failsafe cooling required")
			break
		}
	}

	// Determine actions based on current state and needs
	if needsOverride && !overrideActive {
		// Need to activate override
		action.SetOverride = true
		action.OverrideMode = requiredMode
		action.TriggerZone = triggerZone
		action.TriggerTemp = triggerTemp
		action.ActivateZones[triggerZone] = requiredMode

	} else if overrideActive && !needsOverride {
		// Check if all zones are safely within bounds (with spread)
		allZonesSafe := true
		safeMin := minTemp + spread
		safeMax := maxTemp - spread

		for _, zoneState := range zoneStates {
			// Skip ignored zones for safety evaluation too
			if isZoneIgnored(zoneState.Zone.ID) {
				continue
			}
			
			if zoneState.Temperature < safeMin || zoneState.Temperature > safeMax {
				allZonesSafe = false
				break
			}
		}

		if allZonesSafe {
			// Clear override and deactivate all zones
			action.ClearOverride = true
			for _, zoneState := range zoneStates {
				action.DeactivateZones = append(action.DeactivateZones, zoneState.Zone.ID)
			}
		}
	}

	return action
}

func executeFailsafeActions(dbConn *sql.DB, action FailsafeAction) {
	if action.SetOverride {
		log.Warn().
			Str("trigger_zone", action.TriggerZone).
			Float64("trigger_temp", action.TriggerTemp).
			Str("required_mode", string(action.OverrideMode)).
			Msg("Activating failsafe override")

		if err := db.SetSystemOverride(dbConn, action.OverrideMode); err != nil {
			log.Error().Err(err).Msg("Failed to set system override")
			return
		}
	}

	if action.ClearOverride {
		log.Info().Msg("All zones within safe range - clearing failsafe override")

		if err := db.ClearSystemOverride(dbConn); err != nil {
			log.Error().Err(err).Msg("Failed to clear system override")
			return
		}
	}

	// Activate zones as needed
	for zoneID, mode := range action.ActivateZones {
		activateZoneDistribution(dbConn, zoneID, mode)
	}

	// Deactivate zones as needed
	for _, zoneID := range action.DeactivateZones {
		deactivateZoneDistribution(dbConn, zoneID)
	}
}

func activateZoneDistribution(dbConn *sql.DB, zoneID string, mode model.SystemMode) {
	handler, err := db.GetAirHandlerByID(dbConn, zoneID)
	if err == nil && handler != nil {
		if device.CanToggle(&handler.Device, time.Now()) {
			if mode == model.ModeCooling {
				log.Info().Str("zone", zoneID).Msg("Activating air handler for failsafe cooling")
				device.ActivateBlower(handler, dbConn)
				device.ActivateAirHandler(handler, dbConn)
			} else if mode == model.ModeHeating {
				log.Info().Str("zone", zoneID).Msg("Activating air handler for failsafe heating")
				device.ActivateBlower(handler, dbConn)
				device.ActivateAirHandler(handler, dbConn)
			}
		}
	}

	loop, err := db.GetRadiantLoopByID(dbConn, zoneID)
	if err == nil && loop != nil && mode == model.ModeHeating {
		if device.CanToggle(&loop.Device, time.Now()) {
			log.Info().Str("zone", zoneID).Msg("Activating radiant loop for failsafe heating")
			device.ActivateRadiantLoop(loop, dbConn)
		}
	}
}

func deactivateZoneDistribution(dbConn *sql.DB, zoneID string) {
	handler, err := db.GetAirHandlerByID(dbConn, zoneID)
	if err == nil && handler != nil {
		if device.CanToggle(&handler.Device, time.Now()) {
			log.Info().Str("zone", zoneID).Msg("Deactivating air handler after failsafe")
			device.DeactivateAirHandler(handler, dbConn)
			device.DeactivateBlower(handler, dbConn)
		}
	}

	loop, err := db.GetRadiantLoopByID(dbConn, zoneID)
	if err == nil && loop != nil {
		if device.CanToggle(&loop.Device, time.Now()) {
			log.Info().Str("zone", zoneID).Msg("Deactivating radiant loop after failsafe")
			device.DeactivateRadiantLoop(loop, dbConn)
		}
	}
}