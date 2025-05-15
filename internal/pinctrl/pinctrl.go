package pinctrl

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type PinState struct {
	Pin     int
	Mode    string // e.g., "ip", "op", "no"
	Pull    string // e.g., "pu", "pd", "pn"
	Drive   string // e.g., "dh", "dl", ""
	Level   string // e.g., "hi", "lo", "--"
	Comment string // full comment, typically includes // GPIO#
}

var pinLineRegex = regexp.MustCompile(`^\s*(\d+):\s+(\S+)\s+(.*?)\s+\|\s+(\S+)\s+//\s+(.*GPIO(\d+).*)$`)

// ReadAllPins returns the parsed result of `pinctrl get`, mapping each GPIO pin number to its PinState
func ReadAllPins() (map[int]PinState, error) {
	cmd := exec.Command("pinctrl", "get")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute pinctrl get: %w", err)
	}

	result := make(map[int]PinState)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		matches := pinLineRegex.FindStringSubmatch(line)
		if len(matches) != 7 {
			continue
		}

		index, _ := strconv.Atoi(matches[1])
		state := PinState{
			Pin:     index,
			Mode:    matches[2],
			Level:   matches[4],
			Comment: matches[5],
		}

		opts := strings.Fields(matches[3])
		for _, opt := range opts {
			if state.Pull == "" && (opt == "pu" || opt == "pd" || opt == "pn") {
				state.Pull = opt
			} else if state.Drive == "" && (opt == "dh" || opt == "dl") {
				state.Drive = opt
			}
		}

		result[state.Pin] = state
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning pinctrl output: %w", err)
	}

	return result, nil
}

// ReadPin returns the PinState for a specific GPIO pin
func ReadPin(pin int) (*PinState, error) {
	all, err := ReadAllPins()
	if err != nil {
		return nil, err
	}
	state, ok := all[pin]
	if !ok {
		return nil, fmt.Errorf("pin %d not found in pinctrl output", pin)
	}
	return &state, nil
}

// ReadLevel performs a fast read of the logic level of a pin using `pinctrl lev <pin>`
func ReadLevel(pin int) (bool, error) {
	cmd := exec.Command("pinctrl", "lev", fmt.Sprint(pin))
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to read level for pin %d: %w", pin, err)
	}
	trimmed := strings.TrimSpace(string(out))
	switch trimmed {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected output from pinctrl lev: %q", trimmed)
	}
}

// SetPin applies one or more pinctrl set options to the specified GPIO pin
// Example: SetPin(10, "op", "pn", "dh") sets pin 10 as output, no pull, drive high
func SetPin(pin int, opts ...string) error {
	args := []string{"set", fmt.Sprint(pin)}
	args = append(args, opts...)
	cmd := exec.Command("pinctrl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pinctrl set failed: %s (output: %s)", err, string(out))
	}
	return nil
}
