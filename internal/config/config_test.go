package config

import (
	"testing"
)

func intPtr(i int) *int {
	return &i
}

func TestValidate_GPIOValid(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			MainFloorTempSensor: intPtr(5),
			BasementTempSensor:  intPtr(6),
			GarageTempSensor:    intPtr(12),

			MainFloorAirBlower: intPtr(13),
			MainFloorAirPump:   intPtr(16),
			BasementAirBlower:  intPtr(19),
			BasementAirPump:    intPtr(20),

			BasementRadiantPump: intPtr(21),
			GarageRadiantPump:   intPtr(26),

			BoilerRelayPin:      intPtr(17),
			HeatPumpARelayPin:   intPtr(27),
			HeatPumpBRelayPin:   intPtr(22),
			BufferTempSensorPin: intPtr(4),
			MainPowerRelayPin:   intPtr(23),
		},
	}

	// should not panic
	cfg.validate()
}

func TestValidate_MissingPin(t *testing.T) {
	cfg := Config{
		GPIO: GPIO{
			MainFloorTempSensor: nil,
			BasementTempSensor:  intPtr(6),
			GarageTempSensor:    intPtr(12),
			MainFloorAirBlower:  intPtr(13),
			MainFloorAirPump:    intPtr(16),
			BasementAirBlower:   intPtr(19),
			BasementAirPump:     intPtr(20),
			BasementRadiantPump: intPtr(21),
			GarageRadiantPump:   intPtr(26),
			BoilerRelayPin:      intPtr(17),
			HeatPumpARelayPin:   intPtr(27),
			HeatPumpBRelayPin:   intPtr(22),
			BufferTempSensorPin: intPtr(4),
			MainPowerRelayPin:   intPtr(23),
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to missing pin, but did not panic")
		}
	}()

	cfg.validate()
}

func TestValidate_ConflictingPins(t *testing.T) {
	samePin := intPtr(4)

	cfg := Config{
		GPIO: GPIO{
			MainFloorTempSensor: samePin,
			BasementTempSensor:  samePin, // conflict
			GarageTempSensor:    intPtr(12),
			MainFloorAirBlower:  intPtr(13),
			MainFloorAirPump:    intPtr(16),
			BasementAirBlower:   intPtr(19),
			BasementAirPump:     intPtr(20),
			BasementRadiantPump: intPtr(21),
			GarageRadiantPump:   intPtr(26),
			BoilerRelayPin:      intPtr(17),
			HeatPumpARelayPin:   intPtr(27),
			HeatPumpBRelayPin:   intPtr(22),
			BufferTempSensorPin: intPtr(5),
			MainPowerRelayPin:   intPtr(23),
		},
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic due to conflicting pins, but did not panic")
		}
	}()

	cfg.validate()
}
