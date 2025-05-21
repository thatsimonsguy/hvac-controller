package device

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func ActivateAirHandler(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Activating air handler")
	gpio.Activate(ah.CircPumpPin)
	time.Sleep(5 * time.Second)
	gpio.Activate(ah.Pin)
	ah.LastChanged = time.Now()
}

func DeactivateAirHandler(ah *model.AirHandler) {
	log.Info().Str("device", ah.Name).Msg("Deactivating air handler")
	gpio.Deactivate(ah.Pin)
	time.Sleep(30 * time.Second)
	gpio.Deactivate(ah.CircPumpPin)
	ah.LastChanged = time.Now()
}

func ActivateRadiantLoop(rl *model.RadiantFloorLoop) {
	log.Info().Str("device", rl.Name).Msg("Activating radiant loop")
	gpio.Activate(rl.Pin)
	rl.LastChanged = time.Now()
}

func DeactivateRadiantLoop(rl *model.RadiantFloorLoop) {
	log.Info().Str("device", rl.Name).Msg("Deactivating radiant loop")
	gpio.Deactivate(rl.Pin)
	rl.LastChanged = time.Now()
}

func ActivateBoiler(b *model.Boiler) {
	log.Info().Str("device", b.Name).Msg("Activating boiler")
	gpio.Activate(b.Pin)
	b.LastChanged = time.Now()
}

func DeactivateBoiler(b *model.Boiler) {
	log.Info().Str("device", b.Name).Msg("Deactivating boiler")
	gpio.Deactivate(b.Pin)
	b.LastChanged = time.Now()
}

func ActivateHeatPump(hp *model.HeatPump) {
	log.Info().Str("device", hp.Name).Msg("Activating heat pump")
	gpio.Activate(hp.Pin)
	hp.LastChanged = time.Now()
}

func DeactivateHeatPump(hp *model.HeatPump) {
	log.Info().Str("device", hp.Name).Msg("Deactivating heat pump")
	gpio.Deactivate(hp.Pin)
	hp.LastChanged = time.Now()
}

// returns whether a device is eligible to be toggled based on its configured minimum on/off times
func CanToggle(d *model.Device, now time.Time) bool {
	active := gpio.CurrentlyActive(d.Pin)
	
	if active {
		return now.Sub(d.LastChanged) >= d.MinOn
	}
	return now.Sub(d.LastChanged) >= d.MinOff
}
