package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/rs/zerolog"
)

type GPIO struct {
	// temp sensors
	MainFloorTempSensor *int `json:"main_floor_temp_sensor"`
	BasementTempSensor  *int `json:"basement_temp_sensor"`
	GarageTempSensor    *int `json:"garage_temp_sensor"`

	// air handlers
	MainFloorAirBlower *int `json:"main_floor_air_blower"`
	MainFloorAirPump   *int `json:"main_floor_air_pump"`
	BasementAirBlower  *int `json:"basement_air_blower"`
	BasementAirPump    *int `json:"basement_air_pump"`

	// radiant heating pumps
	BasementRadiantPump *int `json:"basement_radiant_pump"`
	GarageRadiantPump   *int `json:"garage_radiant_pump"`

	// heating and cooling sources
	BoilerRelayPin    *int `json:"boiler_relay"`
	HeatPumpARelayPin *int `json:"heat_pump_A_relay"`
	HeatPumpBRelayPin *int `json:"heat_pump_B_relay"`

	// misc
	BufferTempSensorPin *int `json:"buffer_temp_sensor"`
	MainPowerRelayPin   *int `json:"main_power_relay"`
}

type Config struct {
	StateFile  string
	ConfigFile string
	LogLevel   zerolog.Level

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
	var (
		missingFields []string
		usedPins      = map[int]string{}
		conflicts     []string
	)

	v := reflect.ValueOf(cfg.GPIO)
	t := reflect.TypeOf(cfg.GPIO)

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldName := t.Field(i).Tag.Get("json")

		if field.IsNil() {
			missingFields = append(missingFields, "gpio."+fieldName)
			continue
		}

		pin := field.Elem().Int()
		if other, exists := usedPins[int(pin)]; exists {
			conflicts = append(conflicts, fmt.Sprintf("gpio.%s and gpio.%s both use pin %d", fieldName, other, pin))
		} else {
			usedPins[int(pin)] = fieldName
		}
	}

	if len(missingFields) > 0 {
		panic("Missing required GPIO config fields: " + strings.Join(missingFields, ", "))
	}
	if len(conflicts) > 0 {
		panic("Conflicting GPIO pins: " + strings.Join(conflicts, ", "))
	}
}
