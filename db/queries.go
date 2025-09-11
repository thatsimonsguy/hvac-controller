package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

// GetSystemMode retrieves the current system mode.
func GetSystemMode(db *sql.DB) (model.SystemMode, error) {
	var mode string
	err := db.QueryRow(`SELECT system_mode FROM system WHERE id = 1`).Scan(&mode)
	if err != nil {
		return model.ModeOff, fmt.Errorf("failed to get system mode: %w", err)
	}
	return model.SystemMode(mode), nil
}

// GetAllZones retrieves all zones from the database.
func GetAllZones(db *sql.DB) ([]model.Zone, error) {
	rows, err := db.Query(`SELECT id, label, setpoint, mode, capabilities, sensor_id FROM zones`)
	if err != nil {
		return nil, fmt.Errorf("failed to query zones: %w", err)
	}
	defer rows.Close()

	var zones []model.Zone
	for rows.Next() {
		var z model.Zone
		var capabilities string
		err = rows.Scan(&z.ID, &z.Label, &z.Setpoint, &z.Mode, &capabilities, &z.Sensor.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan zone: %w", err)
		}
		json.Unmarshal([]byte(capabilities), &z.Capabilities)
		zones = append(zones, z)
	}
	return zones, nil
}

// GetZoneByID retrieves a specific zone by its ID.
func GetZoneByID(db *sql.DB, id string) (*model.Zone, error) {
	var z model.Zone
	var capabilities string
	err := db.QueryRow(`SELECT id, label, setpoint, mode, capabilities, sensor_id FROM zones WHERE id = ?`, id).Scan(&z.ID, &z.Label, &z.Setpoint, &z.Mode, &capabilities, &z.Sensor.ID)
	if err != nil {
		return &z, fmt.Errorf("failed to get zone %s: %w", id, err)
	}
	json.Unmarshal([]byte(capabilities), &z.Capabilities)
	return &z, nil
}

// Sensor queries
func GetAllSensors(db *sql.DB) ([]model.Sensor, error) {
	rows, err := db.Query(`SELECT id, bus FROM sensors`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sensors: %w", err)
	}
	defer rows.Close()

	var sensors []model.Sensor
	for rows.Next() {
		var s model.Sensor
		err = rows.Scan(&s.ID, &s.Bus)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sensor: %w", err)
		}
		sensors = append(sensors, s)
	}
	return sensors, nil
}

func GetSensorByID(db *sql.DB, id string) (*model.Sensor, error) {
	var s model.Sensor
	err := db.QueryRow(`SELECT id, bus FROM sensors WHERE id = ?`, id).Scan(&s.ID, &s.Bus)
	if err != nil {
		return &s, fmt.Errorf("failed to get sensor %s: %w", id, err)
	}
	return &s, nil
}

// GetHeatPumps retrieves all heat pumps from the database.
func GetHeatPumps(db *sql.DB) ([]model.HeatPump, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, mode_pin_number, mode_pin_active_high, is_primary, last_rotated FROM devices WHERE device_type = 'heat_pump'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query heat pumps: %w", err)
	}
	defer rows.Close()

	var heatPumps []model.HeatPump
	for rows.Next() {
		var d model.Device
		var modePinNumber int
		var modePinActiveHigh bool
		var isPrimary bool
		var lastRotatedStr string
		var activeModes string
		var lastChanged sql.NullString

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes, &modePinNumber, &modePinActiveHigh, &isPrimary, &lastRotatedStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan heat pump: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		lastRotated, _ := time.Parse(time.RFC3339, lastRotatedStr)
		hp := model.HeatPump{
			Device:      d,
			ModePin:     model.GPIOPin{Number: modePinNumber, ActiveHigh: modePinActiveHigh},
			IsPrimary:   isPrimary,
			LastRotated: lastRotated,
		}
		heatPumps = append(heatPumps, hp)
	}
	return heatPumps, nil
}

// GetBoilers retrieves all boilers from the database.
func GetBoilers(db *sql.DB) ([]model.Boiler, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes FROM devices WHERE device_type = 'boiler'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query boilers: %w", err)
	}
	defer rows.Close()

	var boilers []model.Boiler
	for rows.Next() {
		var d model.Device
		var activeModes string
		var lastChanged sql.NullString

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes)
		if err != nil {
			return nil, fmt.Errorf("failed to scan boiler: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		boilers = append(boilers, model.Boiler{Device: d})
	}
	return boilers, nil
}

// GetAirHandlers retrieves all air handlers from the database.
func GetAirHandlers(db *sql.DB) ([]model.AirHandler, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, zone_id, circ_pump_pin_number, circ_pump_pin_active_high FROM devices WHERE device_type = 'air_handler'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query air handlers: %w", err)
	}
	defer rows.Close()

	var airHandlers []model.AirHandler
	for rows.Next() {
		var d model.Device
		var activeModes string
		var lastChanged sql.NullString
		var zoneID string
		var circPumpPinNumber int
		var circPumpPinActiveHigh bool

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes, &zoneID, &circPumpPinNumber, &circPumpPinActiveHigh)
		if err != nil {
			return nil, fmt.Errorf("failed to scan air handler: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		ah := model.AirHandler{
			Device:      d,
			Zone:        &model.Zone{ID: zoneID},
			CircPumpPin: model.GPIOPin{Number: circPumpPinNumber, ActiveHigh: circPumpPinActiveHigh},
		}
		airHandlers = append(airHandlers, ah)
	}
	return airHandlers, nil
}

// GetRadiantLoops retrieves all radiant loops from the database.
func GetRadiantLoops(db *sql.DB) ([]model.RadiantFloorLoop, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, zone_id FROM devices WHERE device_type = 'radiant_floor'`)
	if err != nil {
		return nil, fmt.Errorf("failed to query radiant loops: %w", err)
	}
	defer rows.Close()

	var radiantLoops []model.RadiantFloorLoop
	for rows.Next() {
		var d model.Device
		var activeModes string
		var lastChanged sql.NullString
		var zoneID string

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes, &zoneID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan radiant loop: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		rf := model.RadiantFloorLoop{
			Device: d,
			Zone:   &model.Zone{ID: zoneID},
		}
		radiantLoops = append(radiantLoops, rf)
	}
	return radiantLoops, nil
}

// GetAirHandlerByID retrieves a single air handler by device name (ID).
func GetAirHandlerByID(db *sql.DB, id string) (*model.AirHandler, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, zone_id, circ_pump_pin_number, circ_pump_pin_active_high FROM devices WHERE device_type = 'air_handler' AND zone_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query air handler %s: %w", id, err)
	}
	defer rows.Close()

	var airHandlers []model.AirHandler
	for rows.Next() {
		var d model.Device
		var activeModes string
		var lastChanged sql.NullString
		var zoneID string
		var circPumpPinNumber int
		var circPumpPinActiveHigh bool

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes, &zoneID, &circPumpPinNumber, &circPumpPinActiveHigh)
		if err != nil {
			return nil, fmt.Errorf("failed to scan air handler: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		ah := model.AirHandler{
			Device:      d,
			Zone:        &model.Zone{ID: zoneID},
			CircPumpPin: model.GPIOPin{Number: circPumpPinNumber, ActiveHigh: circPumpPinActiveHigh},
		}
		airHandlers = append(airHandlers, ah)
	}

	if len(airHandlers) == 0 {
		return nil, nil // Valid case for garage zone: no air handler found
	}
	if len(airHandlers) > 1 {
		return &airHandlers[0], fmt.Errorf("warning: multiple air handlers found for ID %s", id)
	}
	return &airHandlers[0], nil
}

// GetRadiantLoopByID retrieves a single radiant floor loop by device name (ID).
func GetRadiantLoopByID(db *sql.DB, id string) (*model.RadiantFloorLoop, error) {
	rows, err := db.Query(`SELECT name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, zone_id FROM devices WHERE device_type = 'radiant_floor' AND zone_id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query radiant loop %s: %w", id, err)
	}
	defer rows.Close()

	var radiantLoops []model.RadiantFloorLoop
	for rows.Next() {
		var d model.Device
		var activeModes string
		var lastChanged sql.NullString
		var zoneID string

		err = rows.Scan(&d.Name, &d.Pin.Number, &d.Pin.ActiveHigh, &d.MinOn, &d.MinOff, &d.Online, &lastChanged, &activeModes, &zoneID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan radiant loop: %w", err)
		}
		json.Unmarshal([]byte(activeModes), &d.ActiveModes)
		if lastChanged.Valid {
			d.LastChanged, _ = time.Parse(time.RFC3339, lastChanged.String)
		}
		rf := model.RadiantFloorLoop{
			Device: d,
			Zone:   &model.Zone{ID: zoneID},
		}
		radiantLoops = append(radiantLoops, rf)
	}

	if len(radiantLoops) == 0 {
		return nil, nil // Valid case for main floor: no radiant loop found
	}
	if len(radiantLoops) > 1 {
		return &radiantLoops[0], fmt.Errorf("warning: multiple radiant loops found for ID %s", id)
	}
	return &radiantLoops[0], nil
}

// GetMainPowerPin retrieves the main power pin configuration as a GPIOPin.
func GetMainPowerPin(db *sql.DB) (model.GPIOPin, error) {
	var pinNumber int
	var activeHigh bool
	err := db.QueryRow(`SELECT main_power_pin_number, main_power_pin_active_high FROM system WHERE id = 1`).Scan(&pinNumber, &activeHigh)
	if err != nil {
		return model.GPIOPin{}, fmt.Errorf("failed to get MainPowerPin: %w", err)
	}
	return model.GPIOPin{
		Number:     pinNumber,
		ActiveHigh: activeHigh,
	}, nil
}

// GetSystemOverride retrieves the current override state.
func GetSystemOverride(db *sql.DB) (bool, error) {
	var overrideActive bool
	err := db.QueryRow(`SELECT override_active FROM system WHERE id = 1`).Scan(&overrideActive)
	if err != nil {
		return false, fmt.Errorf("failed to get system override state: %w", err)
	}
	return overrideActive, nil
}

