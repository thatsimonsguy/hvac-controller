package buffercontroller_test

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/controllers/buffercontroller"
	"github.com/thatsimonsguy/hvac-controller/internal/device"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

func setupTestDB(t *testing.T) *sql.DB {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	schema, err := os.ReadFile("../../../db/schema.sql")
	if err != nil {
		t.Fatalf("failed to read schema.sql: %v", err)
	}
	_, err = conn.Exec(string(schema))
	if err != nil {
		t.Fatalf("failed to apply schema: %v", err)
	}
	return conn
}

func insertTestHeatPump(t *testing.T, db *sql.DB, name string, online bool, isPrimary bool, lastChanged time.Time, lastRotated time.Time) {
	_, err := db.Exec(`INSERT INTO devices 
	(name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role, mode_pin_number, mode_pin_active_high, is_primary, last_rotated) 
	VALUES (?, 1, 0, 1, 1, ?, ?, '["heating", "cooling"]', 'heat_pump', 'source', 1, 1, ?, ?)`,
		name, online, lastChanged, isPrimary, lastRotated)
	assert.NoError(t, err)
}

func insertTestBoiler(t *testing.T, db *sql.DB, name string, online bool, lastChanged time.Time) {
	_, err := db.Exec(`INSERT INTO devices 
	(name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role)
	VALUES (?, 1, 0, 1, 1, ?, ?, '["heating"]', 'boiler', 'source')`,
		name, online, lastChanged)
	assert.NoError(t, err)
}

func setTestSystemMode(t *testing.T, db *sql.DB, mode string) {
	// First, check if a system mode record exists
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM system`).Scan(&count)
	assert.NoError(t, err)

	if count == 0 {
		// Insert a new system mode record
		_, err := db.Exec(`INSERT INTO system (id, system_mode) VALUES (1, ?)`, mode)
		assert.NoError(t, err)
	} else {
		// Update the existing system mode record
		_, err := db.Exec(`UPDATE system SET system_mode = ? WHERE id = 1`, mode)
		assert.NoError(t, err)
	}
}

func OverrideEnvCfg(newCfg *config.Config) (restore func()) {
	// Save the original config
	originalCfg := env.Cfg

	// Override with the new config
	env.Cfg = newCfg

	// Return a restore function
	return func() {
		env.Cfg = originalCfg
	}
}

func TestGetHeatSourcesHappyPath(t *testing.T) {
	dbConn := setupTestDB(t)
	now := time.Now()
	insertTestHeatPump(t, dbConn, "hp1", true, true, now, now)
	insertTestHeatPump(t, dbConn, "hp2", true, false, now, now)
	insertTestBoiler(t, dbConn, "boiler1", true, now)

	sources := buffercontroller.GetHeatSources(dbConn)
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp1", sources.Primary.Name)
	assert.NotNil(t, sources.Secondary)
	assert.Equal(t, "hp2", sources.Secondary.Name)
	assert.NotNil(t, sources.Tertiary)
	assert.Equal(t, "boiler1", sources.Tertiary.Name)
}
func TestGetHeatSourcesMultiplePrimaries(t *testing.T) {
	dbConn := setupTestDB(t)
	now := time.Now()
	insertTestHeatPump(t, dbConn, "hp1", true, true, now, now)
	insertTestHeatPump(t, dbConn, "hp2", true, true, now, now)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic due to multiple primaries, but did not panic")
		}
	}()
	buffercontroller.GetHeatSources(dbConn)
}

func TestGetHeatSourcesNoPrimary(t *testing.T) {
	dbConn := setupTestDB(t)
	now := time.Now()
	insertTestHeatPump(t, dbConn, "hp1", true, false, now, now)
	insertTestHeatPump(t, dbConn, "hp2", true, false, now, now)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic due to no primary, but did not panic")
		}
	}()
	buffercontroller.GetHeatSources(dbConn)
}

func TestGetHeatSourcesQueryError(t *testing.T) {
	dbConn := setupTestDB(t)
	dbConn.Close() // Force an error on query
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic due to query error, but did not panic")
		}
	}()
	buffercontroller.GetHeatSources(dbConn)
}

// MockHeatSourcesProvider implements HeatSourcesProvider for testing
type MockHeatSourcesProvider struct {
	HeatSources buffercontroller.HeatSources
}

func (m *MockHeatSourcesProvider) GetHeatSources(_ *sql.DB) buffercontroller.HeatSources {
	return m.HeatSources
}

func TestRefreshSourcesHappyPath(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: true},
				IsPrimary:   true,
				LastRotated: time.Now(),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: true},
				IsPrimary:   false,
				LastRotated: time.Now(),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "heating")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp1", sources.Primary.Name)
	assert.NotNil(t, sources.Secondary)
	assert.Equal(t, "hp2", sources.Secondary.Name)
	assert.NotNil(t, sources.Tertiary)
	assert.Equal(t, "boiler1", sources.Tertiary.Name)
}

func TestRefreshSourcesRotationHeating(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: true},
				IsPrimary:   true,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: true},
				IsPrimary:   false,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "heating")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp2", sources.Primary.Name)
	assert.NotNil(t, sources.Secondary)
	assert.Equal(t, "hp1", sources.Secondary.Name)
	assert.NotNil(t, sources.Tertiary)
	assert.Equal(t, "boiler1", sources.Tertiary.Name)
}

func TestRefreshSourcesRotationCooling(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: true},
				IsPrimary:   true,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: true},
				IsPrimary:   false,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "cooling")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp2", sources.Primary.Name)
	assert.NotNil(t, sources.Secondary)
	assert.Equal(t, "hp1", sources.Secondary.Name)
	assert.Nil(t, sources.Tertiary)
}

func TestRefreshSourcesHeatingNoHeatPumps(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: false},
				IsPrimary:   true,
				LastRotated: time.Now(),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: false},
				IsPrimary:   false,
				LastRotated: time.Now(),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "heating")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.Nil(t, sources.Primary)
	assert.Nil(t, sources.Secondary)
	assert.NotNil(t, sources.Tertiary)
	assert.Equal(t, "boiler1", sources.Tertiary.Name)
}

func TestRefreshSourcesCoolingNoHeatPumps(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: false},
				IsPrimary:   true,
				LastRotated: time.Now(),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: false},
				IsPrimary:   false,
				LastRotated: time.Now(),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "cooling")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	assert.Nil(t, sources.Primary)
	assert.Nil(t, sources.Secondary)
	assert.Nil(t, sources.Tertiary)
}

func TestRefreshSourcesOnePumpRotationCooling(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: false},
				IsPrimary:   true,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: true},
				IsPrimary:   false,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "cooling")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp2", sources.Primary.Name)
	assert.Nil(t, sources.Secondary)
	assert.Nil(t, sources.Tertiary)
}

func TestRefreshSourcesOnePumpRotationHeating(t *testing.T) {
	// Setup a dummy DB connection (in-memory, unused but needed for the signature)
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	// Override env.Cfg for this test and ensure cleanup
	restore := OverrideEnvCfg(&config.Config{
		DeviceConfig: config.DeviceConfig{
			HeatPumps: config.HeatPumpGroup{
				DeviceProfile: config.DeviceProfile{
					MinTimeOff: 1, // minutes
				},
			},
		},
		HeatingThreshold:    60.0,
		CoolingThreshold:    75.0,
		Spread:              2.0,
		SecondaryMargin:     1.0,
		TertiaryMargin:      2.0,
		RoleRotationMinutes: 10,
		PollIntervalSeconds: 10,
	})
	defer restore()

	// Create a mock provider that returns fixed heat sources
	mockProvider := &MockHeatSourcesProvider{
		HeatSources: buffercontroller.HeatSources{
			Primary: &model.HeatPump{
				Device:      model.Device{Name: "hp1", Online: false},
				IsPrimary:   true,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Secondary: &model.HeatPump{
				Device:      model.Device{Name: "hp2", Online: true},
				IsPrimary:   false,
				LastRotated: time.Now().Add(-1 * time.Hour),
			},
			Tertiary: &model.Boiler{
				Device: model.Device{Name: "boiler1", Online: true},
			},
		},
	}

	setTestSystemMode(t, dbConn, "heating")

	// Create the SourceRefresher with the mock provider
	refresher := buffercontroller.SourceRefresher{
		Provider: mockProvider,
	}

	// Call RefreshSources
	sources := refresher.RefreshSources(dbConn)

	// Assertions
	assert.NotNil(t, sources.Primary)
	assert.Equal(t, "hp2", sources.Primary.Name)
	assert.Nil(t, sources.Secondary)
	assert.NotNil(t, sources.Tertiary)
	assert.Equal(t, "boiler1", sources.Tertiary.Name)
}

func TestShouldBeOn(t *testing.T) {
	tests := []struct {
		name      string
		bt        float64
		threshold float64
		mode      model.SystemMode
		expected  bool
	}{
		{"Heating: below threshold", 45.0, 50.0, model.ModeHeating, true},
		{"Heating: at threshold", 50.0, 50.0, model.ModeHeating, false},
		{"Heating: above threshold", 55.0, 50.0, model.ModeHeating, false},
		{"Cooling: below threshold", 65.0, 70.0, model.ModeCooling, false},
		{"Cooling: at threshold", 70.0, 70.0, model.ModeCooling, false},
		{"Cooling: above threshold", 75.0, 70.0, model.ModeCooling, true},
		{"Invalid mode", 70.0, 70.0, "invalid_mode", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buffercontroller.ShouldBeOn(tt.bt, tt.threshold, tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetThreshold(t *testing.T) {
	// Setup fake config
	env.Cfg = &config.Config{
		HeatingThreshold: 50.0,
		CoolingThreshold: 70.0,
		SecondaryMargin:  2.0,
		TertiaryMargin:   5.0,
		Spread:           1.0,
	}

	tests := []struct {
		name     string
		role     string
		mode     model.SystemMode
		active   bool
		expected float64
	}{
		{"primary heating inactive", "primary", model.ModeHeating, false, 50.0},
		{"primary heating active", "primary", model.ModeHeating, true, 50.0 + env.Cfg.Spread},

		{"secondary heating inactive", "secondary", model.ModeHeating, false, 48.0},
		{"secondary heating active", "secondary", model.ModeHeating, true, 48.0 + env.Cfg.Spread},

		{"tertiary heating inactive", "tertiary", model.ModeHeating, false, 45.0},
		{"tertiary heating active", "tertiary", model.ModeHeating, true, 45.0 + env.Cfg.Spread},

		{"primary cooling inactive", "primary", model.ModeCooling, false, 70.0},
		{"primary cooling active", "primary", model.ModeCooling, true, 70.0 - env.Cfg.Spread},

		{"secondary cooling inactive", "secondary", model.ModeCooling, false, 72.0},
		{"secondary cooling active", "secondary", model.ModeCooling, true, 72.0 - env.Cfg.Spread},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := buffercontroller.GetThreshold(tt.role, tt.mode, tt.active)
			assert.Equal(t, tt.expected, actual)
		})
	}

}

func TestGetThreshold_ShutdownCases(t *testing.T) {
	// Override ExitFunc to avoid killing the test process
	shutdown.ExitFunc = func(code int) {
		panic(errors.New("shutdown called"))
	}
	defer func() { shutdown.ExitFunc = os.Exit }()

	tests := []struct {
		name   string
		role   string
		mode   model.SystemMode
		active bool
	}{
		{"invalid role", "invalid", model.ModeHeating, true},
		{"tertiary in cooling mode", "tertiary", model.ModeCooling, true},
		{"unknown mode", "primary", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic on shutdown, got none")
				}
			}()

			_ = buffercontroller.GetThreshold(tt.role, tt.mode, tt.active) // should panic
		})
	}
}

func TestEvaluateToggleSource(t *testing.T) {
	// Override CanToggle to control it
	originalCanToggle := device.CanToggle
	defer func() { device.CanToggle = originalCanToggle }()

	// Set config so thresholds are predictable
	env.Cfg = &config.Config{
		HeatingThreshold: 50.0,
		CoolingThreshold: 70.0,
		SecondaryMargin:  2.0,
		TertiaryMargin:   5.0,
	}

	tests := []struct {
		name       string
		role       string
		mode       model.SystemMode
		bt         float64
		active     bool
		canToggle  bool
		expectFlip bool
	}{
		// HEATING CASES
		{"Heating: should be on, but already on", "primary", model.ModeHeating, 40, true, true, false},
		{"Heating: should be on, currently off, can toggle", "primary", model.ModeHeating, 40, false, true, true},
		{"Heating: should be on, currently off, CANNOT toggle", "primary", model.ModeHeating, 40, false, false, false},
		{"Heating: should be off, currently on, can toggle", "primary", model.ModeHeating, 60, true, true, true},
		{"Heating: should be off, currently on, CANNOT toggle", "primary", model.ModeHeating, 60, true, false, false},

		// COOLING CASES
		{"Cooling: should be on, currently off, can toggle", "primary", model.ModeCooling, 80, false, true, true},
		{"Cooling: should be off, currently on, can toggle", "primary", model.ModeCooling, 60, true, true, true},
		{"Cooling: correct state, no toggle needed", "primary", model.ModeCooling, 60, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device.CanToggle = func(d *model.Device, now time.Time) bool {
				return tt.canToggle
			}

			result := buffercontroller.EvaluateToggleSource(tt.role, tt.bt, tt.active, &model.Device{Name: "test"}, tt.mode)
			assert.Equal(t, tt.expectFlip, result)
		})
	}
}

func TestEvaluateAndToggle(t *testing.T) {
	var activated, deactivated bool

	mockActivate := func() { activated = true }
	mockDeactivate := func() { deactivated = true }

	// Override evaluateToggleSource for control
	origEval := buffercontroller.EvaluateToggleSource
	defer func() { buffercontroller.EvaluateToggleSource = origEval }()
	buffercontroller.EvaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
		// simulate "should flip"
		return true
	}

	t.Run("should activate when currently off", func(t *testing.T) {
		activated, deactivated = false, false

		buffercontroller.EvaluateAndToggle("primary", model.Device{Name: "hp1"}, false, 45, model.ModeHeating, mockActivate, mockDeactivate)
		assert.True(t, activated)
		assert.False(t, deactivated)
	})

	t.Run("should deactivate when currently on", func(t *testing.T) {
		activated, deactivated = false, false

		buffercontroller.EvaluateAndToggle("secondary", model.Device{Name: "hp2"}, true, 55, model.ModeHeating, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.True(t, deactivated)
	})

	t.Run("no toggle needed", func(t *testing.T) {
		activated, deactivated = false, false

		// simulate "already in correct state"
		buffercontroller.EvaluateToggleSource = func(role string, bt float64, active bool, d *model.Device, mode model.SystemMode) bool {
			return false
		}

		buffercontroller.EvaluateAndToggle("tertiary", model.Device{Name: "boil1"}, false, 60, model.ModeHeating, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.False(t, deactivated)
	})

	t.Run("mode is off, no toggle should occur", func(t *testing.T) {
		activated, deactivated = false, false

		buffercontroller.EvaluateAndToggle("primary", model.Device{Name: "offcase"}, false, 45, model.ModeOff, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.False(t, deactivated)
	})

	t.Run("mode is circulate, no toggle should occur", func(t *testing.T) {
		activated, deactivated = false, false

		buffercontroller.EvaluateAndToggle("primary", model.Device{Name: "circ"}, true, 45, model.ModeCirculate, mockActivate, mockDeactivate)
		assert.False(t, activated)
		assert.False(t, deactivated)
	})

}
