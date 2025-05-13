package controller

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

type HeatPump struct {
	Name  string
	Relay *Device
	Role  string // "primary" or "secondary"
}

type Controller struct {
	cfg   config.Config
	state *model.SystemState

	heatPumps            [2]*HeatPump
	boiler               *Device
	lastRoleRotation     time.Time
	roleRotationInterval time.Duration
}

func New(cfg config.Config, state *model.SystemState) *Controller {
	now := time.Now()

	primaryDevice := &Device{
		Name:        "heat_pump_A",
		Pin:         *cfg.GPIO.HeatPumpARelayPin,
		LastChanged: now,
		MinOn:       5 * time.Minute,
		MinOff:      5 * time.Minute,
	}

	secondaryDevice := &Device{
		Name:        "heat_pump_B",
		Pin:         *cfg.GPIO.HeatPumpBRelayPin,
		LastChanged: now,
		MinOn:       5 * time.Minute,
		MinOff:      5 * time.Minute,
	}

	boiler := &Device{
		Name:        "boiler",
		Pin:         *cfg.GPIO.BoilerRelayPin,
		LastChanged: now,
		MinOn:       5 * time.Minute,
		MinOff:      5 * time.Minute,
	}

	return &Controller{
		cfg:   cfg,
		state: state,
		heatPumps: [2]*HeatPump{
			{Name: "heat_pump_A", Relay: primaryDevice, Role: "primary"},
			{Name: "heat_pump_B", Relay: secondaryDevice, Role: "secondary"},
		},
		boiler:               boiler,
		lastRoleRotation:     now,
		roleRotationInterval: time.Duration(cfg.RoleRotationMinutes) * time.Minute,
	}
}

func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(c.cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Shutting down controller loop")
			return
		case <-ticker.C:
			c.evaluate()
		}
	}
}

func (c *Controller) evaluate() {
	mode := c.state.SystemMode
	temp := c.readBufferTemp()
	now := time.Now()

	if now.Sub(c.lastRoleRotation) >= c.roleRotationInterval {
		c.rotateHeatPumpRoles()
		c.lastRoleRotation = now
	}

	log.Debug().
		Str("mode", string(mode)).
		Float64("temp", temp).
		Msg("Evaluating buffer tank status")

	switch mode {
	case model.ModeHeating:
		c.handleHeating(temp, now)
	case model.ModeCooling:
		c.handleCooling(temp, now)
	case model.ModeCirculate, model.ModeOff:
		c.turnEverythingOff(now)
	}
}

func (c *Controller) handleHeating(temp float64, now time.Time) {
	t1 := c.cfg.HeatingThreshold
	t2 := t1 - c.cfg.SecondaryMargin
	t3 := t1 - c.cfg.TertiaryMargin
	offLimit := t1 + c.cfg.Spread

	primary := c.getPrimary().Relay
	secondary := c.getSecondary().Relay

	switch {
	case temp <= t3:
		c.boiler.TurnOn(now)
		secondary.TurnOn(now)
		primary.TurnOn(now)
	case temp <= t2:
		c.boiler.TurnOff(now)
		secondary.TurnOn(now)
		primary.TurnOn(now)
	case temp <= t1:
		c.boiler.TurnOff(now)
		secondary.TurnOff(now)
		primary.TurnOn(now)
	case temp >= offLimit:
		c.turnEverythingOff(now)
	}
}

func (c *Controller) handleCooling(temp float64, now time.Time) {
	t1 := c.cfg.CoolingThreshold
	t2 := t1 + c.cfg.SecondaryMargin
	offLimit := t1 - c.cfg.Spread

	primary := c.getPrimary().Relay
	secondary := c.getSecondary().Relay

	switch {
	case temp >= t2:
		secondary.TurnOn(now)
		primary.TurnOn(now)
	case temp >= t1:
		secondary.TurnOff(now)
		primary.TurnOn(now)
	case temp <= offLimit:
		c.turnEverythingOff(now)
	}
}

func (c *Controller) turnEverythingOff(now time.Time) {
	c.getPrimary().Relay.TurnOff(now)
	c.getSecondary().Relay.TurnOff(now)
	c.boiler.TurnOff(now)
}

func (c *Controller) getPrimary() *HeatPump {
	for _, hp := range c.heatPumps {
		if hp.Role == "primary" {
			return hp
		}
	}
	return nil
}

func (c *Controller) getSecondary() *HeatPump {
	for _, hp := range c.heatPumps {
		if hp.Role == "secondary" {
			return hp
		}
	}
	return nil
}

func (c *Controller) rotateHeatPumpRoles() {
	c.heatPumps[0].Role, c.heatPumps[1].Role = c.heatPumps[1].Role, c.heatPumps[0].Role
	log.Info().
		Str("now_primary", c.getPrimary().Name).
		Str("now_secondary", c.getSecondary().Name).
		Msg("Rotated heat pump roles")
}

func (c *Controller) readBufferTemp() float64 {
	// TODO: replace with GPIO sensor read
	now := time.Now().Unix()
	return 110.0 + float64(now%30) // cycles between 110–139
}
