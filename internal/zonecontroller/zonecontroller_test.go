package zonecontroller

import (
	"testing"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func TestEvaluateZoneActions(t *testing.T) {
	tests := []struct {
		name               string
		handlerNil         bool
		blowerActive       bool
		pumpActive         bool
		loopNil            bool
		loopActive         bool
		canToggleHandler   bool
		canToggleLoop      bool
		temp               float64
		mode               model.SystemMode
		threshold          float64
		secondaryThreshold float64
		want               map[string]bool
	}{
		{
			name:               "Early out - neither handler nor loop can toggle",
			handlerNil:         false,
			blowerActive:       false,
			pumpActive:         false,
			loopNil:            false,
			loopActive:         false,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeHeating,
			threshold:          72,
			secondaryThreshold: 68,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Circulate mode - blower off, should activate blower",
			handlerNil:         false,
			blowerActive:       false,
			pumpActive:         false,
			loopNil:            true,
			loopActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			threshold:          0,
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   true,
				"deactivate_pump":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Circulate mode - blower and pump on, should deactivate pump",
			handlerNil:         false,
			blowerActive:       true,
			pumpActive:         true,
			loopNil:            true,
			loopActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			threshold:          0,
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   true,
				"deactivate_pump":   true,
				"activate_pump":     false,
				"deactivate_blower": false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Circulate mode - blower on, pump already off",
			handlerNil:         false,
			blowerActive:       true,
			pumpActive:         false,
			loopNil:            true,
			loopActive:         false,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			threshold:          0,
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"deactivate_pump":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Circulate mode - no handler present (invalid config)",
			handlerNil:         true,
			blowerActive:       false,
			pumpActive:         false,
			loopNil:            true,
			loopActive:         false,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			threshold:          0,
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"deactivate_pump":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Loop only - heating - should activate loop",
			handlerNil:         true,
			blowerActive:       false, // ignored
			pumpActive:         false, // ignored
			loopNil:            false,
			loopActive:         false,
			canToggleHandler:   false, // ignored
			canToggleLoop:      true,
			temp:               68,
			mode:               model.ModeHeating,
			threshold:          70, // temp < threshold => should be on
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_loop":     true,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"deactivate_blower": false,
				"activate_pump":     false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Loop only - heating - should deactivate loop",
			handlerNil:         true,
			loopNil:            false,
			loopActive:         true,
			temp:               72,
			mode:               model.ModeHeating,
			threshold:          70, // temp > threshold => should be off
			secondaryThreshold: 0,
			canToggleHandler:   false,
			canToggleLoop:      true,
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   true,
				"activate_blower":   false,
				"deactivate_blower": false,
				"activate_pump":     false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Loop only - heating - loop already on, no change",
			handlerNil:         true,
			loopNil:            false,
			loopActive:         true,
			temp:               68,
			mode:               model.ModeHeating,
			threshold:          70,
			secondaryThreshold: 0,
			canToggleHandler:   false,
			canToggleLoop:      true,
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"deactivate_blower": false,
				"activate_pump":     false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Loop only - heating - loop already off, no change",
			handlerNil:         true,
			loopNil:            false,
			loopActive:         false,
			temp:               72,
			mode:               model.ModeHeating,
			threshold:          70,
			secondaryThreshold: 0,
			canToggleHandler:   false,
			canToggleLoop:      true,
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"deactivate_blower": false,
				"activate_pump":     false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Loop only - cooling mode - should skip",
			handlerNil:         true,
			loopNil:            false,
			loopActive:         true,
			temp:               72,
			mode:               model.ModeCooling,
			threshold:          70,
			secondaryThreshold: 0,
			canToggleHandler:   false,
			canToggleLoop:      true,
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"deactivate_blower": false,
				"activate_pump":     false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Handler only - should activate blower and pump",
			handlerNil:         false,
			loopNil:            true,
			blowerActive:       false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               74,
			mode:               model.ModeCooling,
			threshold:          72, // shouldBeOn = true
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   true,
				"activate_pump":     true,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Handler only - should deactivate blower and pump",
			handlerNil:         false,
			loopNil:            true,
			blowerActive:       true,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeCooling,
			threshold:          72, // shouldBeOn = false
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": true,
				"deactivate_pump":   true,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Handler only - pump already on, should stay on",
			handlerNil:         false,
			loopNil:            true,
			blowerActive:       true,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               74,
			mode:               model.ModeCooling,
			threshold:          72, // shouldBeOn = true
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Handler only - pump already off, no action needed",
			handlerNil:         false,
			loopNil:            true,
			blowerActive:       false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeCooling,
			threshold:          72, // shouldBeOn = false
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Both - cooling - should activate blower and pump",
			handlerNil:         false,
			loopNil:            false,
			blowerActive:       false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               76,
			mode:               model.ModeCooling,
			threshold:          74, // shouldPrimary = true
			secondaryThreshold: 0,  // ignored
			want: map[string]bool{
				"activate_blower":   true,
				"activate_pump":     true,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Both - cooling - should deactivate blower and pump",
			handlerNil:         false,
			loopNil:            false,
			blowerActive:       true,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               72,
			mode:               model.ModeCooling,
			threshold:          74, // shouldPrimary = false
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": true,
				"deactivate_pump":   true,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Both - cooling - already active, no change",
			handlerNil:         false,
			loopNil:            false,
			blowerActive:       true,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               76,
			mode:               model.ModeCooling,
			threshold:          74, // shouldPrimary = true
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Both - cooling - already off, no change",
			handlerNil:         false,
			loopNil:            false,
			blowerActive:       false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               72,
			mode:               model.ModeCooling,
			threshold:          74, // shouldPrimary = false
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Both - heating - activate loop only",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               66,
			mode:               model.ModeHeating,
			threshold:          68, // shouldPrimary = true
			secondaryThreshold: 64, // shouldSecondary = false
			want: map[string]bool{
				"activate_loop":     true,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Both - heating - deactivate loop only",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         true,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               70,
			mode:               model.ModeHeating,
			threshold:          68, // shouldPrimary = false
			secondaryThreshold: 64, // shouldSecondary = false
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   true,
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Both - heating - activate handler only (backup heat)",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         true,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               63,
			mode:               model.ModeHeating,
			threshold:          66, // shouldPrimary = false
			secondaryThreshold: 65, // shouldSecondary = true
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   true,
				"activate_pump":     true,
				"deactivate_blower": false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Both - heating - deactivate handler only",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         false,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               68,
			mode:               model.ModeHeating,
			threshold:          66, // shouldPrimary = false
			secondaryThreshold: 65, // shouldSecondary = false
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": true,
				"deactivate_pump":   true,
			},
		},
		{
			name:               "Both - heating - all already on, no changes",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         true,
			pumpActive:         true,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               64,
			mode:               model.ModeHeating,
			threshold:          66, // shouldPrimary = true
			secondaryThreshold: 65, // shouldSecondary = true
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
			},
		},
		{
			name:               "Both - heating - all already off, no changes",
			handlerNil:         false,
			loopNil:            false,
			loopActive:         false,
			pumpActive:         false,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               70,
			mode:               model.ModeHeating,
			threshold:          66, // shouldPrimary = false
			secondaryThreshold: 65, // shouldSecondary = false
			want: map[string]bool{
				"activate_loop":     false,
				"deactivate_loop":   false,
				"activate_blower":   false,
				"activate_pump":     false,
				"deactivate_blower": false,
				"deactivate_pump":   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateZoneActions(
				"test-zone",
				tt.handlerNil,
				tt.blowerActive,
				tt.pumpActive,
				tt.loopNil,
				tt.loopActive,
				tt.canToggleHandler,
				tt.canToggleLoop,
				tt.temp,
				tt.mode,
				tt.threshold,
				tt.secondaryThreshold,
			)

			for key, wantVal := range tt.want {
				if got[key] != wantVal {
					t.Errorf("evaluateZoneActions[%s] = %v; want %v", key, got[key], wantVal)
				}
			}
		})
	}
}
