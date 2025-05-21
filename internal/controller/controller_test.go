// controller_unit_test.go
package controller

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

func TestShouldBeOn(t *testing.T) {
	tests := []struct {
		name      string
		bt        float64
		threshold float64
		mode      model.SystemMode
		expected  bool
	}{
		{"Heating: below threshold", 45.0, 50.0, model.ModeHeating, true},
		{"Heating: at threshold", 50.0, 50.0, model.ModeHeating, false},
		{"Heating: above threshold", 55.0, 50.0, model.ModeHeating, false},
		{"Cooling: below threshold", 65.0, 70.0, model.ModeCooling, false},
		{"Cooling: at threshold", 70.0, 70.0, model.ModeCooling, false},
		{"Cooling: above threshold", 75.0, 70.0, model.ModeCooling, true},
		{"Invalid mode", 70.0, 70.0, "invalid_mode", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldBeOn(tt.bt, tt.threshold, tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetThreshold(t *testing.T) {
	// Setup fake config
	env.Cfg = &config.Config{
		HeatingThreshold: 50.0,
		CoolingThreshold: 70.0,
		SecondaryMargin:  2.0,
		TertiaryMargin:   5.0,
	}

	tests := []struct {
		name     string
		role     string
		mode     model.SystemMode
		expected float64
	}{
		{"primary heating", "primary", model.ModeHeating, 50.0},
		{"secondary heating", "secondary", model.ModeHeating, 48.0},
		{"tertiary heating", "tertiary", model.ModeHeating, 45.0},
		{"primary cooling", "primary", model.ModeCooling, 70.0},
		{"secondary cooling", "secondary", model.ModeCooling, 72.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := getThreshold(tt.role, tt.mode)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetThreshold_ShutdownCases(t *testing.T) {
	// Override ExitFunc to avoid killing the test process
	shutdown.ExitFunc = func(code int) {
		panic(errors.New("shutdown called"))
	}
	defer func() { shutdown.ExitFunc = os.Exit }()

	tests := []struct {
		name string
		role string
		mode model.SystemMode
	}{
		{"invalid role", "invalid", model.ModeHeating},
		{"tertiary in cooling mode", "tertiary", model.ModeCooling},
		{"unknown mode", "primary", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic on shutdown, got none")
				}
			}()

			_ = getThreshold(tt.role, tt.mode) // should panic
		})
	}
}

func TestGetHeatSources(t *testing.T) {
	t.Run("one primary, one secondary, one boiler", func(t *testing.T) {
		hp1 := model.HeatPump{
			Device:    model.Device{Name: "hp1"},
			IsPrimary: true,
		}
		hp2 := model.HeatPump{
			Device:    model.Device{Name: "hp2"},
			IsPrimary: false,
		}

		boiler := model.Boiler{
			Device: model.Device{Name: "b1"},
		}

		env.SystemState = &state.SystemState{
			HeatPumps: []model.HeatPump{hp1, hp2},
			Boilers:   []model.Boiler{boiler},
		}

		sources := getHeatSources()
		assert.Equal(t, "hp1", sources.Primary.Name)
		assert.Equal(t, "hp2", sources.Secondary.Name)
		assert.Equal(t, "b1", sources.Tertiary.Name)
	})

	t.Run("multiple primaries should call shutdown", func(t *testing.T) {
		shutdownCalled := false
		shutdown.ExitFunc = func(code int) {
			shutdownCalled = true
			panic("shutdown triggered")
		}
		defer func() {
			shutdown.ExitFunc = os.Exit
			_ = recover() // swallow panic
		}()

		hp1 := model.HeatPump{
			Device:    model.Device{Name: "hp1"},
			IsPrimary: true,
		}
		hp2 := model.HeatPump{
			Device:    model.Device{Name: "hp2"},
			IsPrimary: true,
		}

		env.SystemState = &state.SystemState{
			HeatPumps: []model.HeatPump{hp1, hp2},
		}

		getHeatSources()

		if !shutdownCalled {
			t.Errorf("expected shutdown due to multiple primaries")
		}
	})

	t.Run("no primary, one heat pump and boiler", func(t *testing.T) {
		hp1 := model.HeatPump{
			Device:    model.Device{Name: "hp1"},
			IsPrimary: false,
		}

		boiler := model.Boiler{
			Device: model.Device{Name: "b1"},
		}

		env.SystemState = &state.SystemState{
			HeatPumps: []model.HeatPump{hp1},
			Boilers:   []model.Boiler{boiler},
		}

		sources := getHeatSources()
		assert.Nil(t, sources.Primary)
		assert.Equal(t, "hp1", sources.Secondary.Name)
		assert.Equal(t, "b1", sources.Tertiary.Name)
	})
}

func TestRefreshSources(t *testing.T) {
	now := time.Now()

	baseHP1 := model.HeatPump{
		Device: model.Device{
			Name:   "hp1",
			Online: true,
		},
		IsPrimary:   true,
		LastRotated: now.Add(-30 * time.Minute), // 30 min ago
	}

	baseHP2 := model.HeatPump{
		Device: model.Device{
			Name:   "hp2",
			Online: true,
		},
		IsPrimary:   false,
		LastRotated: now.Add(-30 * time.Minute),
	}

	t.Run("rotates primary if LastRotated exceeds RoleRotationMinutes", func(t *testing.T) {
		env.SystemState = &state.SystemState{
			SystemMode: model.ModeHeating,
			HeatPumps:  []model.HeatPump{baseHP1, baseHP2},
		}
		env.Cfg.RoleRotationMinutes = 10 // rotation threshold = 10 min

		result := refreshSources()

		assert.Equal(t, "hp2", result.Primary.Name)
		assert.Equal(t, "hp1", result.Secondary.Name)
		assert.True(t, result.Primary.IsPrimary)
		assert.False(t, result.Secondary.IsPrimary)
	})

	t.Run("does not rotate if LastRotated is recent", func(t *testing.T) {
		hp1 := baseHP1
		hp2 := baseHP2
		hp1.LastRotated = now.Add(-5 * time.Minute)
		hp2.LastRotated = now.Add(-5 * time.Minute)

		env.SystemState = &state.SystemState{
			SystemMode: model.ModeHeating,
			HeatPumps:  []model.HeatPump{hp1, hp2},
		}
		env.Cfg.RoleRotationMinutes = 10 // too soon

		result := refreshSources()

		assert.Equal(t, "hp1", result.Primary.Name)
		assert.Equal(t, "hp2", result.Secondary.Name)
	})

	t.Run("uses boiler if mode is heating and boiler is online", func(t *testing.T) {
		boiler := model.Boiler{
			Device: model.Device{
				Name:   "boil1",
				Online: true,
			},
		}

		env.SystemState = &state.SystemState{
			SystemMode: model.ModeHeating,
			HeatPumps:  []model.HeatPump{baseHP1, baseHP2},
			Boilers:    []model.Boiler{boiler},
		}
		env.Cfg.RoleRotationMinutes = 10

		result := refreshSources()
		assert.NotNil(t, result.Tertiary)
		assert.Equal(t, "boil1", result.Tertiary.Name)
	})

	t.Run("skips boiler in cooling mode", func(t *testing.T) {
		boiler := model.Boiler{
			Device: model.Device{
				Name:   "boil1",
				Online: true,
			},
		}

		env.SystemState = &state.SystemState{
			SystemMode: model.ModeCooling,
			HeatPumps:  []model.HeatPump{baseHP1, baseHP2},
			Boilers:    []model.Boiler{boiler},
		}
		env.Cfg.RoleRotationMinutes = 10

		result := refreshSources()
		assert.Nil(t, result.Tertiary)
	})
}

func TestEvaluateToggleSource(t *testing.T) {
	// Override CanToggle to control it
	originalCanToggle := device.CanToggle
	defer func() { device.CanToggle = originalCanToggle }()

	// Set config so thresholds are predictable
	env.Cfg = &config.Config{
		HeatingThreshold: 50.0,
		CoolingThreshold: 70.0,
		SecondaryMargin:  2.0,
		TertiaryMargin:   5.0,
	}

	tests := []struct {
		name       string
		role       string
		mode       model.SystemMode
		bt         float64
		active     bool
		canToggle  bool
		expectFlip bool
	}{
		// HEATING CASES
		{"Heating: should be on, but already on", "primary", model.ModeHeating, 40, true, true, false},
		{"Heating: should be on, currently off, can toggle", "primary", model.ModeHeating, 40, false, true, true},
		{"Heating: should be on, currently off, CANNOT toggle", "primary", model.ModeHeating, 40, false, false, false},
		{"Heating: should be off, currently on, can toggle", "primary", model.ModeHeating, 60, true, true, true},
		{"Heating: should be off, currently on, CANNOT toggle", "primary", model.ModeHeating, 60, true, false, false},

		// COOLING CASES
		{"Cooling: should be on, currently off, can toggle", "primary", model.ModeCooling, 80, false, true, true},
		{"Cooling: should be off, currently on, can toggle", "primary", model.ModeCooling, 60, true, true, true},
		{"Cooling: correct state, no toggle needed", "primary", model.ModeCooling, 60, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device.CanToggle = func(d *model.Device, now time.Time) bool {
				return tt.canToggle
			}

			result := evaluateToggleSource(tt.role, tt.bt, tt.active, &model.Device{Name: "test"}, tt.mode)
			assert.Equal(t, tt.expectFlip, result)
		})
	}
}

func TestEvaluateAndToggle(t *testing.T) {
	var activated, deactivated bool

	mockActivate := func() { activated = true }
	mockDeactivate := func() { deactivated = true }

	// Override evaluateToggleSource for control
	origEval := evaluateToggleSource
	defer func() { evaluateToggleSource = origEval }()
	evaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
		// simulate "should flip"
		return true
	}

	t.Run("should activate when currently off", func(t *testing.T) {
		activated, deactivated = false, false

		evaluateAndToggle("primary", model.Device{Name: "hp1"}, false, 45, model.ModeHeating, mockActivate, mockDeactivate)
		assert.True(t, activated)
		assert.False(t, deactivated)
	})

	t.Run("should deactivate when currently on", func(t *testing.T) {
		activated, deactivated = false, false

		evaluateAndToggle("secondary", model.Device{Name: "hp2"}, true, 55, model.ModeHeating, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.True(t, deactivated)
	})

	t.Run("no toggle needed", func(t *testing.T) {
		activated, deactivated = false, false

		// simulate "already in correct state"
		evaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
			return false
		}

		evaluateAndToggle("tertiary", model.Device{Name: "boil1"}, false, 60, model.ModeHeating, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.False(t, deactivated)
	})
}
