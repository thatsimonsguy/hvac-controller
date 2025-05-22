package gpio

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thatsimonsguy/hvac-controller/internal/pinctrl"

	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

var safeMode bool

func ValidateInitialPinStates() error {
	type pinWithMeta struct {
		Name       string
		Pin        model.GPIOPin
		ShouldBeOn bool
	}

	var checks []pinWithMeta

	for _, hp := range env.SystemState.HeatPumps {
		checks = append(checks, pinWithMeta{
			Name:       hp.Name,
			Pin:        hp.Pin,
			ShouldBeOn: false,
		})

		modeActive := contains(hp.Device.ActiveModes, string(env.SystemState.SystemMode)) &&
			env.SystemState.SystemMode == model.ModeCooling && hp.Device.Online

		checks = append(checks, pinWithMeta{
			Name:       hp.Name + ".mode_pin",
			Pin:        hp.ModePin,
			ShouldBeOn: modeActive,
		})
	}

	for _, ah := range env.SystemState.AirHandlers {
		checks = append(checks,
			pinWithMeta{ah.Name, ah.Pin, false},
			pinWithMeta{ah.Name + ".circ_pump", ah.CircPumpPin, false},
		)
	}
	for _, b := range env.SystemState.Boilers {
		checks = append(checks, pinWithMeta{b.Name, b.Pin, false})
	}
	for _, rf := range env.SystemState.RadiantLoops {
		checks = append(checks, pinWithMeta{rf.Name, rf.Pin, false})
	}

	checks = append(checks, pinWithMeta{"main_power", env.SystemState.MainPowerPin, false})

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

func Read(pin model.GPIOPin) bool {
	level, err := pinctrl.ReadLevel(pin.Number)
	if err != nil {
		shutdown.ShutdownWithError(err, fmt.Sprintf("Failed to read pin level for pin %d", pin.Number))
	}
	return level
}

func Activate(pin model.GPIOPin) {
	if safeMode {
		return
	}

	if pin.ActiveHigh {
		err := pinctrl.SetPin(pin.Number, "op", "pn", "dh")
		if err != nil {
			shutdown.ShutdownWithError(err, fmt.Sprintf("Failed to activate pin %d", pin.Number))
		}
		return
	}

	err := pinctrl.SetPin(pin.Number, "op", "pn", "dl")
	if err != nil {
		shutdown.ShutdownWithError(err, fmt.Sprintf("Failed to activate pin %d", pin.Number))
	}
}

func Deactivate(pin model.GPIOPin) {
	if safeMode {
		return
	}

	if pin.ActiveHigh {
		err := pinctrl.SetPin(pin.Number, "op", "pn", "dl")
		if err != nil {
			shutdown.ShutdownWithError(err, fmt.Sprintf("Failed to deactivate pin %d", pin.Number))
		}
		return
	}

	err := pinctrl.SetPin(pin.Number, "op", "pn", "dh")
	if err != nil {
		shutdown.ShutdownWithError(err, fmt.Sprintf("Failed to deactivate pin %d", pin.Number))
	}
}

func CurrentlyActive(pin model.GPIOPin) bool {
	level := Read(pin)
	return pin.ActiveHigh == level
}

func ReadSensorTemp(sensorPath string) float64 {
	file := filepath.Join(sensorPath, "w1_slave")
	data, err := os.ReadFile(file)
	if err != nil {
		shutdown.ShutdownWithError(fmt.Errorf("failed to read sensor data: %w", err), "fatal sensor read failure")
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "t=") {
		shutdown.ShutdownWithError(fmt.Errorf("temperature data missing or malformed"), "fatal sensor read failure")
	}

	parts := strings.Split(lines[1], "t=")
	if len(parts) != 2 {
		shutdown.ShutdownWithError(fmt.Errorf("could not parse temperature line: %s", lines[1]), "fatal sensor read failure")
	}

	tempMilliC, err := strconv.Atoi(parts[1])
	if err != nil {
		shutdown.ShutdownWithError(fmt.Errorf("failed to convert temperature to int: %w", err), "fatal sensor read failure")
	}

	// Celsius to Fahrenheit: F = C × 9/5 + 32
	tempC := float64(tempMilliC) / 1000.0
	return tempC*9.0/5.0 + 32.0
}
