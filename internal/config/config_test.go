package config

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected zerolog.Level
	}{
		{"default to info", "", zerolog.InfoLevel},
		{"debug", "debug", zerolog.DebugLevel},
		{"warn", "warn", zerolog.WarnLevel},
		{"error", "error", zerolog.ErrorLevel},
		{"unknown", "weird", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := parseLogLevel(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestConfigValidate_UniqueZones(t *testing.T) {
	cfg := &Config{
		Zones: []model.Zone{
			{ID: "zone1"},
			{ID: "zone1"}, // duplicate
		},
	}

	assert.PanicsWithValue(t,
		"Duplicate zone ID found: zone1",
		func() { cfg.validate() },
	)
}

func TestConfigValidate_UnknownZoneReference(t *testing.T) {
	cfg := &Config{
		Zones: []model.Zone{
			{ID: "zone1"},
		},
		DeviceConfig: DeviceConfig{
			AirHandlers: AirHandlerGroup{
				Devices: []AirHandlerConfig{
					{Name: "ah1", Zone: "zoneX"},
				},
			},
		},
	}

	assert.PanicsWithValue(t,
		"Air handler ah1 references unknown zone ID: zoneX",
		func() { cfg.validate() },
	)
}

func TestConfigValidate_GPIOConflict(t *testing.T) {
	cfg := &Config{
		TempSensorBusGPIO: 4,
		MainPowerGPIO:     4, // conflict with above
		Zones:             []model.Zone{{ID: "zone1"}},
	}

	assert.PanicsWithValue(t,
		"GPIO pin conflict: temp_sensor_bus_gpio and main_power_gpio both use pin 4",
		func() { cfg.validate() },
	)
}
