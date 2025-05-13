package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/rs/zerolog"
)

type GPIOPin struct {
	Pin       int   `json:"pin"`
	SafeState *bool `json:"safe_state"` // true = HIGH, false = LOW, null = SENSOR INPUT
}

type GPIO map[string]*GPIOPin
type Sensors map[string]string // e.g. "garage_temp" => "28-xxxxxx"

type Config struct {
	StateFile  string
	ConfigFile string
	LogLevel   zerolog.Level
	SafeMode   bool `json:"safe_mode"`

	HeatingThreshold float64 `json:"heating_threshold"`
	CoolingThreshold float64 `json:"cooling_threshold"`
	Spread           float64 `json:"spread"`
	SecondaryMargin  float64 `json:"secondary_margin"`
	TertiaryMargin   float64 `json:"tertiary_margin"`

	RoleRotationMinutes int `json:"role_rotation_minutes"`
	PollIntervalSeconds int `json:"poll_interval_seconds"`

	GPIO GPIO `json:"gpio"`
}

func Load() Config {
	var cfg Config
	var logLevel string

	flag.StringVar(&cfg.StateFile, "state-file", "data/state.json", "Path to system state file")
	flag.StringVar(&cfg.ConfigFile, "config-file", "config.json", "Path to controller config file")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.SafeMode, "safe-mode", true, "Run without energizing relays or controlling GPIO")
	flag.Parse()

	cfg.LogLevel = parseLogLevel(logLevel)

	file, err := os.Open(cfg.ConfigFile)
	if err != nil {
		panic("Failed to load config file: " + err.Error())
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		panic("Failed to parse config file: " + err.Error())
	}

	if cfg.PollIntervalSeconds == 0 {
		cfg.PollIntervalSeconds = 30
	}

	cfg.validate()
	return cfg
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
	pins := map[int]string{}

	for name, gpioPin := range cfg.GPIO {
		if gpioPin == nil {
			panic("Missing required GPIO config for: gpio." + name)
		}
		if other, exists := pins[gpioPin.Pin]; exists {
			panic(fmt.Sprintf("Conflicting GPIO pin usage: gpio.%s and gpio.%s both use pin %d", name, other, gpioPin.Pin))
		}
		pins[gpioPin.Pin] = name
	}
}
