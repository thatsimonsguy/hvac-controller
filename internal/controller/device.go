package controller

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
)

type Device struct {
	Name        string
	Pin         int
	LastChanged time.Time
	MinOn       time.Duration
	MinOff      time.Duration
}

func (d *Device) IsOn() bool {
	return gpio.Read(d.Pin)
}

func (d *Device) CanTurnOn(now time.Time) bool {
	return !d.IsOn() && now.Sub(d.LastChanged) >= d.MinOff
}

func (d *Device) CanTurnOff(now time.Time) bool {
	return d.IsOn() && now.Sub(d.LastChanged) >= d.MinOn
}

func (d *Device) TurnOn(now time.Time) {
	if d.CanTurnOn(now) {
		gpio.Set(d.Pin, true)
		d.LastChanged = now
		log.Info().Str("device", d.Name).Msg("Turned ON")
	}
}

func (d *Device) TurnOff(now time.Time) {
	if d.CanTurnOff(now) {
		gpio.Set(d.Pin, false)
		d.LastChanged = now
		log.Info().Str("device", d.Name).Msg("Turned OFF")
	}
}
