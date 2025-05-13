package config

import (
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestValidate_GPIOValid(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			"temp_sensor_bus": &GPIOPin{Pin: 4, SafeState: nil}, // unmanaged

			"boiler_relay":          &GPIOPin{Pin: 17, SafeState: boolPtr(true)},
			"main_power_relay":      &GPIOPin{Pin: 23, SafeState: boolPtr(false)},
			"main_floor_air_blower": &GPIOPin{Pin: 5, SafeState: boolPtr(true)},
			"basement_air_pump":     &GPIOPin{Pin: 6, SafeState: boolPtr(true)},
		},
	}

	cfg.validate() // should not panic
}

func TestValidate_GPIO_Missing(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			"boiler_relay":     nil,
			"main_power_relay": &GPIOPin{Pin: 23, SafeState: boolPtr(false)},
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
			"boiler_relay":     &GPIOPin{Pin: 17, SafeState: boolPtr(true)},
			"main_power_relay": &GPIOPin{Pin: 17, SafeState: boolPtr(false)}, // conflict
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to conflicting pin numbers, but got none")
		}
	}()

	cfg.validate()
}
