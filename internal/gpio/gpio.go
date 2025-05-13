package gpio

import (
	"sync"
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
