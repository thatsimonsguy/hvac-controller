package device

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func ActivateAirHandler(ah *model.AirHandler) error {
	log.Info().Str("device", ah.Name).Msg("Activating air handler")
	if err := gpio.Activate(ah.CircPumpPin); err != nil {
		return err
	}
	time.Sleep(5 * time.Second)
	if err := gpio.Activate(ah.Pin); err != nil {
		return err
	}
	ah.LastChanged = time.Now()
	return nil
}

func DeactivateAirHandler(ah *model.AirHandler) error {
	log.Info().Str("device", ah.Name).Msg("Deactivating air handler")
	if err := gpio.Deactivate(ah.Pin); err != nil {
		return err
	}
	time.Sleep(30 * time.Second)
	if err := gpio.Deactivate(ah.CircPumpPin); err != nil {
		return err
	}
	ah.LastChanged = time.Now()
	return nil
}

func ActivateRadiantLoop(rl *model.RadiantFloorLoop) error {
	log.Info().Str("device", rl.Name).Msg("Activating radiant loop")
	if err := gpio.Activate(rl.Pin); err != nil {
		return err
	}
	rl.LastChanged = time.Now()
	return nil
}

func DeactivateRadiantLoop(rl *model.RadiantFloorLoop) error {
	log.Info().Str("device", rl.Name).Msg("Deactivating radiant loop")
	if err := gpio.Deactivate(rl.Pin); err != nil {
		return err
	}
	rl.LastChanged = time.Now()
	return nil
}

func ActivateBoiler(b *model.Boiler) error {
	log.Info().Str("device", b.Name).Msg("Activating boiler")
	if err := gpio.Activate(b.Pin); err != nil {
		return err
	}
	b.LastChanged = time.Now()
	return nil
}

func DeactivateBoiler(b *model.Boiler) error {
	log.Info().Str("device", b.Name).Msg("Deactivating boiler")
	if err := gpio.Deactivate(b.Pin); err != nil {
		return err
	}
	b.LastChanged = time.Now()
	return nil
}

func ActivateHeatPump(hp *model.HeatPump) error {
	log.Info().Str("device", hp.Name).Msg("Activating heat pump")
	if err := gpio.Activate(hp.Pin); err != nil {
		return err
	}
	hp.LastChanged = time.Now()
	return nil
}

func DeactivateHeatPump(hp *model.HeatPump) error {
	log.Info().Str("device", hp.Name).Msg("Deactivating heat pump")
	if err := gpio.Deactivate(hp.Pin); err != nil {
		return err
	}
	hp.LastChanged = time.Now()
	return nil
}

// returns whether a device is eligible to be toggled based on its configured minimum on/off times
func CanToggle(d *model.Device, now time.Time) (bool, error) {
	active, err := gpio.CurrentlyActive(d.Pin)
	if err != nil {
		return false, err
	}
	if active {
		return now.Sub(d.LastChanged) >= d.MinOn, nil
	}
	return now.Sub(d.LastChanged) >= d.MinOff, nil
}
