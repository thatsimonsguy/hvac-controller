package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

var (
	// ErrZoneNotFound is returned when a zone with the given ID is not found
	ErrZoneNotFound = errors.New("zone not found")
)

var cfg *config.Config

func Init(c *config.Config) {
	cfg = c
}

type SystemState struct {
	mu               sync.RWMutex
	SystemMode       model.SystemMode         `json:"system_mode"`
	Zones            []model.Zone             `json:"zones"`
	HeatPumps        []model.HeatPump         `json:"heat_pumps"`
	AirHandlers      []model.AirHandler       `json:"air_handlers"`
	Boilers          []model.Boiler           `json:"boilers"`
	RadiantLoops     []model.RadiantFloorLoop `json:"radiant_loops"`
	MainPowerPin     model.GPIOPin            `json:"main_power_pin"`
	TempSensorBusPin int                      `json:"temp_sensor_bus_pin"`
	SystemSensors    map[string]model.Sensor  `json:"system_sensors"`
}

func NewSystemStateFromConfig() *SystemState {
	return &SystemState{
		SystemMode:       model.ModeOff,
		Zones:            hydrateZones(),
		HeatPumps:        hydrateHeatPumps(),
		AirHandlers:      hydrateAirHandlers(),
		Boilers:          hydrateBoilers(),
		RadiantLoops:     hydrateRadiantLoops(),
		MainPowerPin:     model.GPIOPin{Number: cfg.MainPowerGPIO, ActiveHigh: cfg.MainPowerActiveHigh},
		TempSensorBusPin: cfg.TempSensorBusGPIO,
		SystemSensors:    cfg.SystemSensors,
	}
}

func LoadSystemState(path string) (*SystemState, error) {
	f, err := os.Open(filepath.Join(path, "state.json"))
	log.Info().Str("resolved_path", filepath.Join(path, "state.json")).Msg("Loading system state")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var state SystemState
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SetSystemMode safely updates the system mode
func (s *SystemState) SetSystemMode(mode model.SystemMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SystemMode = mode
}

// GetSystemMode safely retrieves the current system mode
func (s *SystemState) GetSystemMode() model.SystemMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SystemMode
}

// findZoneIndex returns the index of the zone with the given ID, or -1 if not found
// Caller must hold at least a read lock
func (s *SystemState) findZoneIndex(zoneID string) int {
	for i, zone := range s.Zones {
		if zone.ID == zoneID {
			return i
		}
	}
	return -1
}

// SetZoneSetpoint safely updates the setpoint for a specific zone
func (s *SystemState) SetZoneSetpoint(zoneID string, setpoint float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := s.findZoneIndex(zoneID); i != -1 {
		s.Zones[i].Setpoint = setpoint
		return nil
	}
	return ErrZoneNotFound
}

// SetZoneMode safely updates the mode for a specific zone
func (s *SystemState) SetZoneMode(zoneID string, mode model.SystemMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if i := s.findZoneIndex(zoneID); i != -1 {
		s.Zones[i].Mode = mode
		return nil
	}
	return ErrZoneNotFound
}

// GetZone safely retrieves a copy of a zone by ID
func (s *SystemState) GetZone(zoneID string) (model.Zone, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if i := s.findZoneIndex(zoneID); i != -1 {
		return s.Zones[i], nil
	}
	return model.Zone{}, ErrZoneNotFound
}

// GetAllZones safely retrieves a copy of all zones
func (s *SystemState) GetAllZones() []model.Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a deep copy to prevent external modifications
	zones := make([]model.Zone, len(s.Zones))
	copy(zones, s.Zones)
	return zones
}

// SaveSystemState saves the current state to disk in a thread-safe manner
func SaveSystemState(path string, state *SystemState) error {
	// Create a copy of the state while holding a read lock
	state.mu.RLock()
	defer state.mu.RUnlock()

	tmp := filepath.Join(path, "state.json.tmp")
	out := filepath.Join(path, "state.json")
	log.Info().Str("resolved_path", filepath.Join(path, "state.json")).Msg("Saving system state")

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		f.Close()
		return err
	}
	f.Sync()
	f.Close()
	return os.Rename(tmp, out)
}

func hydrateZones() []model.Zone {
	zones := make([]model.Zone, 0, len(cfg.Zones))
	for _, z := range cfg.Zones {
		zones = append(zones, model.Zone{
			ID:           z.ID,
			Label:        z.Label,
			Setpoint:     z.Setpoint,
			Capabilities: z.Capabilities,
			Sensor:       z.Sensor,
			Mode:         model.ModeOff,
		})
	}
	return zones
}

func hydrateHeatPumps() []model.HeatPump {
	hpProfile := cfg.DeviceConfig.HeatPumps.DeviceProfile
	hpList := make([]model.HeatPump, 0, len(cfg.DeviceConfig.HeatPumps.Devices))
	for _, hp := range cfg.DeviceConfig.HeatPumps.Devices {
		hpList = append(hpList, model.HeatPump{
			Device: model.Device{
				Name:        hp.Name,
				Pin:         model.GPIOPin{Number: hp.Pin, ActiveHigh: cfg.RelayBoardActiveHigh},
				MinOn:       time.Duration(hpProfile.MinTimeOn) * time.Minute,
				MinOff:      time.Duration(hpProfile.MinTimeOff) * time.Minute,
				Online:      true,
				ActiveModes: hpProfile.ActiveModes,
			},
			ModePin:     model.GPIOPin{Number: hp.ModePin, ActiveHigh: cfg.RelayBoardActiveHigh},
			IsPrimary:   false,
			LastRotated: time.Now(),
		})
	}
	if len(hpList) > 0 {
		hpList[0].IsPrimary = true
	}
	return hpList
}

func hydrateAirHandlers() []model.AirHandler {
	ahProfile := cfg.DeviceConfig.AirHandlers.DeviceProfile
	ahList := make([]model.AirHandler, 0, len(cfg.DeviceConfig.AirHandlers.Devices))
	zoneLookup := buildZoneLookup()

	for _, ah := range cfg.DeviceConfig.AirHandlers.Devices {
		zone := zoneLookup[ah.Zone]
		ahList = append(ahList, model.AirHandler{
			Device: model.Device{
				Name:        ah.Name,
				Pin:         model.GPIOPin{Number: ah.Pin, ActiveHigh: cfg.RelayBoardActiveHigh},
				MinOn:       time.Duration(ahProfile.MinTimeOn) * time.Minute,
				MinOff:      time.Duration(ahProfile.MinTimeOff) * time.Minute,
				Online:      true,
				ActiveModes: ahProfile.ActiveModes,
			},
			Zone:        zone,
			CircPumpPin: model.GPIOPin{Number: ah.CircPumpPin, ActiveHigh: cfg.RelayBoardActiveHigh},
		})
	}
	return ahList
}

func hydrateBoilers() []model.Boiler {
	boilerProfile := cfg.DeviceConfig.Boilers.DeviceProfile
	boilerList := make([]model.Boiler, 0, len(cfg.DeviceConfig.Boilers.Devices))

	for _, b := range cfg.DeviceConfig.Boilers.Devices {
		boilerList = append(boilerList, model.Boiler{
			Device: model.Device{
				Name:        b.Name,
				Pin:         model.GPIOPin{Number: b.Pin, ActiveHigh: cfg.RelayBoardActiveHigh},
				MinOn:       time.Duration(boilerProfile.MinTimeOn) * time.Minute,
				MinOff:      time.Duration(boilerProfile.MinTimeOff) * time.Minute,
				Online:      true,
				ActiveModes: boilerProfile.ActiveModes,
			},
		})
	}
	return boilerList
}

func hydrateRadiantLoops() []model.RadiantFloorLoop {
	rfProfile := cfg.DeviceConfig.RadiantFloorLoops.DeviceProfile
	rfList := make([]model.RadiantFloorLoop, 0, len(cfg.DeviceConfig.RadiantFloorLoops.Devices))
	zoneLookup := buildZoneLookup()

	for _, rf := range cfg.DeviceConfig.RadiantFloorLoops.Devices {
		zone := zoneLookup[rf.Zone]
		rfList = append(rfList, model.RadiantFloorLoop{
			Device: model.Device{
				Name:        rf.Name,
				Pin:         model.GPIOPin{Number: rf.Pin, ActiveHigh: cfg.RelayBoardActiveHigh},
				MinOn:       time.Duration(rfProfile.MinTimeOn) * time.Minute,
				MinOff:      time.Duration(rfProfile.MinTimeOff) * time.Minute,
				Online:      true,
				ActiveModes: rfProfile.ActiveModes,
			},
			Zone: zone,
		})
	}
	return rfList
}

// Helper: create a map of zone ID to pointer
func buildZoneLookup() map[string]*model.Zone {
	zoneLookup := make(map[string]*model.Zone)
	for i := range cfg.Zones {
		z := &cfg.Zones[i]
		zoneLookup[z.ID] = &model.Zone{
			ID:           z.ID,
			Label:        z.Label,
			Setpoint:     z.Setpoint,
			Capabilities: z.Capabilities,
			Sensor:       z.Sensor,
			Mode:         z.Mode,
		}
	}
	return zoneLookup
}

// Helper: convert duration minutes to time.Duration
func minDur(mins int) time.Duration {
	return time.Duration(mins) * time.Minute
}
