package controller

import (
	"testing"
	"time"

	"github.com/thatsimonsguy/hvac-controller/internal/gpio"
)

func TestDevice_CanTurnOnAndOff(t *testing.T) {
	defer gpio.ResetGPIO()

	var setCalls []string
	gpio.MockGPIO(func(pin int, on bool) {
		if on {
			setCalls = append(setCalls, "ON")
		} else {
			setCalls = append(setCalls, "OFF")
		}
	}, func(pin int) bool {
		return len(setCalls) > 0 && setCalls[len(setCalls)-1] == "ON"
	})

	now := time.Now()
	dev := &Device{
		Name:        "test_pump",
		Pin:         5,
		LastChanged: now.Add(-10 * time.Minute), // long enough ago
		MinOn:       5 * time.Minute,
		MinOff:      5 * time.Minute,
	}

	if !dev.CanTurnOn(now) {
		t.Fatal("expected device to be turn-on eligible")
	}

	dev.TurnOn(now)
	if !dev.IsOn() {
		t.Fatal("expected device to be ON after TurnOn")
	}

	// simulate too-soon off attempt
	tooSoon := now.Add(2 * time.Minute)
	if dev.CanTurnOff(tooSoon) {
		t.Fatal("expected device to be too new to turn off")
	}

	// simulate valid off
	later := now.Add(10 * time.Minute)
	if !dev.CanTurnOff(later) {
		t.Fatal("expected device to be turn-off eligible")
	}

	dev.TurnOff(later)
	if dev.IsOn() {
		t.Fatal("expected device to be OFF after TurnOff")
	}
}
