-- schema.sql

-- 🏠 System table (singleton)
CREATE TABLE IF NOT EXISTS system (
    id INTEGER PRIMARY KEY CHECK(id=1),
    system_mode TEXT NOT NULL,
    main_power_pin_number INTEGER,
    main_power_pin_active_high BOOLEAN,
    temp_sensor_bus_pin INTEGER,
    override_active BOOLEAN DEFAULT FALSE,
    prior_system_mode TEXT DEFAULT NULL,
    recirculation_active BOOLEAN DEFAULT FALSE,
    recirculation_started_at TEXT,  -- ISO8601 timestamp when recirculation started
    UNIQUE (id)  -- Ensure singleton record
);

-- 🌍 Zones table (from model.Zone)
CREATE TABLE IF NOT EXISTS zones (
    id TEXT PRIMARY KEY,  -- Using Zone.ID (string key)
    label TEXT,
    setpoint REAL,
    mode TEXT,  -- Enum from SystemMode
    capabilities TEXT,  -- JSON array of capabilities
    sensor_id TEXT REFERENCES sensors(id) ON DELETE CASCADE  -- Foreign key to sensors
);

-- 🔌 Devices table (including role: source/distributor)
CREATE TABLE IF NOT EXISTS devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    pin_number INTEGER,
    pin_active_high BOOLEAN,
    min_on INTEGER,  -- Stored as seconds or ms
    min_off INTEGER,
    online BOOLEAN,  -- Maintenance mode flag from Device struct
    last_changed TEXT,  -- ISO8601 datetime string
    active_modes TEXT,  -- JSON array of strings
    device_type TEXT,  -- Enum: heat_pump, boiler, air_handler, radiant_floor
    role TEXT NOT NULL DEFAULT 'source',  -- New column: 'source' or 'distributor'
    zone_id TEXT REFERENCES zones(id) ON DELETE SET NULL,  -- Nullable FK to zones
    circ_pump_pin_number INTEGER,  -- For air handlers only
    circ_pump_pin_active_high BOOLEAN,
    mode_pin_number INTEGER,  -- For heat pumps only
    mode_pin_active_high BOOLEAN,
    is_primary BOOLEAN,  -- For heat pumps only
    last_rotated TEXT  -- For heat pumps only
);

-- 🌡️ Sensors table (from model.Sensor)
CREATE TABLE IF NOT EXISTS sensors (
    id TEXT PRIMARY KEY,
    bus TEXT
);
