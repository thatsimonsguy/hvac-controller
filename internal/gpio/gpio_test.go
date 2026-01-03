package gpio

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thatsimonsguy/hvac-controller/internal/model"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create minimal schema for testing (matches actual schema.sql)
	schema := `
		CREATE TABLE system (
			id INTEGER PRIMARY KEY CHECK(id=1),
			system_mode TEXT NOT NULL,
			main_power_pin_number INTEGER,
			main_power_pin_active_high BOOLEAN
		);
		INSERT INTO system (id, system_mode, main_power_pin_number, main_power_pin_active_high)
		VALUES (1, 'heating', 25, 1);

		CREATE TABLE devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT,
			pin_number INTEGER,
			pin_active_high BOOLEAN,
			min_on INTEGER,
			min_off INTEGER,
			online BOOLEAN,
			last_changed TEXT,
			active_modes TEXT,
			device_type TEXT,
			role TEXT NOT NULL DEFAULT 'source',
			zone_id TEXT,
			circ_pump_pin_number INTEGER,
			circ_pump_pin_active_high BOOLEAN,
			mode_pin_number INTEGER,
			mode_pin_active_high BOOLEAN,
			is_primary BOOLEAN,
			last_rotated TEXT
		);

		-- Heat pump
		INSERT INTO devices (name, pin_number, pin_active_high, mode_pin_number, mode_pin_active_high,
			online, min_on, min_off, active_modes, device_type, role, is_primary, last_rotated)
		VALUES ('heat_pump_A', 23, 0, 18, 1, 1, 10, 5, '["heating","cooling"]', 'heat_pump', 'source', 1, '2024-01-01T00:00:00Z');

		-- Air handler
		INSERT INTO devices (name, pin_number, pin_active_high, circ_pump_pin_number, circ_pump_pin_active_high,
			zone_id, online, min_on, min_off, active_modes, device_type, role)
		VALUES ('main_floor_ah', 5, 0, 6, 0, 'main_floor', 1, 3, 1, '["heating","cooling","fan"]', 'air_handler', 'distributor');

		-- Boiler
		INSERT INTO devices (name, pin_number, pin_active_high, online, min_on, min_off, active_modes, device_type, role)
		VALUES ('boiler', 22, 0, 1, 2, 5, '["heating"]', 'boiler', 'source');

		-- Radiant floor loop
		INSERT INTO devices (name, pin_number, pin_active_high, zone_id, online, min_on, min_off, active_modes, device_type, role)
		VALUES ('garage_loop', 17, 0, 'garage', 1, 5, 3, '["heating"]', 'radiant_floor', 'distributor');
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	return db
}

func TestValidateInitialPinStates_AllSafe(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Mock pin reading to return safe states (all inactive)
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		// For active-low pins (most devices): HIGH (true) = inactive = safe
		// For active-high pins (18, 25): LOW (false) = inactive = safe
		if pin == 18 || pin == 25 {
			return false, nil // Active-high pins: LOW = inactive
		}
		return true, nil // Active-low pins: HIGH = inactive
	}

	// Mock activate/deactivate to track calls
	activateCalls := 0
	deactivateCalls := 0

	originalActivate := Activate
	originalDeactivate := Deactivate
	defer func() {
		Activate = originalActivate
		Deactivate = originalDeactivate
	}()

	Activate = func(pin model.GPIOPin) {
		activateCalls++
	}
	Deactivate = func(pin model.GPIOPin) {
		deactivateCalls++
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 0, activateCalls, "Should not activate any pins when all are safe")
	assert.Equal(t, 0, deactivateCalls, "Should not deactivate any pins when all are safe")
}

func TestValidateInitialPinStates_CorrectionNeeded(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Mock pin reading to return unsafe states for specific pins
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		switch pin {
		case 23:
			return false, nil // heat_pump_A pin (active-low, FALSE = active = unsafe)
		case 5:
			return false, nil // main_floor_ah pin (active-low, FALSE = active = unsafe)
		case 18, 25:
			return false, nil // Active-high pins: LOW = inactive = safe
		default:
			return true, nil // All other active-low pins safe (HIGH = inactive)
		}
	}

	// Mock activate/deactivate to track which pins were corrected
	correctedPins := make(map[int]string)

	originalActivate := Activate
	originalDeactivate := Deactivate
	defer func() {
		Activate = originalActivate
		Deactivate = originalDeactivate
	}()

	Activate = func(pin model.GPIOPin) {
		correctedPins[pin.Number] = "activate"
	}
	Deactivate = func(pin model.GPIOPin) {
		correctedPins[pin.Number] = "deactivate"
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 2, len(correctedPins), "Should correct 2 unsafe pins")
	assert.Equal(t, "deactivate", correctedPins[23], "Pin 23 should be deactivated")
	assert.Equal(t, "deactivate", correctedPins[5], "Pin 5 should be deactivated")
}

func TestValidateInitialPinStates_ActiveHighCorrection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Mock pin reading - main_power (pin 25) is active-high and should be OFF at startup
	// If it reads HIGH (true), that means it's active, which is unsafe
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		switch pin {
		case 25:
			return true, nil // HIGH = active for active-high pin = unsafe
		case 18:
			return false, nil // Mode pin (active-high): LOW = inactive = safe
		default:
			return true, nil // All other active-low pins: HIGH = inactive = safe
		}
	}

	// Mock deactivate to track the call
	var deactivatedPin *model.GPIOPin

	originalDeactivate := Deactivate
	originalActivate := Activate
	defer func() {
		Deactivate = originalDeactivate
		Activate = originalActivate
	}()

	Activate = func(pin model.GPIOPin) {
		t.Errorf("Should not activate any pins")
	}
	Deactivate = func(pin model.GPIOPin) {
		if pin.Number == 25 {
			deactivatedPin = &pin
		}
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, deactivatedPin, "Pin 25 should be deactivated")
	assert.True(t, deactivatedPin.ActiveHigh, "Pin should be active-high")
}

func TestValidateInitialPinStates_ReadError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Mock pin reading to return an error
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		if pin == 23 {
			return false, fmt.Errorf("simulated pin read error")
		}
		return false, nil
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read pin level")
	assert.Contains(t, err.Error(), "simulated pin read error")
}

func TestValidateInitialPinStates_MultiplePinsNeedCorrection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Mock all device pins to be in wrong state
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		// Return FALSE (low) for all pins
		// For active-low devices (most of them), FALSE means they're ACTIVE (unsafe)
		// For active-high devices (mode_pin 18, main_power 25), FALSE means they're INACTIVE (safe)
		return false, nil
	}

	// Track corrections
	deactivateCount := 0

	originalActivate := Activate
	originalDeactivate := Deactivate
	defer func() {
		Activate = originalActivate
		Deactivate = originalDeactivate
	}()

	Activate = func(pin model.GPIOPin) {
		t.Errorf("Should not activate any pins in this test")
	}
	Deactivate = func(pin model.GPIOPin) {
		deactivateCount++
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	// Active-low pins that need correction: heat_pump pin (23), air_handler pin (5),
	// air_handler circ_pump (6), boiler (22), radiant_loop (17) = 5 pins
	assert.Equal(t, 5, deactivateCount, "Should deactivate 5 active-low pins that are reading LOW")
}

func TestValidateInitialPinStates_CoolingModeWithModePin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Set system to cooling mode
	_, err := db.Exec("UPDATE system SET system_mode = 'cooling'")
	assert.NoError(t, err)

	// Mock pin reading to return safe states (all inactive)
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		// Active-high pins (18, 25): LOW = inactive
		// Active-low pins: HIGH = inactive
		if pin == 18 || pin == 25 {
			return false, nil
		}
		return true, nil
	}

	// Track activate calls
	activateCalls := make(map[int]string)
	deactivateCalls := make(map[int]string)

	originalActivate := Activate
	originalDeactivate := Deactivate
	defer func() {
		Activate = originalActivate
		Deactivate = originalDeactivate
	}()

	Activate = func(pin model.GPIOPin) {
		activateCalls[pin.Number] = "activate"
	}
	Deactivate = func(pin model.GPIOPin) {
		deactivateCalls[pin.Number] = "deactivate"
	}

	// Run validation
	err = ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	// Mode pin (18) should be activated because:
	// 1. System mode is cooling
	// 2. Heat pump active_modes includes cooling
	// 3. Heat pump is online
	assert.Contains(t, activateCalls, 18, "Mode pin should be activated in cooling mode")
	// Main relay pin should remain off
	assert.NotContains(t, activateCalls, 23, "Main heat pump pin should stay off")
}

func TestValidateInitialPinStates_HeatingModeNoModePin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// System is already in heating mode (default from setupTestDB)

	// Mock pin reading - mode pin is HIGH (active for active-high pin) but shouldn't be
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		switch pin {
		case 18:
			return true, nil // Mode pin (active-high): HIGH = active = unsafe in heating mode
		case 25:
			return false, nil // Main power (active-high): LOW = inactive = safe
		default:
			return true, nil // All other active-low pins: HIGH = inactive = safe
		}
	}

	// Track deactivate calls
	deactivatedPins := make(map[int]bool)

	originalActivate := Activate
	originalDeactivate := Deactivate
	defer func() {
		Activate = originalActivate
		Deactivate = originalDeactivate
	}()

	Activate = func(pin model.GPIOPin) {
		t.Errorf("Should not activate any pins")
	}
	Deactivate = func(pin model.GPIOPin) {
		deactivatedPins[pin.Number] = true
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.NoError(t, err)
	assert.True(t, deactivatedPins[18], "Mode pin should be deactivated in heating mode")
}

func TestValidateInitialPinStates_DatabaseError(t *testing.T) {
	db := setupTestDB(t)
	db.Close() // Close DB to trigger errors

	// Mock pin reading (won't be called due to DB error)
	originalReadPinLevel := readPinLevel
	defer func() { readPinLevel = originalReadPinLevel }()

	readPinLevel = func(pin int) (bool, error) {
		t.Error("Should not reach pin reading due to database error")
		return false, nil
	}

	// Run validation
	err := ValidateInitialPinStates(db)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sql: database is closed")
}
