package gpio

import (
	"testing"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestValidateStartupPins_Valid(t *testing.T) {
	ResetGPIO()

	// Simulate GPIO state
	fakeState := map[int]bool{}
	MockGPIO(
		func(pin int, state bool) { fakeState[pin] = state },
		func(pin int) bool { return fakeState[pin] },
	)

	cfg := config.Config{
		GPIO: config.GPIO{
			"boiler_relay":     &config.GPIOPin{Pin: 17, SafeState: boolPtr(true)},
			"main_power_relay": &config.GPIOPin{Pin: 23, SafeState: boolPtr(false)},
			"temp_sensor_bus":  &config.GPIOPin{Pin: 4, SafeState: nil},
		},
	}

	fakeState[17] = true
	fakeState[23] = false

	if err := ValidateStartupPins(cfg); err != nil {
		t.Fatalf("expected valid state, got error: %v", err)
	}
}

func TestValidateStartupPins_Mismatch(t *testing.T) {
	ResetGPIO()

	fakeState := map[int]bool{}
	MockGPIO(
		func(pin int, state bool) { fakeState[pin] = state },
		func(pin int) bool { return fakeState[pin] },
	)

	cfg := config.Config{
		GPIO: config.GPIO{
			"boiler_relay": &config.GPIOPin{Pin: 17, SafeState: boolPtr(true)},
		},
	}

	fakeState[17] = false // mismatch

	if err := ValidateStartupPins(cfg); err == nil {
		t.Fatal("expected error due to GPIO state mismatch, got nil")
	}
}

func TestValidateStartupPins_SkipUnmanaged(t *testing.T) {
	ResetGPIO()

	fakeState := map[int]bool{}
	MockGPIO(
		func(pin int, state bool) { fakeState[pin] = state },
		func(pin int) bool { return fakeState[pin] },
	)

	cfg := config.Config{
		GPIO: config.GPIO{
			"temp_sensor_bus": &config.GPIOPin{Pin: 4, SafeState: nil},
		},
	}

	fakeState[4] = false // shouldn't matter

	if err := ValidateStartupPins(cfg); err != nil {
		t.Fatalf("expected safe pass on unmanaged pin, got error: %v", err)
	}
}
