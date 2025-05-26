package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var cfg *config.Config
var schemaPath = "db/schema.sql" // not putting this is config, since the schema is actively part of the codebase and not an artifact of it

func InitConfig(c *config.Config) {
	cfg = c
}

func InitializeIfMissing() error {
	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		// Touch the file and set permissions
		f, err := os.Create(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("failed to create database file: %w", err)
		}
		f.Close()
		os.Chmod(cfg.DBPath, 0660) // Optional: Set desired permissions

		// Apply the schema
		if err := ApplySchema(); err != nil {
			return err
		}
		// Seed the database
		return SeedDatabase()
	}
	return nil // DB file exists, no action needed
}

func ApplySchema() error {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}
	schema := string(schemaBytes)

	_, err = db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to apply schema: %w", err)
	}

	log.Info().Str("path", schemaPath).Msg("Schema successfully applied")
	return nil
}

func SeedDatabase() error {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert system record
	_, err = tx.Exec(`INSERT OR REPLACE INTO system (id, system_mode, main_power_pin_number, main_power_pin_active_high, temp_sensor_bus_pin) VALUES (1, ?, ?, ?, ?)`,
		model.ModeOff, cfg.MainPowerGPIO, cfg.MainPowerActiveHigh, cfg.TempSensorBusGPIO)
	if err != nil {
		return fmt.Errorf("failed to insert system record: %w", err)
	}

	// Insert sensors
	for _, s := range cfg.SystemSensors {
		_, err = tx.Exec(`INSERT OR REPLACE INTO sensors (id, bus) VALUES (?, ?)`, s.ID, s.Bus)
		if err != nil {
			return fmt.Errorf("failed to insert sensor %s: %w", s.ID, err)
		}
	}

	// Insert zones
	for _, z := range cfg.Zones {
		_, err = tx.Exec(`INSERT OR REPLACE INTO zones (id, label, setpoint, mode, capabilities, sensor_id) VALUES (?, ?, ?, ?, ?, ?)`,
			z.ID, z.Label, z.Setpoint, model.ModeOff, marshalJSON(z.Capabilities), z.Sensor.ID)
		if err != nil {
			return fmt.Errorf("failed to insert zone %s: %w", z.ID, err)
		}
	}

	// Insert devices from config with role assignment
	for i, d := range cfg.DeviceConfig.HeatPumps.Devices {
		primary := i == 0 // Mark the first HP as primary
		_, err = tx.Exec(`INSERT INTO devices (name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role, zone_id, mode_pin_number, mode_pin_active_high, is_primary, last_rotated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Name, d.Pin, cfg.RelayBoardActiveHigh, int(cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOn*60), int(cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOff*60), true, time.Now().Format(time.RFC3339), marshalJSON(cfg.DeviceConfig.HeatPumps.DeviceProfile.ActiveModes), "heat_pump", "source", nil, d.ModePin, cfg.RelayBoardActiveHigh, primary, time.Now().Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("failed to insert heat pump %s: %w", d.Name, err)
		}
	}
	for _, d := range cfg.DeviceConfig.Boilers.Devices {
		_, err = tx.Exec(`INSERT INTO devices (name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Name, d.Pin, cfg.RelayBoardActiveHigh, int(cfg.DeviceConfig.Boilers.DeviceProfile.MinTimeOn*60), int(cfg.DeviceConfig.Boilers.DeviceProfile.MinTimeOff*60), true, time.Now().Format(time.RFC3339), marshalJSON(cfg.DeviceConfig.Boilers.DeviceProfile.ActiveModes), "boiler", "source")
		if err != nil {
			return fmt.Errorf("failed to insert boiler %s: %w", d.Name, err)
		}
	}
	for _, d := range cfg.DeviceConfig.AirHandlers.Devices {
		_, err = tx.Exec(`INSERT INTO devices (name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role, zone_id, circ_pump_pin_number, circ_pump_pin_active_high) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Name, d.Pin, cfg.RelayBoardActiveHigh, int(cfg.DeviceConfig.AirHandlers.DeviceProfile.MinTimeOn*60), int(cfg.DeviceConfig.AirHandlers.DeviceProfile.MinTimeOff*60), true, time.Now().Format(time.RFC3339), marshalJSON(cfg.DeviceConfig.AirHandlers.DeviceProfile.ActiveModes), "air_handler", "distributor", d.Zone, d.CircPumpPin, cfg.RelayBoardActiveHigh)
		if err != nil {
			return fmt.Errorf("failed to insert air handler %s: %w", d.Name, err)
		}
	}
	for _, d := range cfg.DeviceConfig.RadiantFloorLoops.Devices {
		_, err = tx.Exec(`INSERT INTO devices (name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role, zone_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Name, d.Pin, cfg.RelayBoardActiveHigh, int(cfg.DeviceConfig.RadiantFloorLoops.DeviceProfile.MinTimeOn*60), int(cfg.DeviceConfig.RadiantFloorLoops.DeviceProfile.MinTimeOff*60), true, time.Now().Format(time.RFC3339), marshalJSON(cfg.DeviceConfig.RadiantFloorLoops.DeviceProfile.ActiveModes), "radiant_floor", "distributor", d.Zone)
		if err != nil {
			return fmt.Errorf("failed to insert radiant loop %s: %w", d.Name, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit seed transaction: %w", err)
	}

	log.Info().Str("path", cfg.DBPath).Msg("Database seeded from config")
	return nil
}

func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func ValidateDatabase() error {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Check for expected tables and count of key entries
	tables := []string{"system", "zones", "devices", "sensors"}
	for _, table := range tables {
		var count int
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to query table %s: %w", table, err)
		}
		log.Debug().Str("table", table).Int("records", count).Msg("Table validated")
	}

	log.Info().Msg("Database validated")
	return nil
}
