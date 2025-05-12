package gpio

import (
	"sync"
)

var (
	stateMu sync.RWMutex
	state   = make(map[int]bool) // pin number → current state
)

// Set sets the given pin to high (true) or low (false)
func Set(pin int, on bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	state[pin] = on
}

// Read returns true if the pin is energized
func Read(pin int) bool {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return state[pin]
}
