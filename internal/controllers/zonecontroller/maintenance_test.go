package zonecontroller

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func newHandler(online bool) *model.AirHandler {
	return &model.AirHandler{
		Device: model.Device{
			Name:   "test-handler",
			Online: online,
			Pin:    model.GPIOPin{Number: 1, ActiveHigh: true},
		},
		CircPumpPin: model.GPIOPin{Number: 2, ActiveHigh: false},
		Zone:        &model.Zone{ID: "test-zone"},
	}
}

func newLoop(online bool) *model.RadiantFloorLoop {
	return &model.RadiantFloorLoop{
		Device: model.Device{
			Name:   "test-loop",
			Online: online,
			Pin:    model.GPIOPin{Number: 3, ActiveHigh: true},
		},
		Zone: &model.Zone{ID: "test-zone"},
	}
}

func TestExcludeOfflineDevices(t *testing.T) {
	origCurrentlyActive := gpio.CurrentlyActive
	origDeactivateAH := device.DeactivateAirHandler
	origDeactivateLoop := device.DeactivateRadiantLoop
	defer func() {
		gpio.CurrentlyActive = origCurrentlyActive
		device.DeactivateAirHandler = origDeactivateAH
		device.DeactivateRadiantLoop = origDeactivateLoop
	}()

	t.Run("online devices are returned unchanged and never forced off", func(t *testing.T) {
		gpio.CurrentlyActive = func(model.GPIOPin) bool { return true }
		ahOff, loopOff := false, false
		device.DeactivateAirHandler = func(*model.AirHandler, *sql.DB) { ahOff = true }
		device.DeactivateRadiantLoop = func(*model.RadiantFloorLoop, *sql.DB) { loopOff = true }

		h := newHandler(true)
		l := newLoop(true)
		gotH, gotL := excludeOfflineDevices(h, l, true, true, nil)

		assert.Same(t, h, gotH)
		assert.Same(t, l, gotL)
		assert.False(t, ahOff)
		assert.False(t, loopOff)
	})

	t.Run("offline active handler is forced off and excluded", func(t *testing.T) {
		gpio.CurrentlyActive = func(model.GPIOPin) bool { return true }
		ahOff := false
		device.DeactivateAirHandler = func(*model.AirHandler, *sql.DB) { ahOff = true }
		device.DeactivateRadiantLoop = func(*model.RadiantFloorLoop, *sql.DB) {}

		gotH, gotL := excludeOfflineDevices(newHandler(false), nil, true, false, nil)

		assert.Nil(t, gotH)
		assert.Nil(t, gotL)
		assert.True(t, ahOff, "an active offline handler must be forced off")
	})

	t.Run("offline inactive handler is excluded without a redundant deactivate", func(t *testing.T) {
		gpio.CurrentlyActive = func(model.GPIOPin) bool { return false }
		ahOff := false
		device.DeactivateAirHandler = func(*model.AirHandler, *sql.DB) { ahOff = true }

		gotH, _ := excludeOfflineDevices(newHandler(false), nil, true, false, nil)

		assert.Nil(t, gotH)
		assert.False(t, ahOff, "an already-off handler should not be re-deactivated")
	})

	t.Run("offline handler that cannot toggle is excluded but not forced off", func(t *testing.T) {
		gpio.CurrentlyActive = func(model.GPIOPin) bool { return true }
		ahOff := false
		device.DeactivateAirHandler = func(*model.AirHandler, *sql.DB) { ahOff = true }

		gotH, _ := excludeOfflineDevices(newHandler(false), nil, false, false, nil)

		assert.Nil(t, gotH, "an offline handler is always excluded from control")
		assert.False(t, ahOff, "min on/off time is respected before forcing off")
	})

	t.Run("offline active loop is forced off and excluded", func(t *testing.T) {
		gpio.CurrentlyActive = func(model.GPIOPin) bool { return true }
		loopOff := false
		device.DeactivateAirHandler = func(*model.AirHandler, *sql.DB) {}
		device.DeactivateRadiantLoop = func(*model.RadiantFloorLoop, *sql.DB) { loopOff = true }

		onlineH := newHandler(true)
		gotH, gotL := excludeOfflineDevices(onlineH, newLoop(false), true, true, nil)

		assert.Same(t, onlineH, gotH) // online handler retained
		assert.Nil(t, gotL)
		assert.True(t, loopOff)
	})
}
