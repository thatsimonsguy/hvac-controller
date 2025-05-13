package gpio

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
)

var (
	mu    sync.RWMutex
	state = make(map[int]bool)

	setFn  = defaultSet
	readFn = defaultRead
)

// Set energizes or de-energizes a GPIO pin (true = ON)
func Set(pin int, on bool) {
	setFn(pin, on)
}

// Read returns true if the GPIO pin is currently energized
func Read(pin int) bool {
	return readFn(pin)
}

// --- Default backend ---

func defaultSet(pin int, on bool) {
	mu.Lock()
	defer mu.Unlock()
	state[pin] = on
}

func defaultRead(pin int) bool {
	mu.RLock()
	defer mu.RUnlock()
	return state[pin]
}

func ValidateStartupPins(cfg config.Config) error {
	var violations []string

	for name, pinDef := range cfg.GPIO {
		actual := Read(pinDef.Pin)
		if actual != pinDef.SafeState {
			violations = append(violations,
				fmt.Sprintf("pin %d (gpio.%s) is %v but expected %v",
					pinDef.Pin, name, actual, pinDef.SafeState))
		}
	}

	if len(violations) > 0 {
		for _, v := range violations {
			log.Error().Msg(v)
		}
		return fmt.Errorf("unsafe GPIO pin states at startup")
	}

	log.Info().Msg("All GPIO pins match safe state config at startup")
	return nil
}

func GetPin(cfg config.Config, key string) int {
	pinDef, ok := cfg.GPIO[key]
	if !ok {
		panic(fmt.Sprintf("GPIO config missing for key: %s", key))
	}
	return pinDef.Pin
}

// --- Testing hooks ---

// MockGPIO overrides the Set and Read logic for tests
func MockGPIO(set func(int, bool), read func(int) bool) {
	setFn = set
	readFn = read
}

// ResetGPIO resets the internal GPIO state and restores default behavior
func ResetGPIO() {
	mu.Lock()
	defer mu.Unlock()

	state = make(map[int]bool)
	setFn = defaultSet
	readFn = defaultRead
}
