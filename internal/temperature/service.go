package temperature

import (
	"database/sql"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

type Reading struct {
	Temperature float64
	Timestamp   time.Time
	Valid       bool
}

type Service struct {
	dbConn      *sql.DB
	readings    map[string]Reading // sensorID -> Reading
	mutex       sync.RWMutex
	pollInterval time.Duration
}

func NewService(dbConn *sql.DB, pollIntervalSeconds int) *Service {
	return &Service{
		dbConn:       dbConn,
		readings:     make(map[string]Reading),
		pollInterval: time.Duration(pollIntervalSeconds) * time.Second,
	}
}

func (s *Service) Start() {
	go func() {
		log.Info().Msg("Starting centralized temperature reading service")
		
		// Initial delay to let system stabilize
		time.Sleep(30 * time.Second)
		
		for {
			s.readAllSensors()
			time.Sleep(s.pollInterval)
		}
	}()
}

func (s *Service) readAllSensors() {
	log.Info().Msg("Reading all temperature sensors")
	
	// Get all zones and their sensors
	zones, err := db.GetAllZones(s.dbConn)
	if err != nil {
		log.Error().Err(err).Msg("Could not retrieve zones for temperature reading")
		return
	}

	// Get buffer tank sensor - it uses "buffer_tank" as the sensor ID
	bufferSensor, err := db.GetSensorByID(s.dbConn, "buffer_tank")
	if err != nil {
		log.Error().Err(err).Msg("Could not retrieve buffer tank sensor for temperature reading")
	}

	var sensorsToRead []model.Sensor
	
	// Collect all unique sensors
	sensorMap := make(map[string]model.Sensor)
	
	for _, zone := range zones {
		sensor, err := db.GetSensorByID(s.dbConn, zone.Sensor.ID)
		if err != nil {
			log.Error().Err(err).Str("zone", zone.ID).Msg("Could not retrieve sensor for zone")
			continue
		}
		sensorMap[sensor.ID] = *sensor
	}
	
	if bufferSensor != nil {
		sensorMap[bufferSensor.ID] = *bufferSensor
	}
	
	// Convert map to slice
	for _, sensor := range sensorMap {
		sensorsToRead = append(sensorsToRead, sensor)
	}
	
	// Read all sensors sequentially with delays between reads
	newReadings := make(map[string]Reading)
	
	for i, sensor := range sensorsToRead {
		// Add delay between sensor reads to avoid overwhelming the one-wire bus
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		
		sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
		temp := gpio.ReadSensorTempWithRetries(sensorPath, 3) // Reduced retries since we're doing this centrally
		
		reading := Reading{
			Temperature: temp,
			Timestamp:   time.Now(),
			Valid:       temp > 0, // Consider 0° as invalid reading
		}
		
		if !reading.Valid {
			log.Warn().
				Str("sensor_id", sensor.ID).
				Float64("temp", temp).
				Msg("Invalid temperature reading from sensor")
		} else {
			log.Debug().
				Str("sensor_id", sensor.ID).
				Float64("temp", temp).
				Msg("Temperature reading successful")
		}
		
		newReadings[sensor.ID] = reading
	}
	
	// Update readings atomically
	s.mutex.Lock()
	s.readings = newReadings
	s.mutex.Unlock()
	
	log.Info().
		Int("sensors_read", len(sensorsToRead)).
		Msg("Completed temperature reading cycle")
}

func (s *Service) GetTemperature(sensorID string) (float64, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	reading, exists := s.readings[sensorID]
	if !exists {
		log.Warn().Str("sensor_id", sensorID).Msg("No temperature reading available for sensor")
		return 0, false
	}
	
	// Check if reading is stale (older than 2x poll interval)
	if time.Since(reading.Timestamp) > 2*s.pollInterval {
		log.Warn().
			Str("sensor_id", sensorID).
			Dur("age", time.Since(reading.Timestamp)).
			Msg("Temperature reading is stale")
		return 0, false
	}
	
	if !reading.Valid {
		log.Debug().Str("sensor_id", sensorID).Msg("Temperature reading marked as invalid")
		return 0, false
	}
	
	return reading.Temperature, true
}

func (s *Service) GetAllReadings() map[string]Reading {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Return a copy to avoid race conditions
	result := make(map[string]Reading)
	for k, v := range s.readings {
		result[k] = v
	}
	return result
}