package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

type GPIO map[string]*model.GPIOPin
type Sensors map[string]string // e.g. "garage_temp" => "28-xxxxxx"

type Config struct {
	ConfigFile         string
	DBPath             string `json:"dbPath"`
	StateFilePath      string `json:"state_file_path"`
	BootScriptFilePath string `json:"boot_script_file_path"`
	OSServicePath      string `json:"os_service_path"`
	MainServicePath    string `json:"main_service_path"`
	LogLevel           zerolog.Level
	SafeMode           bool `json:"safe_mode"`

	HeatingThreshold      float64 `json:"heating_threshold"`
	CoolingThreshold      float64 `json:"cooling_threshold"`
	ZoneMaxTemp           float64 `json:"zone_max_temp"`
	ZoneMinTemp           float64 `json:"zone_min_temp"`
	SystemOverrideMaxTemp float64 `json:"system_override_max_temp"` // TODO create async system protection override handler that heats
	SystemOverrideMinTemp float64 `json:"system_override_min_temp"` //       or cools to maintain interior zones within safe min/max
	Spread                float64 `json:"spread"`
	SecondaryMargin       float64 `json:"secondary_margin"`
	TertiaryMargin        float64 `json:"tertiary_margin"`

	RoleRotationMinutes int `json:"role_rotation_minutes"`
	PollIntervalSeconds int `json:"poll_interval_seconds"`

	TempSensorBusGPIO    int  `json:"temp_sensor_bus_gpio"`
	MainPowerGPIO        int  `json:"main_power_gpio"`
	MainPowerActiveHigh  bool `json:"main_power_active_high"`
	RelayBoardActiveHigh bool `json:"relay_board_active_high"`

	Zones         []model.Zone            `json:"zones"`
	DeviceConfig  DeviceConfig            `json:"devices"`
	SystemSensors map[string]model.Sensor `json:"system_sensors"`

	EnableDatadog bool     `json:"enable_datadog"`
	DDAgentAddr   string   `json:"dd_agent_addr"`
	DDNamespace   string   `json:"dd_namespace"`
	DDTags        []string `json:"dd_tags"`

	NtfyTopic string `json:"ntfy_topic"`
}

// DeviceConfig and related structs

type DeviceConfig struct {
	HeatPumps         HeatPumpGroup    `json:"heat_pumps"`
	AirHandlers       AirHandlerGroup  `json:"air_handlers"`
	Boilers           BoilerGroup      `json:"boilers"`
	RadiantFloorLoops RadiantLoopGroup `json:"radiant_floor_loops"`
}

type HeatPumpGroup struct {
	DeviceProfile DeviceProfile    `json:"device_profile"`
	Devices       []HeatPumpConfig `json:"devices"`
}

type AirHandlerGroup struct {
	DeviceProfile DeviceProfile      `json:"device_profile"`
	Devices       []AirHandlerConfig `json:"devices"`
}

type BoilerGroup struct {
	DeviceProfile DeviceProfile  `json:"device_profile"`
	Devices       []BoilerConfig `json:"devices"`
}

type RadiantLoopGroup struct {
	DeviceProfile DeviceProfile       `json:"device_profile"`
	Devices       []RadiantLoopConfig `json:"devices"`
}

type DeviceProfile struct {
	MinTimeOn   int      `json:"min_time_on"`
	MinTimeOff  int      `json:"min_time_off"`
	ActiveModes []string `json:"active_modes"`
}

type HeatPumpConfig struct {
	Name    string `json:"name"`
	Pin     int    `json:"pin"`
	ModePin int    `json:"mode_pin"`
}

type AirHandlerConfig struct {
	Name        string `json:"name"`
	Pin         int    `json:"pin"`
	CircPumpPin int    `json:"circ_pump_pin"`
	Zone        string `json:"zone"`
}

type BoilerConfig struct {
	Name string `json:"name"`
	Pin  int    `json:"pin"`
}

type RadiantLoopConfig struct {
	Name string `json:"name"`
	Pin  int    `json:"pin"`
	Zone string `json:"zone"`
}

// Load parses config file and CLI flags
func Load() *Config {
	var path string
	var logLevel string
	var safeMode bool

	flag.StringVar(&path, "config-file", "config.json", "Path to controller config file")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&safeMode, "safe-mode", false, "Run in dry mode without energizing GPIO")
	flag.Parse()

	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("Failed to open config file: %s", err))
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		panic(fmt.Sprintf("Failed to parse config file: %s", err))
	}

	cfg.ConfigFile = path
	cfg.LogLevel = parseLogLevel(logLevel)
	cfg.SafeMode = safeMode

	cfg.validate()
	return &cfg
}

func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func (cfg *Config) validate() {
	// Validate unique zone IDs
	zoneIDs := make(map[string]bool)
	for _, z := range cfg.Zones {
		if zoneIDs[z.ID] {
			panic(fmt.Sprintf("Duplicate zone ID found: %s", z.ID))
		}
		zoneIDs[z.ID] = true
	}

	// Validate device zone references
	for _, ah := range cfg.DeviceConfig.AirHandlers.Devices {
		if !zoneIDs[ah.Zone] {
			panic(fmt.Sprintf("Air handler %s references unknown zone ID: %s", ah.Name, ah.Zone))
		}
	}
	for _, rf := range cfg.DeviceConfig.RadiantFloorLoops.Devices {
		if !zoneIDs[rf.Zone] {
			panic(fmt.Sprintf("Radiant loop %s references unknown zone ID: %s", rf.Name, rf.Zone))
		}
	}

	// Validate GPIO pin uniqueness
	usedPins := make(map[int]string)

	check := func(pin int, label string) {
		if existing, exists := usedPins[pin]; exists {
			panic(fmt.Sprintf("GPIO pin conflict: %s and %s both use pin %d", existing, label, pin))
		}
		usedPins[pin] = label
	}

	check(cfg.TempSensorBusGPIO, "temp_sensor_bus_gpio")
	check(cfg.MainPowerGPIO, "main_power_gpio")

	for _, hp := range cfg.DeviceConfig.HeatPumps.Devices {
		check(hp.Pin, hp.Name+".pin")
		check(hp.ModePin, hp.Name+".mode_pin")
	}
	for _, ah := range cfg.DeviceConfig.AirHandlers.Devices {
		check(ah.Pin, ah.Name+".pin")
		check(ah.CircPumpPin, ah.Name+".circ_pump_pin")
	}
	for _, b := range cfg.DeviceConfig.Boilers.Devices {
		check(b.Pin, b.Name+".pin")
	}
	for _, rf := range cfg.DeviceConfig.RadiantFloorLoops.Devices {
		check(rf.Pin, rf.Name+".pin")
	}
}
