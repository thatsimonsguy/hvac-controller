package buffercontroller_test

import (
	"testing"
	"time"

	"github.com/thatsimonsguy/hvac-controller/internal/controllers/buffercontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func TestShouldToggle(t *testing.T) {
	tests := []struct {
		name           string
		pumpActive     bool
		modeActive     bool
		sysMode        model.SystemMode
		canToggle      bool
		online         bool
		minOn          time.Duration
		expectedResult bool
		expectPumpCall bool
	}{
		{
			name:           "No toggle needed - cooling/cooling",
			pumpActive:     false,
			modeActive:     true,
			sysMode:        model.ModeCooling,
			canToggle:      true,
			online:         true,
			minOn:          0,
			expectedResult: false,
			expectPumpCall: false,
		},
		{
			name:           "No toggle needed - heating/heating",
			pumpActive:     false,
			modeActive:     false,
			sysMode:        model.ModeHeating,
			canToggle:      true,
			online:         true,
			minOn:          0,
			expectedResult: false,
			expectPumpCall: false,
		},
		{
			name:           "Toggle needed - from cooling to heating with pump running and canToggle false",
			pumpActive:     true,
			modeActive:     true,
			sysMode:        model.ModeHeating,
			canToggle:      false,
			online:         true,
			minOn:          1 * time.Millisecond,
			expectedResult: true,
			expectPumpCall: true,
		},
		{
			name:           "Toggle needed - from heating to cooling with pump running and canToggle false",
			pumpActive:     true,
			modeActive:     false,
			sysMode:        model.ModeCooling,
			canToggle:      false,
			online:         true,
			minOn:          1 * time.Millisecond,
			expectedResult: true,
			expectPumpCall: true,
		},
		{
			name:           "Toggle needed - from heating to cooling with pump not running",
			pumpActive:     false,
			modeActive:     false,
			sysMode:        model.ModeCooling,
			canToggle:      false,
			online:         true,
			minOn:          1 * time.Millisecond,
			expectedResult: true,
			expectPumpCall: false,
		},
		{
			name:           "Toggle needed - from cooling to heating with pump not running",
			pumpActive:     false,
			modeActive:     true,
			sysMode:        model.ModeHeating,
			canToggle:      false,
			online:         true,
			minOn:          1 * time.Millisecond,
			expectedResult: true,
			expectPumpCall: false,
		},
		{
			name:           "No toggle - pump offline when trying to switch into cooling",
			pumpActive:     false,
			modeActive:     false,
			sysMode:        model.ModeCooling,
			canToggle:      false,
			online:         false,
			minOn:          1 * time.Millisecond,
			expectedResult: false,
			expectPumpCall: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pumpCalled := false
			mockPumpDeactivate := func() { pumpCalled = true }
			mockSleep := func(d time.Duration) { /* no-op */ }

			result := buffercontroller.ShouldToggle(
				tc.pumpActive,
				tc.modeActive,
				tc.sysMode,
				tc.canToggle,
				tc.online,
				tc.minOn,
				mockPumpDeactivate,
				mockSleep,
			)

			if result != tc.expectedResult {
				t.Errorf("expected result %v, got %v", tc.expectedResult, result)
			}

			if pumpCalled != tc.expectPumpCall {
				t.Errorf("expected pump call %v, got %v", tc.expectPumpCall, pumpCalled)
			}
		})
	}
}
