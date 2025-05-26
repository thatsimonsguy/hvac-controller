package db

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "time"

    _ "github.com/mattn/go-sqlite3"
    "github.com/thatsimonsguy/hvac-controller/internal/config"
    "github.com/thatsimonsguy/hvac-controller/internal/model"
)

var cfg *config.Config

func InitConfig(c *config.Config) {
    cfg = c
}

func SeedDatabase(dbPath string) error {
    db, err := sql.Open("sqlite3", dbPath)
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
    for _, d := range cfg.DeviceConfig.HeatPumps.Devices {
        _, err = tx.Exec(`INSERT INTO devices (name, pin_number, pin_active_high, min_on, min_off, online, last_changed, active_modes, device_type, role, zone_id, mode_pin_number, mode_pin_active_high, is_primary, last_rotated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
            d.Name, d.Pin, cfg.RelayBoardActiveHigh, int(cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOn*60), int(cfg.DeviceConfig.HeatPumps.DeviceProfile.MinTimeOff*60), true, time.Now().Format(time.RFC3339), marshalJSON(cfg.DeviceConfig.HeatPumps.DeviceProfile.ActiveModes), "heat_pump", "source", nil, d.ModePin, cfg.RelayBoardActiveHigh, false, time.Now().Format(time.RFC3339))
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

    log.Printf("Database seeded at %s from config", dbPath)
    return nil
}

func marshalJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}
