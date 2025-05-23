package device

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var ActivateAirHandler = func(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Activating air handler")
	gpio.Activate(ah.CircPumpPin)
	time.Sleep(5 * time.Second)
	gpio.Activate(ah.Pin)
	ah.LastChanged = time.Now()
}

var ActivateBlower = func(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Activating blower")
	gpio.Activate(ah.Pin)
	ah.LastChanged = time.Now()
}

var DeactivateBlower = func(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Activating blower")
	gpio.Deactivate(ah.Pin)
	ah.LastChanged = time.Now()
}

var DeactivateAirHandler = func(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Deactivating air handler")
	gpio.Deactivate(ah.Pin)
	time.Sleep(30 * time.Second)
	gpio.Deactivate(ah.CircPumpPin)
	ah.LastChanged = time.Now()
}

var ActivateRadiantLoop = func(rl *model.RadiantFloorLoop) {
	log.Info().Str("device", rl.Name).Msg("Activating radiant loop")
	gpio.Activate(rl.Pin)
	rl.LastChanged = time.Now()
}

var DeactivateRadiantLoop = func(rl *model.RadiantFloorLoop) {
	log.Info().Str("device", rl.Name).Msg("Deactivating radiant loop")
	gpio.Deactivate(rl.Pin)
	rl.LastChanged = time.Now()
}

var ActivateBoiler = func(b *model.Boiler) {
	log.Info().Str("device", b.Name).Msg("Activating boiler")
	gpio.Activate(b.Pin)
	b.LastChanged = time.Now()
}

var DeactivateBoiler = func(b *model.Boiler) {
	log.Info().Str("device", b.Name).Msg("Deactivating boiler")
	gpio.Deactivate(b.Pin)
	b.LastChanged = time.Now()
}

var ActivateHeatPump = func(hp *model.HeatPump) {
	log.Info().Str("device", hp.Name).Msg("Activating heat pump")
	gpio.Activate(hp.Pin)
	hp.LastChanged = time.Now()
}

var DeactivateHeatPump = func(hp *model.HeatPump) {
	log.Info().Str("device", hp.Name).Msg("Deactivating heat pump")
	gpio.Deactivate(hp.Pin)
	hp.LastChanged = time.Now()
}

// returns whether a device is eligible to be toggled based on its configured minimum on/off times
var CanToggle = func(d *model.Device, now time.Time) bool {
	active := gpio.CurrentlyActive(d.Pin)

	if active {
		return now.Sub(d.LastChanged) >= d.MinOn
	}
	return now.Sub(d.LastChanged) >= d.MinOff
}
