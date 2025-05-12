package config

import (
	"encoding/json"
	"flag"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

type GPIO struct {
	BoilerRelayPin      *int `json:"boiler_relay"`
	SecondaryRelayPin   *int `json:"secondary_relay"`
	PrimaryRelayPin     *int `json:"primary_relay"`
	BufferTempSensorPin *int `json:"buffer_temp_sensor"`
}

type Config struct {
	StateFile        string
	ConfigFile       string
	LogLevel         zerolog.Level

	HeatingThreshold float64 `json:"heating_threshold"`
	CoolingThreshold float64 `json:"cooling_threshold"`
	Spread           float64 `json:"spread"`
	SecondaryMargin  float64 `json:"secondary_margin"`
	TertiaryMargin   float64 `json:"tertiary_margin"`

	RoleRotationMinutes int `json:"role_rotation_minutes"`

	GPIO GPIO `json:"gpio"`
}

func Load() Config {
	var cfg Config
	var logLevel string

	flag.StringVar(&cfg.StateFile, "state-file", "data/state.json", "Path to system state file")
	flag.StringVar(&cfg.ConfigFile, "config-file", "config.json", "Path to controller config file")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
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
	missing := []string{}

	if cfg.GPIO.BoilerRelayPin == nil {
		missing = append(missing, "gpio.boiler_relay")
	}
	if cfg.GPIO.SecondaryRelayPin == nil {
		missing = append(missing, "gpio.secondary_relay")
	}
	if cfg.GPIO.PrimaryRelayPin == nil {
		missing = append(missing, "gpio.primary_relay")
	}
	if cfg.GPIO.BufferTempSensorPin == nil {
		missing = append(missing, "gpio.buffer_temp_sensor")
	}

	if len(missing) > 0 {
		panic("Missing required GPIO config fields: " + strings.Join(missing, ", "))
	}
}
