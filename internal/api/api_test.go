package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/temperature"
)

func setupTestDB(t *testing.T) *sql.DB {
	// Create in-memory SQLite database for testing
	database, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Apply schema
	schemaSQL := `
		CREATE TABLE system (
			id INTEGER PRIMARY KEY,
			system_mode TEXT NOT NULL,
			main_power_pin_number INTEGER NOT NULL,
			main_power_pin_active_high BOOLEAN NOT NULL,
			temp_sensor_bus_pin INTEGER,
			override_active BOOLEAN DEFAULT FALSE,
			prior_system_mode TEXT DEFAULT NULL
		);

		CREATE TABLE zones (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			setpoint REAL NOT NULL,
			mode TEXT NOT NULL,
			capabilities TEXT NOT NULL,
			sensor_id TEXT NOT NULL
		);

		CREATE TABLE sensors (
			id TEXT PRIMARY KEY,
			bus TEXT NOT NULL
		);

		CREATE TABLE devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			pin_number INTEGER NOT NULL,
			pin_active_high BOOLEAN NOT NULL,
			min_on INTEGER NOT NULL,
			min_off INTEGER NOT NULL,
			online BOOLEAN NOT NULL,
			last_changed TEXT,
			active_modes TEXT NOT NULL,
			device_type TEXT NOT NULL,
			role TEXT,
			zone_id TEXT,
			mode_pin_number INTEGER,
			mode_pin_active_high BOOLEAN,
			is_primary BOOLEAN DEFAULT FALSE,
			last_rotated TEXT,
			circ_pump_pin_number INTEGER,
			circ_pump_pin_active_high BOOLEAN
		);
	`
	_, err = database.Exec(schemaSQL)
	require.NoError(t, err)

	// Seed test data
	_, err = database.Exec(`INSERT INTO system (id, system_mode, main_power_pin_number, main_power_pin_active_high, temp_sensor_bus_pin, override_active, prior_system_mode) 
		VALUES (1, 'off', 25, TRUE, 4, FALSE, NULL)`)
	require.NoError(t, err)

	_, err = database.Exec(`INSERT INTO sensors (id, bus) VALUES ('test_sensor_1', '/sys/bus/w1/devices/test1/temperature')`)
	require.NoError(t, err)

	_, err = database.Exec(`INSERT INTO sensors (id, bus) VALUES ('test_sensor_2', '/sys/bus/w1/devices/test2/temperature')`)
	require.NoError(t, err)

	_, err = database.Exec(`INSERT INTO zones (id, label, setpoint, mode, capabilities, sensor_id) 
		VALUES ('zone1', 'Living Room', 72.0, 'off', '["heating","cooling"]', 'test_sensor_1')`)
	require.NoError(t, err)

	_, err = database.Exec(`INSERT INTO zones (id, label, setpoint, mode, capabilities, sensor_id) 
		VALUES ('zone2', 'Bedroom', 68.0, 'heating', '["heating"]', 'test_sensor_2')`)
	require.NoError(t, err)

	return database
}

func setupTestServer(t *testing.T) (*Server, *sql.DB) {
	database := setupTestDB(t)
	
	// Create a mock temperature service and config
	cfg := &config.Config{
		PollIntervalSeconds: 30,
		ZoneMinTemp:         50.0,
		ZoneMaxTemp:         95.0,
	}
	tempService := temperature.NewService(database, cfg.PollIntervalSeconds)
	
	server := NewServer(database, tempService, cfg)
	return server, database
}

func TestGetSystemMode(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/system/mode", nil)
	w := httptest.NewRecorder()

	server.handleSystemMode(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response SystemModeResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "off", response.Mode)
	// Buffer temp will be 0 since there's no temperature service data in test
	assert.Equal(t, 0.0, response.BufferTemp)
}

func TestSetSystemMode(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	tests := []struct {
		name           string
		mode           string
		expectedStatus int
		expectedMode   string
	}{
		{"valid heating mode", "heating", http.StatusOK, "heating"},
		{"valid cooling mode", "cooling", http.StatusOK, "cooling"},
		{"valid circulate mode", "circulate", http.StatusOK, "circulate"},
		{"valid off mode", "off", http.StatusOK, "off"},
		{"invalid mode", "invalid", http.StatusBadRequest, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := SystemModeRequest{Mode: tt.mode}
			reqJSON, _ := json.Marshal(reqBody)
			
			req := httptest.NewRequest(http.MethodPut, "/api/system/mode", bytes.NewBuffer(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleSystemMode(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				// Verify the mode was actually updated in the database
				actualMode, err := db.GetSystemMode(database)
				require.NoError(t, err)
				assert.Equal(t, model.SystemMode(tt.expectedMode), actualMode)
			}
		})
	}
}

func TestSetSystemModeInvalidJSON(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/system/mode", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleSystemMode(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	
	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid JSON payload", response.Error)
}

func TestGetZones(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/zones", nil)
	w := httptest.NewRecorder()

	server.handleZones(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response []ZoneResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response, 2)
	
	// Check first zone
	assert.Equal(t, "zone1", response[0].ID)
	assert.Equal(t, "Living Room", response[0].Label)
	assert.Equal(t, 72.0, response[0].Setpoint)
	assert.Equal(t, "off", response[0].Mode)
	
	// Check second zone
	assert.Equal(t, "zone2", response[1].ID)
	assert.Equal(t, "Bedroom", response[1].Label)
	assert.Equal(t, 68.0, response[1].Setpoint)
	assert.Equal(t, "heating", response[1].Mode)
}

func TestGetZone(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/zones/zone1", nil)
	w := httptest.NewRecorder()

	server.handleZoneOperations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	
	var response ZoneResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, "zone1", response.ID)
	assert.Equal(t, "Living Room", response.Label)
	assert.Equal(t, 72.0, response.Setpoint)
	assert.Equal(t, "off", response.Mode)
}

func TestGetZoneNotFound(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/zones/nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleZoneOperations(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	
	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Zone not found", response.Error)
}

func TestSetZoneMode(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	tests := []struct {
		name           string
		zoneID         string
		mode           string
		expectedStatus int
	}{
		{"valid mode for existing zone", "zone1", "heating", http.StatusOK},
		{"invalid mode", "zone1", "invalid", http.StatusBadRequest},
		{"nonexistent zone", "nonexistent", "heating", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := ZoneModeRequest{Mode: tt.mode}
			reqJSON, _ := json.Marshal(reqBody)
			
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/zones/%s/mode", tt.zoneID), bytes.NewBuffer(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleZoneOperations(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				// Verify the mode was actually updated in the database
				zone, err := db.GetZoneByID(database, tt.zoneID)
				require.NoError(t, err)
				assert.Equal(t, model.SystemMode(tt.mode), zone.Mode)
			}
		})
	}
}

func TestSetZoneSetpoint(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	tests := []struct {
		name           string
		zoneID         string
		setpoint       float64
		expectedStatus int
	}{
		{"valid setpoint for existing zone", "zone1", 74.5, http.StatusOK},
		{"setpoint too low", "zone1", 40.0, http.StatusBadRequest},
		{"setpoint too high", "zone1", 100.0, http.StatusBadRequest},
		{"nonexistent zone", "nonexistent", 72.0, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := ZoneSetpointRequest{Setpoint: tt.setpoint}
			reqJSON, _ := json.Marshal(reqBody)
			
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/zones/%s/setpoint", tt.zoneID), bytes.NewBuffer(reqJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleZoneOperations(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				// Verify the setpoint was actually updated in the database
				zone, err := db.GetZoneByID(database, tt.zoneID)
				require.NoError(t, err)
				assert.Equal(t, tt.setpoint, zone.Setpoint)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST to system mode", http.MethodPost, "/api/system/mode"},
		{"DELETE to system mode", http.MethodDelete, "/api/system/mode"},
		{"POST to zones", http.MethodPost, "/api/zones"},
		{"DELETE to zone", http.MethodDelete, "/api/zones/zone1"},
		{"GET to zone mode", http.MethodGet, "/api/zones/zone1/mode"},
		{"POST to zone setpoint", http.MethodPost, "/api/zones/zone1/setpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			if tt.path == "/api/system/mode" {
				server.handleSystemMode(w, req)
			} else if tt.path == "/api/zones" {
				server.handleZones(w, req)
			} else {
				server.handleZoneOperations(w, req)
			}

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

func TestInvalidPaths(t *testing.T) {
	server, database := setupTestServer(t)
	defer database.Close()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{"zone without ID", "/api/zones/", http.StatusNotFound},
		{"unknown zone operation", "/api/zones/zone1/unknown", http.StatusNotFound},
		{"too many path segments", "/api/zones/zone1/mode/extra", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, tt.path, nil)
			w := httptest.NewRecorder()

			server.handleZoneOperations(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestIsValidSystemMode(t *testing.T) {
	tests := []struct {
		mode  model.SystemMode
		valid bool
	}{
		{model.ModeOff, true},
		{model.ModeHeating, true},
		{model.ModeCooling, true},
		{model.ModeCirculate, true},
		{model.SystemMode("invalid"), false},
		{model.SystemMode(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidSystemMode(tt.mode))
		})
	}
}