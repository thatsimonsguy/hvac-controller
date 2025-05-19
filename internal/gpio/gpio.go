package gpio

import (
	"fmt"

	"github.com/thatsimonsguy/hvac-controller/internal/pinctrl"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

var safeMode bool

func ValidateInitialPinStates(state *state.SystemState) error {
	type pinWithMeta struct {
		Name       string
		Pin        model.GPIOPin
		ShouldBeOn bool
	}

	var checks []pinWithMeta

	for _, hp := range state.HeatPumps {
		checks = append(checks, pinWithMeta{
			Name:       hp.Name,
			Pin:        hp.Pin,
			ShouldBeOn: false,
		})

		modeActive := contains(hp.Device.ActiveModes, string(state.SystemMode)) &&
			state.SystemMode == model.ModeCooling && hp.Device.Online

		checks = append(checks, pinWithMeta{
			Name:       hp.Name + ".mode_pin",
			Pin:        hp.ModePin,
			ShouldBeOn: modeActive,
		})
	}

	for _, ah := range state.AirHandlers {
		checks = append(checks,
			pinWithMeta{ah.Name, ah.Pin, false},
			pinWithMeta{ah.Name + ".circ_pump", ah.CircPumpPin, false},
		)
	}
	for _, b := range state.Boilers {
		checks = append(checks, pinWithMeta{b.Name, b.Pin, false})
	}
	for _, rf := range state.RadiantLoops {
		checks = append(checks, pinWithMeta{rf.Name, rf.Pin, false})
	}

	checks = append(checks, pinWithMeta{"main_power", state.MainPowerPin, false})

	for _, check := range checks {
		level, err := pinctrl.ReadLevel(check.Pin.Number)
		if err != nil {
			return fmt.Errorf("failed to read pin level for %s (GPIO %d): %w", check.Name, check.Pin.Number, err)
		}
		isActive := (check.Pin.ActiveHigh && level) || (!check.Pin.ActiveHigh && !level)
		if isActive != check.ShouldBeOn {
			return fmt.Errorf("pin %d (%s) is in wrong state at startup (expected active=%v)", check.Pin.Number, check.Name, check.ShouldBeOn)
		}
	}

	return nil
}

func contains(list []string, val string) bool {
	for _, s := range list {
		if s == val {
			return true
		}
	}
	return false
}


func SetSafeMode(enabled bool) {
	safeMode = enabled
}

func Read(pin model.GPIOPin) (bool, error) {
	return pinctrl.ReadLevel(pin.Number)
}

func Activate(pin model.GPIOPin) error {
	if safeMode {
		return nil
	}
	if pin.ActiveHigh {
		return pinctrl.SetPin(pin.Number, "op", "pn", "dh")
	}
	return pinctrl.SetPin(pin.Number, "op", "pn", "dl")
}

func Deactivate(pin model.GPIOPin) error {
	if safeMode {
		return nil
	}
	if pin.ActiveHigh {
		return pinctrl.SetPin(pin.Number, "op", "pn", "dl")
	}
	return pinctrl.SetPin(pin.Number, "op", "pn", "dh")
}