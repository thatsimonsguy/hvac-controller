package controller

import (
	"testing"
	"time"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func makeFakeController() *Controller {
	now := time.Now()

	hpA := &Device{
		Name:        "heat_pump_A",
		Pin:         1,
		LastChanged: now,
		MinOn:       0,
		MinOff:      0,
	}
	hpB := &Device{
		Name:        "heat_pump_B",
		Pin:         2,
		LastChanged: now,
		MinOn:       0,
		MinOff:      0,
	}
	boiler := &Device{
		Name:        "boiler",
		Pin:         3,
		LastChanged: now,
		MinOn:       0,
		MinOff:      0,
	}

	return &Controller{
		cfg: config.Config{
			RoleRotationMinutes: 1440,
		},
		state: &model.SystemState{
			SystemMode: model.ModeHeating,
		},
		boiler: boiler,
		heatPumps: [2]*HeatPump{
			{Name: "heat_pump_A", Relay: hpA, Role: "primary"},
			{Name: "heat_pump_B", Relay: hpB, Role: "secondary"},
		},
		lastRoleRotation:     now,
		roleRotationInterval: 24 * time.Hour,
	}
}

func TestRotateHeatPumpRoles(t *testing.T) {
	ctrl := makeFakeController()

	primaryBefore := ctrl.getPrimary().Name
	secondaryBefore := ctrl.getSecondary().Name

	ctrl.rotateHeatPumpRoles()

	primaryAfter := ctrl.getPrimary().Name
	secondaryAfter := ctrl.getSecondary().Name

	if primaryBefore == primaryAfter {
		t.Errorf("expected primary role to change, got same: %s", primaryAfter)
	}
	if secondaryBefore == secondaryAfter {
		t.Errorf("expected secondary role to change, got same: %s", secondaryAfter)
	}
}
