package temperature

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/notifications"
	"github.com/thatsimonsguy/hvac-controller/system/shutdown"
)

type Reading struct {
	Temperature float64
	Timestamp   time.Time
	Valid       bool
}

type ReadingHistory struct {
	Readings         []Reading
	MaxSize          int
	AnomalyCount     int
	RecoveryCount    int  // Consecutive good readings while disabled
	Disabled         bool
	DisabledAt       time.Time
	LastGoodReading  Reading
	SensorZone       string // Zone ID for this sensor
}

// Notifier interface for sending notifications
type Notifier interface {
	Send(title, message string) error
}

// Shutdowner interface for system shutdown
type Shutdowner interface {
	Shutdown()
}

type Service struct {
	dbConn       *sql.DB
	readings     map[string]Reading          // Current reading (public API)
	history      map[string]*ReadingHistory  // Anomaly detection history
	sensorZones  map[string]string           // sensorID -> zoneID mapping
	mutex        sync.RWMutex
	pollInterval time.Duration

	// Configuration
	maxTempDelta    float64
	garageTempDelta float64
	maxAnomalies    int
	historySize     int

	// Dependencies (for testing)
	notifier  Notifier
	shutdowner Shutdowner
}

func NewService(dbConn *sql.DB, pollIntervalSeconds int) *Service {
	return &Service{
		dbConn:          dbConn,
		readings:        make(map[string]Reading),
		history:         make(map[string]*ReadingHistory),
		sensorZones:     make(map[string]string),
		pollInterval:    time.Duration(pollIntervalSeconds) * time.Second,
		maxTempDelta:    env.Cfg.TempAnomalyMaxDelta,
		garageTempDelta: env.Cfg.TempAnomalyGarageDelta,
		maxAnomalies:    env.Cfg.TempMaxAnomalies,
		historySize:     env.Cfg.TempHistorySize,
		notifier:        &realNotifier{},
		shutdowner:      &realShutdowner{},
	}
}

// TestDeps holds test dependencies
type TestDeps struct {
	Notifier  Notifier
	Shutdowner Shutdowner
}

// NewServiceForTest creates a service with injectable dependencies for testing
func NewServiceForTest(dbConn *sql.DB, pollIntervalSeconds int, deps *TestDeps) *Service {
	s := &Service{
		dbConn:          dbConn,
		readings:        make(map[string]Reading),
		history:         make(map[string]*ReadingHistory),
		sensorZones:     make(map[string]string),
		pollInterval:    time.Duration(pollIntervalSeconds) * time.Second,
		maxTempDelta:    5.0,
		garageTempDelta: 25.0,
		maxAnomalies:    6,
		historySize:     20,
		notifier:        deps.Notifier,
		shutdowner:      deps.Shutdowner,
	}

	return s
}

// Real implementations
type realNotifier struct{}

func (r *realNotifier) Send(title, message string) error {
	return notifications.Send(title, message)
}

type realShutdowner struct{}

func (r *realShutdowner) Shutdown() {
	shutdown.Shutdown()
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

	// Get buffer tank sensor
	bufferSensor, err := db.GetSensorByID(s.dbConn, "buffer_tank")
	if err != nil {
		log.Error().Err(err).Msg("Could not retrieve buffer tank sensor for temperature reading")
	}

	// Build sensor-to-zone mapping
	s.mutex.Lock()
	for _, zone := range zones {
		s.sensorZones[zone.Sensor.ID] = zone.ID
	}
	if bufferSensor != nil {
		s.sensorZones[bufferSensor.ID] = "buffer_tank"
	}
	s.mutex.Unlock()

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
	timestamp := time.Now()

	for i, sensor := range sensorsToRead {
		// Add delay between sensor reads to avoid overwhelming the one-wire bus
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		sensorPath := filepath.Join("/sys/bus/w1/devices", sensor.Bus)
		temp := gpio.ReadSensorTempWithRetries(sensorPath, 3)

		// Get zone for this sensor
		s.mutex.RLock()
		sensorZone := s.sensorZones[sensor.ID]
		s.mutex.RUnlock()

		// Process reading through anomaly detection
		accepted := s.processReading(sensor.ID, sensorZone, temp, timestamp)

		if !accepted {
			log.Warn().
				Str("sensor_id", sensor.ID).
				Str("zone", sensorZone).
				Float64("temp", temp).
				Msg("Temperature reading rejected as anomalous")
		} else {
			log.Debug().
				Str("sensor_id", sensor.ID).
				Str("zone", sensorZone).
				Float64("temp", temp).
				Msg("Temperature reading accepted")
		}
	}

	log.Info().
		Int("sensors_read", len(sensorsToRead)).
		Msg("Completed temperature reading cycle")
}

// processReading handles anomaly detection and validation
func (s *Service) processReading(sensorID, sensorZone string, temp float64, timestamp time.Time) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Initialize history if needed
	if s.history[sensorID] == nil {
		s.history[sensorID] = &ReadingHistory{
			Readings:   make([]Reading, 0, s.historySize),
			MaxSize:    s.historySize,
			SensorZone: sensorZone,
		}
	}

	history := s.history[sensorID]

	// Create new reading
	newReading := Reading{
		Temperature: temp,
		Timestamp:   timestamp,
		Valid:       temp > 0, // Basic validity check
	}

	// If reading is fundamentally invalid (temp <= 0), reject it
	if !newReading.Valid {
		history.AnomalyCount++
		s.checkDisableThreshold(sensorID, history, temp)
		return false
	}

	// Bootstrap phase: accept readings until we have enough history
	if len(history.Readings) < s.maxAnomalies {
		history.Readings = append(history.Readings, newReading)
		history.LastGoodReading = newReading
		s.readings[sensorID] = newReading

		// On 6th reading, analyze history for anomalies
		if len(history.Readings) == s.maxAnomalies {
			s.analyzeBootstrapHistory(history)
		}

		return true
	}

	// Check if sensor is disabled
	if history.Disabled {
		// Check if this is a good reading for recovery
		if s.isGoodReading(history, temp) {
			history.RecoveryCount++
			if history.RecoveryCount >= s.maxAnomalies {
				// Re-enable sensor
				history.Disabled = false
				history.AnomalyCount = 0
				history.RecoveryCount = 0
				history.LastGoodReading = newReading
				s.addToHistory(history, newReading)
				s.readings[sensorID] = newReading

				s.sendRecoveryNotification(sensorZone, temp)
				log.Info().
					Str("sensor_id", sensorID).
					Str("zone", sensorZone).
					Msg("Sensor recovered and re-enabled")

				return true
			}
			// Still recovering, accept but don't fully re-enable yet
			s.addToHistory(history, newReading)
			s.readings[sensorID] = newReading
			return true
		} else {
			// Bad reading while disabled, reset recovery counter
			history.RecoveryCount = 0
			// Use last good reading
			s.readings[sensorID] = history.LastGoodReading
			return false
		}
	}

	// Normal operation: check for anomalies
	if s.isAnomalousReading(history, temp) {
		// Add to history even if anomalous (needed for pattern detection)
		s.addToHistory(history, newReading)

		// Check for stable new baseline (smart recovery)
		if s.detectStableNewBaseline(history, temp) {
			// Accept as new baseline
			history.AnomalyCount = 0
			history.LastGoodReading = newReading
			s.readings[sensorID] = newReading
			log.Info().
				Str("sensor_id", sensorID).
				Str("zone", sensorZone).
				Float64("temp", temp).
				Msg("Stable new baseline detected, accepting temperature")
			return true
		}

		// Anomaly detected
		history.AnomalyCount++
		s.checkDisableThreshold(sensorID, history, temp)

		// Use last good reading
		s.readings[sensorID] = history.LastGoodReading
		return false
	}

	// Good reading - only reset anomaly count if not from bootstrap
	if len(history.Readings) > s.maxAnomalies {
		history.AnomalyCount = 0
	}
	history.RecoveryCount = 0
	history.LastGoodReading = newReading
	s.addToHistory(history, newReading)
	s.readings[sensorID] = newReading

	return true
}

// isAnomalousReading checks if a reading is anomalous
func (s *Service) isAnomalousReading(history *ReadingHistory, temp float64) bool {
	if history.LastGoodReading.Temperature == 0 {
		return false // No baseline yet
	}

	delta := math.Abs(temp - history.LastGoodReading.Temperature)

	// Check against appropriate threshold
	threshold := s.maxTempDelta
	if history.SensorZone == "garage" {
		threshold = s.garageTempDelta
	}

	return delta > threshold
}

// isGoodReading checks if a reading is within acceptable range
func (s *Service) isGoodReading(history *ReadingHistory, temp float64) bool {
	if history.LastGoodReading.Temperature == 0 {
		return true // No baseline, accept
	}

	delta := math.Abs(temp - history.LastGoodReading.Temperature)

	threshold := s.maxTempDelta
	if history.SensorZone == "garage" {
		threshold = s.garageTempDelta
	}

	return delta <= threshold
}

// detectStableNewBaseline checks if recent readings show a stable new level OR gradual legitimate change
func (s *Service) detectStableNewBaseline(history *ReadingHistory, newTemp float64) bool {
	// Need at least 1 prior anomaly before checking for pattern
	// (called on the 2nd anomalous reading)
	if history.AnomalyCount < 1 {
		return false
	}

	// Get the most recent 3 readings (faster detection)
	numToCheck := 3
	// Need at least bootstrap + 2 new readings to establish a pattern
	if len(history.Readings) < s.maxAnomalies+2 {
		return false // Need bootstrap + pattern readings
	}

	recentTemps := make([]float64, 0, numToCheck)
	startIdx := len(history.Readings) - numToCheck
	for i := startIdx; i < len(history.Readings); i++ {
		recentTemps = append(recentTemps, history.Readings[i].Temperature)
	}

	// Calculate standard deviation
	var sum float64
	for _, t := range recentTemps {
		sum += t
	}
	mean := sum / float64(len(recentTemps))

	var variance float64
	for _, t := range recentTemps {
		variance += (t - mean) * (t - mean)
	}
	variance /= float64(len(recentTemps))
	stdDev := math.Sqrt(variance)

	// Check 1: Stable at new level (low variance)
	if stdDev < 2.0 {
		log.Debug().
			Str("zone", history.SensorZone).
			Float64("mean", mean).
			Float64("stddev", stdDev).
			Int("samples", len(recentTemps)).
			Msg("Stable baseline detected")
		return true
	}

	// Check 2: Gradual legitimate change (monotonic with reasonable deltas)
	// Only check anomalous readings (post-bootstrap) for gradual patterns
	anomalousTemps := make([]float64, 0)
	for i := s.maxAnomalies; i < len(history.Readings); i++ {
		anomalousTemps = append(anomalousTemps, history.Readings[i].Temperature)
	}
	// Only check if we haven't accumulated too many anomalies (< 4)
	// If we have 4+ anomalies, it's likely sensor failure, not legitimate change
	if history.AnomalyCount >= 4 {
		return false
	}

	if len(anomalousTemps) < 2 {
		return false
	}

	// Check total deviation from baseline - if too large, it's sensor failure
	totalDeviation := math.Abs(mean - history.LastGoodReading.Temperature)
	maxDeviation := 15.0 // Max 15° total change from baseline
	if history.SensorZone == "garage" {
		maxDeviation = 30.0 // Garage can change more
	}

	if totalDeviation > maxDeviation {
		// Total change too large - likely sensor failure
		return false
	}

	isMonotonic := true
	increasing := anomalousTemps[1] > anomalousTemps[0]
	maxDelta := 0.0

	for i := 1; i < len(anomalousTemps); i++ {
		delta := anomalousTemps[i] - anomalousTemps[i-1]
		absDelta := math.Abs(delta)

		// Check if direction is consistent
		if increasing && delta < -0.5 {
			isMonotonic = false
			break
		}
		if !increasing && delta > 0.5 {
			isMonotonic = false
			break
		}

		// Track max delta between consecutive readings
		if absDelta > maxDelta {
			maxDelta = absDelta
		}
	}

	// If monotonic and deltas are reasonable (not too large), it's a legitimate gradual change
	threshold := s.maxTempDelta
	if history.SensorZone == "garage" {
		threshold = s.garageTempDelta
	}

	if isMonotonic && maxDelta > 0 && maxDelta <= threshold {
		log.Debug().
			Str("zone", history.SensorZone).
			Bool("increasing", increasing).
			Float64("max_delta", maxDelta).
			Float64("total_deviation", totalDeviation).
			Msg("Gradual legitimate temperature change detected")
		return true
	}

	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// analyzeBootstrapHistory analyzes the first 6 readings for anomalies
func (s *Service) analyzeBootstrapHistory(history *ReadingHistory) {
	if len(history.Readings) != s.maxAnomalies {
		return
	}

	// Calculate mean and stddev of bootstrap readings
	var sum float64
	for _, r := range history.Readings {
		sum += r.Temperature
	}
	mean := sum / float64(len(history.Readings))

	var variance float64
	for _, r := range history.Readings {
		variance += (r.Temperature - mean) * (r.Temperature - mean)
	}
	variance /= float64(len(history.Readings))
	stdDev := math.Sqrt(variance)

	// Find outliers (>2 stddev from mean)
	anomalyCount := 0
	var lastGoodTemp float64
	for i := len(history.Readings) - 1; i >= 0; i-- {
		r := history.Readings[i]
		if math.Abs(r.Temperature-mean) > 2*stdDev {
			anomalyCount++
		} else if lastGoodTemp == 0 {
			lastGoodTemp = r.Temperature
		}
	}

	if lastGoodTemp > 0 {
		history.LastGoodReading = Reading{
			Temperature: lastGoodTemp,
			Timestamp:   time.Now(),
			Valid:       true,
		}
	} else {
		// All readings were anomalous? Use mean
		history.LastGoodReading = Reading{
			Temperature: mean,
			Timestamp:   time.Now(),
			Valid:       true,
		}
	}

	history.AnomalyCount = anomalyCount

	if anomalyCount > 0 {
		log.Info().
			Str("zone", history.SensorZone).
			Int("anomalies_found", anomalyCount).
			Float64("baseline_temp", history.LastGoodReading.Temperature).
			Msg("Bootstrap analysis complete")
	}
}

// addToHistory adds a reading to the circular buffer
func (s *Service) addToHistory(history *ReadingHistory, reading Reading) {
	if len(history.Readings) >= history.MaxSize {
		// Remove oldest
		history.Readings = history.Readings[1:]
	}
	history.Readings = append(history.Readings, reading)
}

// checkDisableThreshold checks if we should disable the sensor
func (s *Service) checkDisableThreshold(sensorID string, history *ReadingHistory, temp float64) {
	if history.AnomalyCount >= s.maxAnomalies && !history.Disabled {
		history.Disabled = true
		history.DisabledAt = time.Now()

		zoneName := s.getZoneName(history.SensorZone)
		s.sendDisableNotification(history.SensorZone, zoneName, temp, history.LastGoodReading.Temperature)

		// If buffer tank, shutdown system
		if history.SensorZone == "buffer_tank" {
			log.Error().
				Str("zone", history.SensorZone).
				Float64("temp", temp).
				Msg("Buffer tank sensor failed - shutting down system")
			s.shutdowner.Shutdown()
		}
	}
}

// getZoneName returns a human-readable zone name
func (s *Service) getZoneName(zoneID string) string {
	switch zoneID {
	case "main_floor":
		return "Main Floor"
	case "basement":
		return "Basement"
	case "garage":
		return "Garage"
	case "buffer_tank":
		return "Buffer Tank"
	default:
		return zoneID
	}
}

// sendDisableNotification sends notification when sensor is disabled
func (s *Service) sendDisableNotification(zoneID, zoneName string, currentTemp, lastGoodTemp float64) {
	var prefix string
	if zoneID == "buffer_tank" {
		prefix = "[System Shutdown]"
	} else {
		prefix = fmt.Sprintf("[%s Zone Disabled]", zoneName)
	}

	message := fmt.Sprintf("%s %s: %.1f°F (%d anomalies, last good: %.1f°F)",
		prefix, zoneName, currentTemp, s.maxAnomalies, lastGoodTemp)

	err := s.notifier.Send("HVAC Sensor Failure", message)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send sensor failure notification")
	}
}

// sendRecoveryNotification sends notification when sensor recovers
func (s *Service) sendRecoveryNotification(zoneID string, temp float64) {
	zoneName := s.getZoneName(zoneID)
	message := fmt.Sprintf("[%s Zone Recovered] %s: %.1f°F (%d consecutive good readings)",
		zoneName, zoneName, temp, s.maxAnomalies)

	err := s.notifier.Send("HVAC Sensor Recovery", message)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send sensor recovery notification")
	}
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
