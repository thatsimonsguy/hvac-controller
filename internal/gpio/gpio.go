package gpio

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/pinctrl"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

var safeMode bool

func ValidateInitialPinStates(dbConn *sql.DB) error {
	type pinWithMeta struct {
		Name       string
		Pin        model.GPIOPin
		ShouldBeOn bool
	}

	var checks []pinWithMeta

	heatPumps, err := db.GetHeatPumps(dbConn)
	if err != nil {
		return err
	}
	systemMode, err := db.GetSystemMode(dbConn)
	if err != nil {
		return err
	}

	for _, hp := range heatPumps {
		checks = append(checks, pinWithMeta{
			Name:       hp.Name,
			Pin:        hp.Pin,
			ShouldBeOn: false,
		})

		modeActive := contains(hp.Device.ActiveModes, string(systemMode)) &&
			systemMode == model.ModeCooling && hp.Device.Online

		checks = append(checks, pinWithMeta{
			Name:       hp.Name + ".mode_pin",
			Pin:        hp.ModePin,
			ShouldBeOn: modeActive,
		})
	}

	airHandlers, err := db.GetAirHandlers(dbConn)
	if err != nil {
		return err
	}
	for _, ah := range airHandlers {
		checks = append(checks,
			pinWithMeta{ah.Name, ah.Pin, false},
			pinWithMeta{ah.Name + ".circ_pump", ah.CircPumpPin, false},
		)
	}
	boilers, err := db.GetBoilers(dbConn)
	if err != nil {
		return err
	}
	for _, b := range boilers {
		checks = append(checks, pinWithMeta{b.Name, b.Pin, false})
	}
	radiantLoops, err := db.GetRadiantLoops(dbConn)
	if err != nil {
		return err
	}
	for _, rf := range radiantLoops {
		checks = append(checks, pinWithMeta{rf.Name, rf.Pin, false})
	}

	mainPower, err := db.GetMainPowerPin(dbConn)
	if err != nil {
		return err
	}
	checks = append(checks, pinWithMeta{"main_power", mainPower, false})

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

var Activate = func(pin model.GPIOPin) {
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

var Deactivate = func(pin model.GPIOPin) {
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

var CurrentlyActive = func(pin model.GPIOPin) bool {
	level := Read(pin)
	return pin.ActiveHigh == level
}

func ReadSensorTempWithRetries(sensorPath string, retries int) float64 {
	temp, err := ReadSensorTemp(sensorPath)
	if retries < 0 {
		shutdown.ShutdownWithError(err, "max sensor retries reached")
	}
	if err != nil && retries > 0 {
		time.Sleep(2 * time.Second)
		return ReadSensorTempWithRetries(sensorPath, retries-1)
	}
	return temp
}

var ReadSensorTemp = func(sensorPath string) (float64, error) {
	file := filepath.Join(sensorPath, "w1_slave")
	data, err := os.ReadFile(file)
	if err != nil {
		log.Error().Err(err).Msg("failed to read sensor data")
		return 0.0, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 || !strings.Contains(lines[1], "t=") {
		log.Error().Err(err).Msg("temperature data missing or malformed")
		return 0.0, err
	}

	parts := strings.Split(lines[1], "t=")
	if len(parts) != 2 {
		log.Error().Err(err).Msg("could not parse temperature line")
		return 0.0, err
	}

	tempMilliC, err := strconv.Atoi(parts[1])
	if err != nil {
		log.Error().Err(err).Msg("failed to convert temperature to int")
		return 0.0, err
	}

	// Celsius to Fahrenheit: F = C × 9/5 + 32
	tempC := float64(tempMilliC) / 1000.0
	return tempC*9.0/5.0 + 32.0, nil
}
