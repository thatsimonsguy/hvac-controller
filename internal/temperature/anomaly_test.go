package temperature

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock notification sender
type MockNotifier struct {
	calls []string
}

func (m *MockNotifier) Send(title, message string) error {
	fullMsg := title + ": " + message
	m.calls = append(m.calls, fullMsg)
	return nil
}

// Mock shutdown handler
type MockShutdown struct {
	shutdownCalled bool
}

func (m *MockShutdown) Shutdown() {
	m.shutdownCalled = true
}

// Test scenario structure
type TestScenario struct {
	name                  string
	sensorID              string
	sensorZone            string
	readingSequence       []float64
	expectedAccepted      []bool
	expectedLastGood      float64
	expectedAnomalyCount  int
	expectedDisabled      bool
	expectedShutdown      bool
	expectedNotification  string
	expectedNotifications []string
}

// Test helper to run scenarios
func runScenario(t *testing.T, scenario TestScenario) {
	mockNotifier := &MockNotifier{}
	mockShutdown := &MockShutdown{}

	deps := &TestDeps{
		Notifier:  mockNotifier,
		Shutdowner: mockShutdown,
	}

	service := NewServiceForTest(nil, 30, deps)

	// Process each reading
	for i, temp := range scenario.readingSequence {
		accepted := service.processReading(
			scenario.sensorID,
			scenario.sensorZone,
			temp,
			time.Now().Add(time.Duration(i*30)*time.Second),
		)

		assert.Equal(t, scenario.expectedAccepted[i], accepted,
			"Reading %d (%.1f°F) acceptance mismatch", i, temp)
	}

	// Verify final state
	history := service.history[scenario.sensorID]
	assert.NotNil(t, history, "History should exist for sensor")
	assert.InDelta(t, scenario.expectedLastGood, history.LastGoodReading.Temperature, 0.1,
		"Last good temperature mismatch")
	assert.Equal(t, scenario.expectedAnomalyCount, history.AnomalyCount,
		"Anomaly count mismatch")
	assert.Equal(t, scenario.expectedDisabled, history.Disabled,
		"Disabled state mismatch")
	assert.Equal(t, scenario.expectedShutdown, mockShutdown.shutdownCalled,
		"Shutdown state mismatch")

	// Verify notifications
	if scenario.expectedNotification != "" {
		found := false
		for _, call := range mockNotifier.calls {
			if call == scenario.expectedNotification {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected notification not found: %s\nGot: %v",
			scenario.expectedNotification, mockNotifier.calls)
	}

	if len(scenario.expectedNotifications) > 0 {
		assert.Equal(t, len(scenario.expectedNotifications), len(mockNotifier.calls),
			"Notification count mismatch")
		for i, expected := range scenario.expectedNotifications {
			assert.Equal(t, expected, mockNotifier.calls[i],
				"Notification %d mismatch", i)
		}
	}
}

func TestNormalReadings(t *testing.T) {
	scenario := TestScenario{
		name:             "Normal operation with gradual changes",
		sensorID:         "main_floor_sensor",
		sensorZone:       "main_floor",
		readingSequence:  []float64{70.0, 71.0, 72.0, 71.5, 72.0, 71.0, 72.5},
		expectedAccepted: []bool{true, true, true, true, true, true, true},
		expectedLastGood: 72.5,
		expectedAnomalyCount: 0,
		expectedDisabled: false,
		expectedShutdown: false,
	}
	runScenario(t, scenario)
}

func TestBootstrapBogusFirstReading(t *testing.T) {
	scenario := TestScenario{
		name:       "Bootstrap phase with bogus first reading",
		sensorID:   "basement_sensor",
		sensorZone: "basement",
		readingSequence: []float64{45.0, 67.0, 67.0, 68.0, 67.0, 66.0, 67.0},
		expectedAccepted: []bool{
			true,  // Accept during bootstrap (reading 1/6)
			true,  // Accept during bootstrap (reading 2/6)
			true,  // Accept during bootstrap (reading 3/6)
			true,  // Accept during bootstrap (reading 4/6)
			true,  // Accept during bootstrap (reading 5/6)
			true,  // Accept (reading 6/6 - NOW analyze history)
			true,  // Continue normally
		},
		expectedLastGood:     67.0,
		expectedAnomalyCount: 1,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestBootstrapBogusLastReading(t *testing.T) {
	scenario := TestScenario{
		name:       "Bootstrap phase with bogus last reading",
		sensorID:   "main_floor_sensor",
		sensorZone: "main_floor",
		readingSequence: []float64{67.0, 68.0, 68.0, 68.0, 67.0, 45.0, 46.0},
		expectedAccepted: []bool{
			true,  // Bootstrap 1/6
			true,  // Bootstrap 2/6
			true,  // Bootstrap 3/6
			true,  // Bootstrap 4/6
			true,  // Bootstrap 5/6
			true,  // Bootstrap 6/6 - analyze: 45 is anomaly, count=1
			false, // 46 is anomaly (count=2), reject
		},
		expectedLastGood:     67.0,
		expectedAnomalyCount: 2,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestZoneSensorDisable(t *testing.T) {
	scenario := TestScenario{
		name:       "Zone sensor failure after 6 consecutive anomalies",
		sensorID:   "main_floor_sensor",
		sensorZone: "main_floor",
		readingSequence: []float64{
			// Bootstrap phase
			70.0, 71.0, 70.5, 71.0, 70.0, 71.5,
			// Anomaly phase
			45.0, 40.0, 38.0, 35.0, 32.0, 30.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, false, false, false, false, false,
		},
		expectedLastGood:         71.5,
		expectedAnomalyCount:     6,
		expectedDisabled:         true,
		expectedShutdown:         false,
		expectedNotification:     "HVAC Sensor Failure: [Main Floor Zone Disabled] Main Floor: 30.0°F (6 anomalies, last good: 71.5°F)",
	}
	runScenario(t, scenario)
}

func TestBufferTankShutdown(t *testing.T) {
	scenario := TestScenario{
		name:       "Buffer tank failure triggers system shutdown",
		sensorID:   "buffer_tank",
		sensorZone: "buffer_tank",
		readingSequence: []float64{
			// Bootstrap
			105.0, 106.0, 105.5, 106.0, 105.0, 106.5,
			// Anomalies
			85.0, 80.0, 75.0, 70.0, 65.0, 60.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, false, false, false, false, false,
		},
		expectedLastGood:         106.5,
		expectedAnomalyCount:     6,
		expectedDisabled:         true,
		expectedShutdown:         true,
		expectedNotification:     "HVAC Sensor Failure: [System Shutdown] Buffer Tank: 60.0°F (6 anomalies, last good: 106.5°F)",
	}
	runScenario(t, scenario)
}

func TestSmartRecoveryLegitimateDrop(t *testing.T) {
	scenario := TestScenario{
		name:       "Smart recovery detects legitimate temperature change",
		sensorID:   "main_floor_sensor",
		sensorZone: "main_floor",
		readingSequence: []float64{
			// Bootstrap at 70°F
			70.0, 71.0, 70.0, 71.0, 70.5, 71.0,
			// Windows open, gradual drop
			65.0, 60.0, 58.0, 56.0, 55.0, 54.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, true, true, true, true, true,
		},
		expectedLastGood:     54.0,
		expectedAnomalyCount: 0,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestRecoveryStableAtNewLevel(t *testing.T) {
	scenario := TestScenario{
		name:       "Recovery when readings stabilize at new level",
		sensorID:   "garage_sensor",
		sensorZone: "garage",
		readingSequence: []float64{
			// Bootstrap
			55.0, 56.0, 55.0, 56.0, 55.5, 56.0,
			// Sudden drop (30° - exceeds garage threshold of 25°)
			26.0, 25.0, 26.0, 25.5, 26.0, 25.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, true, true, true, true, true,
		},
		expectedLastGood:     25.0,
		expectedAnomalyCount: 0,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestGarageLargeSwingsAllowed(t *testing.T) {
	scenario := TestScenario{
		name:       "Garage allows large temperature swings",
		sensorID:   "garage_sensor",
		sensorZone: "garage",
		readingSequence: []float64{
			// Bootstrap
			55.0, 56.0, 55.0, 56.0, 55.5, 56.0,
			// Large swings within garage threshold (25°)
			40.0, 38.0, 60.0, 58.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			true, true, true, true,
		},
		expectedLastGood:     58.0,
		expectedAnomalyCount: 0,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestAnomalyCounterReset(t *testing.T) {
	scenario := TestScenario{
		name:       "Anomaly counter resets on good reading",
		sensorID:   "basement_sensor",
		sensorZone: "basement",
		readingSequence: []float64{
			// Bootstrap
			70.0, 71.0, 70.0, 71.0, 70.5, 71.0,
			// Anomalies
			45.0, 40.0, 71.5, 35.0, 72.0,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, false, true, false, true,
		},
		expectedLastGood:     72.0,
		expectedAnomalyCount: 0,
		expectedDisabled:     false,
		expectedShutdown:     false,
	}
	runScenario(t, scenario)
}

func TestRecoveryAfterDisable(t *testing.T) {
	scenario := TestScenario{
		name:       "Sensor recovers and re-enables after 6 good readings",
		sensorID:   "main_floor_sensor",
		sensorZone: "main_floor",
		readingSequence: []float64{
			// Bootstrap
			70.0, 71.0, 70.0, 71.0, 70.5, 71.0,
			// Fail sensor (6 anomalies)
			45.0, 40.0, 38.0, 35.0, 32.0, 30.0,
			// Recovery (6 consecutive good readings)
			70.0, 71.0, 70.5, 71.0, 70.0, 71.5,
		},
		expectedAccepted: []bool{
			true, true, true, true, true, true,
			false, false, false, false, false, false,
			true, true, true, true, true, true,
		},
		expectedLastGood:     71.5,
		expectedAnomalyCount: 0,
		expectedDisabled:     false,
		expectedShutdown:     false,
		expectedNotifications: []string{
			"HVAC Sensor Failure: [Main Floor Zone Disabled] Main Floor: 30.0°F (6 anomalies, last good: 71.0°F)",
			"HVAC Sensor Recovery: [Main Floor Zone Recovered] Main Floor: 71.5°F (6 consecutive good readings)",
		},
	}
	runScenario(t, scenario)
}
