package controller

import (
	"time"

	"github.com/rs/zerolog/log"
)

type Device struct {
	Name        string
	IsOn        bool
	LastChanged time.Time
	MinOn       time.Duration
	MinOff      time.Duration
	OnFunc      func()
	OffFunc     func()
}

func (d *Device) CanTurnOn(now time.Time) bool {
	return !d.IsOn && now.Sub(d.LastChanged) >= d.MinOff
}

func (d *Device) CanTurnOff(now time.Time) bool {
	return d.IsOn && now.Sub(d.LastChanged) >= d.MinOn
}

func (d *Device) TurnOn(now time.Time) {
	if d.CanTurnOn(now) {
		d.OnFunc()
		d.IsOn = true
		d.LastChanged = now
		log.Info().Str("device", d.Name).Msg("Turned ON")
	}
}

func (d *Device) TurnOff(now time.Time) {
	if d.CanTurnOff(now) {
		d.OffFunc()
		d.IsOn = false
		d.LastChanged = now
		log.Info().Str("device", d.Name).Msg("Turned OFF")
	}
}
