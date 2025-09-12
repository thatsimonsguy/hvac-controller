package device

import (
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var ActivateAirHandler = func(ah *model.AirHandler, dbConn *sql.DB) {
	log.Info().Str("device", ah.Name).Msg("Activating air handler")
	gpio.Activate(ah.CircPumpPin)
	time.Sleep(5 * time.Second)
	gpio.Activate(ah.Pin)
	now := time.Now()
	ah.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, ah.Name, now); err != nil {
		log.Error().Err(err).Str("device", ah.Name).Msg("Failed to update device last_changed in database")
	}
}

var ActivateBlower = func(ah *model.AirHandler, dbConn *sql.DB) {
	log.Info().Str("device", ah.Name).Msg("Activating blower")
	gpio.Activate(ah.Pin)
	now := time.Now()
	ah.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, ah.Name, now); err != nil {
		log.Error().Err(err).Str("device", ah.Name).Msg("Failed to update device last_changed in database")
	}
}

var DeactivateBlower = func(ah *model.AirHandler, dbConn *sql.DB) {
	log.Info().Str("device", ah.Name).Msg("Deactivating blower")
	gpio.Deactivate(ah.Pin)
	now := time.Now()
	ah.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, ah.Name, now); err != nil {
		log.Error().Err(err).Str("device", ah.Name).Msg("Failed to update device last_changed in database")
	}
}

var DeactivateAirHandler = func(ah *model.AirHandler, dbConn *sql.DB) {
	log.Info().Str("device", ah.Name).Msg("Deactivating air handler")
	gpio.Deactivate(ah.Pin)
	time.Sleep(30 * time.Second)
	gpio.Deactivate(ah.CircPumpPin)
	now := time.Now()
	ah.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, ah.Name, now); err != nil {
		log.Error().Err(err).Str("device", ah.Name).Msg("Failed to update device last_changed in database")
	}
}

var ActivateRadiantLoop = func(rl *model.RadiantFloorLoop, dbConn *sql.DB) {
	log.Info().Str("device", rl.Name).Msg("Activating radiant loop")
	gpio.Activate(rl.Pin)
	now := time.Now()
	rl.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, rl.Name, now); err != nil {
		log.Error().Err(err).Str("device", rl.Name).Msg("Failed to update device last_changed in database")
	}
}

var DeactivateRadiantLoop = func(rl *model.RadiantFloorLoop, dbConn *sql.DB) {
	log.Info().Str("device", rl.Name).Msg("Deactivating radiant loop")
	gpio.Deactivate(rl.Pin)
	now := time.Now()
	rl.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, rl.Name, now); err != nil {
		log.Error().Err(err).Str("device", rl.Name).Msg("Failed to update device last_changed in database")
	}
}

var ActivateBoiler = func(b *model.Boiler, dbConn *sql.DB) {
	log.Info().Str("device", b.Name).Msg("Activating boiler")
	gpio.Activate(b.Pin)
	now := time.Now()
	b.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, b.Name, now); err != nil {
		log.Error().Err(err).Str("device", b.Name).Msg("Failed to update device last_changed in database")
	}
}

var DeactivateBoiler = func(b *model.Boiler, dbConn *sql.DB) {
	log.Info().Str("device", b.Name).Msg("Deactivating boiler")
	gpio.Deactivate(b.Pin)
	now := time.Now()
	b.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, b.Name, now); err != nil {
		log.Error().Err(err).Str("device", b.Name).Msg("Failed to update device last_changed in database")
	}
}

var ActivateHeatPump = func(hp *model.HeatPump, dbConn *sql.DB) {
	log.Info().Str("device", hp.Name).Msg("Activating heat pump")
	gpio.Activate(hp.Pin)
	now := time.Now()
	hp.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, hp.Name, now); err != nil {
		log.Error().Err(err).Str("device", hp.Name).Msg("Failed to update device last_changed in database")
	}
}

var DeactivateHeatPump = func(hp *model.HeatPump, dbConn *sql.DB) {
	log.Info().Str("device", hp.Name).Msg("Deactivating heat pump")
	gpio.Deactivate(hp.Pin)
	now := time.Now()
	hp.LastChanged = now
	if err := db.UpdateDeviceLastChanged(dbConn, hp.Name, now); err != nil {
		log.Error().Err(err).Str("device", hp.Name).Msg("Failed to update device last_changed in database")
	}
}

// returns whether a device is eligible to be toggled based on its configured minimum on/off times
var CanToggle = func(d *model.Device, now time.Time) bool {
	active := gpio.CurrentlyActive(d.Pin)

	if active {
		return now.Sub(d.LastChanged) >= d.MinOn
	}
	return now.Sub(d.LastChanged) >= d.MinOff
}
