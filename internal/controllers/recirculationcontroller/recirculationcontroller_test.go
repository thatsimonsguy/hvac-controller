package recirculationcontroller

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var testHandler = &model.AirHandler{
	Device: model.Device{
		Name: "test-handler",
		Pin: model.GPIOPin{
			Number:     1,
			ActiveHigh: true,
		},
		MinOn:       3 * time.Minute,
		MinOff:      1 * time.Minute,
		Online:      true,
		LastChanged: time.Now().Add(-13 * time.Hour),
		ActiveModes: []string{"heating", "cooling"},
	},
	Zone: &model.Zone{
		ID:    "test-zone",
		Label: "Test Zone",
	},
	CircPumpPin: model.GPIOPin{
		Number:     2,
		ActiveHigh: false,
	},
}

func TestRecirculationConstants(t *testing.T) {
	assert.Equal(t, 12*time.Hour, RecirculationInterval)
	assert.Equal(t, 15*time.Minute, RecirculationDuration)
}

func TestEvaluateRecirculation_BlowerOffMoreThan12Hours(t *testing.T) {
	handler := &model.AirHandler{
		Device: model.Device{
			Name:        "test-handler",
			MinOn:       3 * time.Minute,
			MinOff:      1 * time.Minute,
			Online:      true,
			LastChanged: time.Now().Add(-13 * time.Hour),
		},
		Zone: &model.Zone{ID: "test-zone"},
	}

	activateCalled := false
	deactivateCalled := false

	origActivateBlower := activateBlower
	origDeactivateBlower := deactivateBlower
	origCanToggle := canToggle
	defer func() {
		activateBlower = origActivateBlower
		deactivateBlower = origDeactivateBlower
		canToggle = origCanToggle
	}()

	activateBlower = func(*model.AirHandler, *sql.DB) { activateCalled = true }
	deactivateBlower = func(*model.AirHandler, *sql.DB) { deactivateCalled = true }
	canToggle = func(*model.Device, time.Time) bool { return true }

	origCurrentlyActive := currentlyActive
	defer func() { currentlyActive = origCurrentlyActive }()
	currentlyActive = func(model.GPIOPin) bool { return false }

	evaluateRecirculation(handler, model.ModeOff, nil)

	assert.True(t, activateCalled)
	assert.False(t, deactivateCalled)
}

func TestEvaluateRecirculation_BlowerOnWithPumpActive(t *testing.T) {
	handler := &model.AirHandler{
		Device: model.Device{
			Name:        "test-handler",
			LastChanged: time.Now().Add(-1 * time.Hour),
		},
		Zone: &model.Zone{ID: "test-zone"},
	}

	activateCalled := false
	deactivateCalled := false

	origActivateBlower := activateBlower
	origDeactivateBlower := deactivateBlower
	origCanToggle := canToggle
	defer func() {
		activateBlower = origActivateBlower
		deactivateBlower = origDeactivateBlower
		canToggle = origCanToggle
	}()

	activateBlower = func(*model.AirHandler, *sql.DB) { activateCalled = true }
	deactivateBlower = func(*model.AirHandler, *sql.DB) { deactivateCalled = true }
	canToggle = func(*model.Device, time.Time) bool { return true }

	origCurrentlyActive := currentlyActive
	defer func() { currentlyActive = origCurrentlyActive }()
	
	callCount := 0
	currentlyActive = func(pin model.GPIOPin) bool {
		callCount++
		if callCount == 1 {
			return true
		}
		return true
	}

	evaluateRecirculation(handler, model.ModeHeating, nil)

	assert.False(t, activateCalled)
	assert.False(t, deactivateCalled)
}

func TestEvaluateRecirculation_BlowerOnSystemCirculate(t *testing.T) {
	handler := &model.AirHandler{
		Device: model.Device{
			Name:        "test-handler",
			LastChanged: time.Now().Add(-1 * time.Hour),
		},
		Zone: &model.Zone{ID: "test-zone"},
	}

	activateCalled := false
	deactivateCalled := false

	origActivateBlower := activateBlower
	origDeactivateBlower := deactivateBlower
	origCanToggle := canToggle
	defer func() {
		activateBlower = origActivateBlower
		deactivateBlower = origDeactivateBlower
		canToggle = origCanToggle
	}()

	activateBlower = func(*model.AirHandler, *sql.DB) { activateCalled = true }
	deactivateBlower = func(*model.AirHandler, *sql.DB) { deactivateCalled = true }
	canToggle = func(*model.Device, time.Time) bool { return true }

	origCurrentlyActive := currentlyActive
	defer func() { currentlyActive = origCurrentlyActive }()
	
	callCount := 0
	currentlyActive = func(pin model.GPIOPin) bool {
		callCount++
		if callCount == 1 {
			return true
		}
		return false
	}

	evaluateRecirculation(handler, model.ModeCirculate, nil)

	assert.False(t, activateCalled)
	assert.False(t, deactivateCalled)
}

func TestEvaluateRecirculation_BlowerOnMoreThan15Minutes(t *testing.T) {
	handler := &model.AirHandler{
		Device: model.Device{
			Name:        "test-handler",
			MinOn:       3 * time.Minute,
			MinOff:      1 * time.Minute,
			Online:      true,
			LastChanged: time.Now().Add(-20 * time.Minute),
		},
		Zone: &model.Zone{ID: "test-zone"},
	}

	activateCalled := false
	deactivateCalled := false

	origActivateBlower := activateBlower
	origDeactivateBlower := deactivateBlower
	origCanToggle := canToggle
	defer func() {
		activateBlower = origActivateBlower
		deactivateBlower = origDeactivateBlower
		canToggle = origCanToggle
	}()

	activateBlower = func(*model.AirHandler, *sql.DB) { activateCalled = true }
	deactivateBlower = func(*model.AirHandler, *sql.DB) { deactivateCalled = true }
	canToggle = func(*model.Device, time.Time) bool { return true }

	origCurrentlyActive := currentlyActive
	defer func() { currentlyActive = origCurrentlyActive }()
	
	callCount := 0
	currentlyActive = func(pin model.GPIOPin) bool {
		callCount++
		if callCount == 1 {
			return true
		}
		return false
	}

	evaluateRecirculation(handler, model.ModeHeating, nil)

	assert.False(t, activateCalled)
	assert.True(t, deactivateCalled)
}

