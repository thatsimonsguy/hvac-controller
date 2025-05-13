package config

import (
	"testing"
)

func TestValidate_GPIOValid(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			"main_floor_temp_sensor": &GPIOPin{Pin: 5, SafeState: true},
			"basement_temp_sensor":   &GPIOPin{Pin: 6, SafeState: true},
			"garage_temp_sensor":     &GPIOPin{Pin: 12, SafeState: true},
			"main_floor_air_blower":  &GPIOPin{Pin: 13, SafeState: true},
			"main_floor_air_pump":    &GPIOPin{Pin: 16, SafeState: true},
			"basement_air_blower":    &GPIOPin{Pin: 19, SafeState: true},
			"basement_air_pump":      &GPIOPin{Pin: 20, SafeState: true},
			"basement_radiant_pump":  &GPIOPin{Pin: 21, SafeState: true},
			"garage_radiant_pump":    &GPIOPin{Pin: 26, SafeState: true},
			"boiler_relay":           &GPIOPin{Pin: 17, SafeState: true},
			"heat_pump_A_relay":      &GPIOPin{Pin: 27, SafeState: true},
			"heat_pump_B_relay":      &GPIOPin{Pin: 22, SafeState: true},
			"buffer_temp_sensor":     &GPIOPin{Pin: 4, SafeState: true},
			"main_power_relay":       &GPIOPin{Pin: 23, SafeState: false},
		},
	}

	cfg.validate() // should not panic
}

func TestValidate_GPIO_Missing(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			"main_floor_temp_sensor": nil, // Missing
			"basement_temp_sensor":   &GPIOPin{Pin: 6, SafeState: true},
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to missing GPIO config, but got none")
		}
	}()

	cfg.validate()
}

func TestValidate_GPIO_Conflict(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			"main_floor_temp_sensor": &GPIOPin{Pin: 5, SafeState: true},
			"basement_temp_sensor":   &GPIOPin{Pin: 5, SafeState: true}, // Conflict
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to conflicting pin numbers, but got none")
		}
	}()

	cfg.validate()
}
