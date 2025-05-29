package zonecontroller

import (
	"testing"
	"time"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var testHandler = &model.AirHandler{
	Device: model.Device{
		Name: "test-handler",
		Pin: model.GPIOPin{
			Number:     1,
			ActiveHigh: true,
		},
		MinOn:       10,
		MinOff:      10,
		Online:      true,
		LastChanged: time.Now(),
		ActiveModes: []string{"heating", "cooling"},
	},
	Zone: &model.Zone{
		ID: "test-zone",
	},
	CircPumpPin: model.GPIOPin{
		Number:     2,
		ActiveHigh: false,
	},
}

var testLoop = &model.RadiantFloorLoop{
	Device: model.Device{
		Name: "test-loop",
		Pin: model.GPIOPin{
			Number:     2,
			ActiveHigh: true,
		},
		MinOn:       10,
		MinOff:      10,
		Online:      true,
		LastChanged: time.Now(),
		ActiveModes: []string{"heating"},
	},
	Zone: &model.Zone{
		ID: "test-zone",
	},
}

func TestEvaluateZoneActions(t *testing.T) {
	tests := []struct {
		name               string
		blowerActive       bool
		pumpActive         bool
		loopActive         bool
		handler            *model.AirHandler
		loop               *model.RadiantFloorLoop
		canToggleHandler   bool
		canToggleLoop      bool
		temp               float64
		mode               model.SystemMode
		sysMode            model.SystemMode
		threshold          float64
		secondaryThreshold float64
		want               map[string]bool
	}{
		{
			name:               "Early out - neither handler nor loop can toggle",
			blowerActive:       false,
			pumpActive:         false,
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			blowerActive:       false,
			pumpActive:         false,
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			sysMode:            model.ModeCirculate,
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
			blowerActive:       true,
			pumpActive:         true,
			loopActive:         false,
			handler:            testHandler,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			sysMode:            model.ModeCirculate,
			threshold:          0,
			secondaryThreshold: 0,
			want: map[string]bool{
				"activate_blower":   false,
				"deactivate_pump":   true,
				"activate_pump":     false,
				"deactivate_blower": false,
				"activate_loop":     false,
				"deactivate_loop":   false,
			},
		},
		{
			name:               "Circulate mode - blower on, pump already off",
			blowerActive:       true,
			pumpActive:         false,
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			sysMode:            model.ModeCirculate,
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
			blowerActive:       false,
			pumpActive:         false,
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   false,
			canToggleLoop:      false,
			temp:               0,
			mode:               model.ModeCirculate,
			sysMode:            model.ModeCirculate,
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
			blowerActive:       false, // ignored
			pumpActive:         false, // ignored
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   false, // ignored
			canToggleLoop:      true,
			temp:               68,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			temp:               72,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			temp:               68,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			temp:               72,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			loop:               testLoop,
			temp:               72,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               74,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       true,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       true,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               74,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      false,
			temp:               70,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               76,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       true,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               72,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       true,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               76,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			blowerActive:       false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               72,
			mode:               model.ModeCooling,
			sysMode:            model.ModeCooling,
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
			loopActive:         false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               66,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               70,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               63,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         false,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               68,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         true,
			pumpActive:         true,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               64,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			loopActive:         false,
			pumpActive:         false,
			handler:            testHandler,
			loop:               testLoop,
			canToggleHandler:   true,
			canToggleLoop:      true,
			temp:               70,
			mode:               model.ModeHeating,
			sysMode:            model.ModeHeating,
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
			got, err := evaluateZoneActions(
				"test-zone",
				tt.blowerActive,
				tt.pumpActive,
				tt.loopActive,
				tt.handler,
				tt.loop,
				tt.canToggleHandler,
				tt.canToggleLoop,
				tt.temp,
				tt.mode,
				tt.sysMode,
				tt.threshold,
				tt.secondaryThreshold,
			)

			if err != nil {
				t.Errorf("evaluateZoneActions() error = %v", err)
			}

			for key, wantVal := range tt.want {
				if got[key] != wantVal {
					t.Errorf("evaluateZoneActions[%s] = %v; want %v", key, got[key], wantVal)
				}
			}
		})
	}
}
