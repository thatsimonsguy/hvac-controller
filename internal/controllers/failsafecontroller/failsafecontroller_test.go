package failsafecontroller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func TestIsZoneIgnored(t *testing.T) {
	assert.True(t, isZoneIgnored("garage"))
	assert.False(t, isZoneIgnored("living_room"))
	assert.False(t, isZoneIgnored("bedroom"))
	assert.False(t, isZoneIgnored("basement"))
}

func TestEvaluateFailsafeActions_IgnoredZoneTemperatureUnsafe(t *testing.T) {
	// Create zone states where garage has unsafe temperature but should be ignored
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "garage"},
			Temperature: 10.0, // Way too cold, but should be ignored
		},
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 72.0, // Normal temperature
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	// Should not trigger override despite garage being very cold
	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride)
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_IgnoredZoneWithNormalZoneUnsafe(t *testing.T) {
	// Garage is cold (ignored), but living room is also cold (not ignored)
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "garage"},
			Temperature: 10.0, // Too cold but ignored
		},
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 45.0, // Too cold and not ignored - should trigger
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	// Should trigger override based on living room, not garage
	assert.True(t, action.SetOverride)
	assert.Equal(t, model.ModeHeating, action.OverrideMode)
	assert.Equal(t, "living_room", action.TriggerZone)
	assert.Equal(t, 45.0, action.TriggerTemp)
	assert.Contains(t, action.ActivateZones, "living_room")
	assert.Equal(t, model.ModeHeating, action.ActivateZones["living_room"])
}

// Test comprehensive coverage of all logic paths

func TestEvaluateFailsafeActions_NormalTemperatures(t *testing.T) {
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 72.0, // Normal temperature
		},
		{
			Zone:        model.Zone{ID: "bedroom"},
			Temperature: 68.0, // Normal temperature
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride)
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_TooHot(t *testing.T) {
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 90.0, // Too hot
		},
		{
			Zone:        model.Zone{ID: "bedroom"},
			Temperature: 70.0, // Normal
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.True(t, action.SetOverride)
	assert.Equal(t, model.ModeCooling, action.OverrideMode)
	assert.Equal(t, "living_room", action.TriggerZone)
	assert.Equal(t, 90.0, action.TriggerTemp)
	assert.Contains(t, action.ActivateZones, "living_room")
	assert.Equal(t, model.ModeCooling, action.ActivateZones["living_room"])
}

func TestEvaluateFailsafeActions_TooCold(t *testing.T) {
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "basement"},
			Temperature: 45.0, // Too cold
		},
		{
			Zone:        model.Zone{ID: "bedroom"},
			Temperature: 70.0, // Normal
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.True(t, action.SetOverride)
	assert.Equal(t, model.ModeHeating, action.OverrideMode)
	assert.Equal(t, "basement", action.TriggerZone)
	assert.Equal(t, 45.0, action.TriggerTemp)
	assert.Contains(t, action.ActivateZones, "basement")
	assert.Equal(t, model.ModeHeating, action.ActivateZones["basement"])
}

func TestEvaluateFailsafeActions_FirstUnsafeZoneWins(t *testing.T) {
	// Multiple zones unsafe - first one found should trigger
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "basement"},
			Temperature: 45.0, // Too cold - should trigger heating
		},
		{
			Zone:        model.Zone{ID: "attic"},
			Temperature: 90.0, // Too hot - but second in list
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.True(t, action.SetOverride)
	assert.Equal(t, model.ModeHeating, action.OverrideMode) // First unsafe zone was too cold
	assert.Equal(t, "basement", action.TriggerZone)
	assert.Equal(t, 45.0, action.TriggerTemp)
}

func TestEvaluateFailsafeActions_OverrideAlreadyActive_NeedsOverride(t *testing.T) {
	// Override is active and still needed
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 45.0, // Still too cold
		},
	}

	action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0)

	// Should not set override again, but also shouldn't clear it
	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride)
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_OverrideActive_AllZonesSafe(t *testing.T) {
	// Override is active but all zones are now safe (within spread bounds)
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 72.0, // Between 52 and 83 (safe range with spread)
		},
		{
			Zone:        model.Zone{ID: "bedroom"},
			Temperature: 68.0, // Between 52 and 83 (safe range with spread)
		},
	}

	action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0)

	assert.False(t, action.SetOverride)
	assert.True(t, action.ClearOverride)
	assert.Equal(t, 2, len(action.DeactivateZones))
	assert.Contains(t, action.DeactivateZones, "living_room")
	assert.Contains(t, action.DeactivateZones, "bedroom")
}

func TestEvaluateFailsafeActions_OverrideActive_OneZoneStillUnsafe(t *testing.T) {
	// Override is active, one zone safe, one still too close to boundary
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 72.0, // Safe
		},
		{
			Zone:        model.Zone{ID: "bedroom"},
			Temperature: 51.0, // Still too close to min (50 + 2 = 52 safe minimum)
		},
	}

	action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0)

	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride) // Don't clear yet - still unsafe
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_BoundaryConditions(t *testing.T) {
	// Test exact boundary conditions
	tests := []struct {
		name          string
		temp          float64
		expectTrigger bool
		expectMode    model.SystemMode
	}{
		{"Exactly at min threshold", 50.0, false, ""},
		{"Just below min threshold", 49.9, true, model.ModeHeating},
		{"Exactly at max threshold", 85.0, false, ""},
		{"Just above max threshold", 85.1, true, model.ModeCooling},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zoneStates := []ZoneState{
				{
					Zone:        model.Zone{ID: "test_zone"},
					Temperature: tt.temp,
				},
			}

			action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

			assert.Equal(t, tt.expectTrigger, action.SetOverride)
			if tt.expectTrigger {
				assert.Equal(t, tt.expectMode, action.OverrideMode)
			}
		})
	}
}

func TestEvaluateFailsafeActions_SpreadBoundaryConditions(t *testing.T) {
	// Test spread boundary conditions for clearing override
	// With minTemp=50, maxTemp=85, spread=2: safe range is 52-83
	tests := []struct {
		name        string
		temp        float64
		expectClear bool
	}{
		{"Exactly at safe min (52.0)", 52.0, true},
		{"Just below safe min (51.9)", 51.9, false},
		{"Exactly at safe max (83.0)", 83.0, true},
		{"Just above safe max (83.1)", 83.1, false},
		{"Well within safe range (70.0)", 70.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zoneStates := []ZoneState{
				{
					Zone:        model.Zone{ID: "test_zone"},
					Temperature: tt.temp,
				},
			}

			action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0) // Override active

			assert.Equal(t, tt.expectClear, action.ClearOverride)
		})
	}
}

func TestEvaluateFailsafeActions_EmptyZoneStates(t *testing.T) {
	// Test with no zones
	zoneStates := []ZoneState{}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride)
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_OnlyIgnoredZones(t *testing.T) {
	// Test with only ignored zones
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "garage"},
			Temperature: 10.0, // Way too cold but ignored
		},
	}

	action := evaluateFailsafeActions(zoneStates, false, 50.0, 85.0, 2.0)

	assert.False(t, action.SetOverride)
	assert.False(t, action.ClearOverride)
	assert.Equal(t, 0, len(action.ActivateZones))
	assert.Equal(t, 0, len(action.DeactivateZones))
}

func TestEvaluateFailsafeActions_OverrideActive_OnlyIgnoredZones(t *testing.T) {
	// Test clearing override when only ignored zones present
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "garage"},
			Temperature: 70.0, // Normal temp but ignored
		},
	}

	action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0) // Override active

	// Should clear override since no non-ignored zones exist
	assert.False(t, action.SetOverride)
	assert.True(t, action.ClearOverride)
	assert.Equal(t, 1, len(action.DeactivateZones))
	assert.Contains(t, action.DeactivateZones, "garage")
}

func TestEvaluateFailsafeActions_IgnoredZoneInDeactivationList(t *testing.T) {
	// When clearing override, ignored zones should still be added to deactivation list
	zoneStates := []ZoneState{
		{
			Zone:        model.Zone{ID: "living_room"},
			Temperature: 70.0, // Safe
		},
		{
			Zone:        model.Zone{ID: "garage"},
			Temperature: 20.0, // Cold but ignored
		},
	}

	action := evaluateFailsafeActions(zoneStates, true, 50.0, 85.0, 2.0) // Override active

	assert.True(t, action.ClearOverride)
	assert.Equal(t, 2, len(action.DeactivateZones))
	assert.Contains(t, action.DeactivateZones, "living_room")
	assert.Contains(t, action.DeactivateZones, "garage") // Even ignored zones get deactivated
}