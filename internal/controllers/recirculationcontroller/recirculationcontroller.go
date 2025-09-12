package recirculationcontroller

import (
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

const RecirculationInterval = 12 * time.Hour
const RecirculationDuration = 15 * time.Minute

var activateBlower = device.ActivateBlower
var deactivateBlower = device.DeactivateBlower
var currentlyActive = gpio.CurrentlyActive
var canToggle = device.CanToggle

func RunRecirculationController(dbConn *sql.DB) {
	go func() {
		log.Info().Msg("Starting recirculation controller")

		time.Sleep(5 * time.Minute)

		for {
			time.Sleep(time.Duration(env.Cfg.PollIntervalSeconds) * time.Second)

			log.Info().Msg("Recirculation controller running evaluation cycle")

			// Safety check: clear recirculation override if it's been active too long
			recircActive, startedAt, err := db.GetRecirculationStatus(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Failed to check recirculation status")
			} else if recircActive && time.Since(startedAt) > RecirculationDuration*2 {
				log.Warn().
					Dur("duration", time.Since(startedAt)).
					Msg("Recirculation has been active too long - clearing override")
				if err := db.SetRecirculationActive(dbConn, false, time.Time{}); err != nil {
					log.Error().Err(err).Msg("Failed to clear stuck recirculation flag")
				}
			}

			zones, err := db.GetAllZones(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not retrieve zones from db")
				continue
			}

			sysMode, err := db.GetSystemMode(dbConn)
			if err != nil {
				log.Error().Err(err).Msg("Could not retrieve system mode from db")
				continue
			}

			for _, zone := range zones {
				handler, err := db.GetAirHandlerByID(dbConn, zone.ID)
				if err != nil {
					log.Debug().Err(err).Str("zone", zone.ID).Msg("Could not retrieve air handler for zone")
					continue
				}

				if handler == nil {
					continue
				}

				evaluateRecirculation(handler, sysMode, dbConn)
			}
		}
	}()
}

func evaluateRecirculation(handler *model.AirHandler, sysMode model.SystemMode, dbConn *sql.DB) {
	now := time.Now()
	blowerActive := currentlyActive(handler.Pin)
	pumpActive := currentlyActive(handler.CircPumpPin)
	timeSinceLastToggle := now.Sub(handler.LastChanged)

	timeUntilRecirc := RecirculationInterval - timeSinceLastToggle
	needsRecirculation := timeSinceLastToggle > RecirculationInterval

	log.Info().
		Str("zone", handler.Zone.ID).
		Bool("blower_active", blowerActive).
		Bool("pump_active", pumpActive).
		Dur("time_since_last_toggle", timeSinceLastToggle).
		Dur("time_until_recirc", timeUntilRecirc).
		Bool("needs_recirculation", needsRecirculation).
		Str("system_mode", string(sysMode)).
		Msg("Evaluating recirculation for air handler")

	if !blowerActive && timeSinceLastToggle > RecirculationInterval {
		if canToggle(&handler.Device, now) {
			log.Info().
				Str("zone", handler.Zone.ID).
				Msg("Activating blower for recirculation - 12+ hours since last activity")
			activateBlower(handler, dbConn)
			
			// Set recirculation override to prevent zone controller from immediately turning it off
			if dbConn != nil {
				if err := db.SetRecirculationActive(dbConn, true, now); err != nil {
					log.Error().Err(err).Msg("Failed to set recirculation active flag")
				}
			}
		}
		return
	}

	if blowerActive {
		if pumpActive {
			log.Debug().
				Str("zone", handler.Zone.ID).
				Msg("Circulation pump running - zone has heating/cooling demand")
			return
		}

		if sysMode == model.ModeCirculate {
			log.Debug().
				Str("zone", handler.Zone.ID).
				Msg("System in circulate mode - no action needed")
			return
		}

		if !pumpActive && sysMode != model.ModeCirculate && timeSinceLastToggle > RecirculationDuration {
			if canToggle(&handler.Device, now) {
				log.Info().
					Str("zone", handler.Zone.ID).
					Msg("Deactivating blower after 15min recirculation")
				deactivateBlower(handler, dbConn)
				
				// Clear recirculation override
				if dbConn != nil {
					if err := db.SetRecirculationActive(dbConn, false, time.Time{}); err != nil {
						log.Error().Err(err).Msg("Failed to clear recirculation active flag")
					}
				}
			}
		}
	}
}