package pinctrl

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestParseGetAllOutput(t *testing.T) {
	sample := `
 0: ip    pu | hi // ID_SDA/GPIO0 = input
 1: ip    pu | hi // ID_SCL/GPIO1 = input
 2: no    pu | -- // GPIO2 = none
 4: ip    pn | lo // GPIO4 = input
 5: op dh pu | hi // GPIO5 = output
 6: op dh pu | hi // GPIO6 = output
12: op dh pd | hi // GPIO12 = output
13: op dh pd | hi // GPIO13 = output
26: op dl pn | lo // GPIO26 = output
`

	reader := strings.NewReader(sample)
	states := parseGetOutput(reader)

	if len(states) != 9 {
		t.Fatalf("expected 9 pins parsed, got %d", len(states))
	}

	if ps := states[5]; ps.Level != "hi" || ps.Mode != "op" || ps.Pull != "pu" || ps.Drive != "dh" {
		t.Errorf("GPIO5 parsed incorrectly: %+v", ps)
	}
	if ps := states[2]; ps.Level != "--" || ps.Mode != "no" {
		t.Errorf("GPIO2 parsed incorrectly: %+v", ps)
	}
	if ps := states[26]; ps.Level != "lo" || ps.Mode != "op" || ps.Pull != "pn" || ps.Drive != "dl" {
		t.Errorf("GPIO26 parsed incorrectly: %+v", ps)
	}
}

func TestParseGetSinglePinOutput(t *testing.T) {
	line := `25: op dl pd | lo // GPIO25 = output`
	reader := strings.NewReader(line)
	states := parseGetOutput(reader)

	ps, ok := states[25]
	if !ok {
		t.Fatalf("GPIO25 not parsed")
	}
	if ps.Mode != "op" || ps.Pull != "pd" || ps.Drive != "dl" || ps.Level != "lo" {
		t.Errorf("unexpected values for GPIO25: %+v", ps)
	}
}

func TestParseLevelOutput(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0", false},
		{"1", true},
		{"\n1\n", true},
		{"\n0\n", false},
	}
	for _, tc := range tests {
		result, err := parseLevelOutput(tc.input)
		if err != nil {
			t.Errorf("error parsing level output %q: %v", tc.input, err)
		}
		if result != tc.expected {
			t.Errorf("expected %v for input %q, got %v", tc.expected, tc.input, result)
		}
	}
}

// --- Helpers used internally for testing ---

func parseLevelOutput(output string) (bool, error) {
	trimmed := strings.TrimSpace(output)
	switch trimmed {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected level output: %q", trimmed)
	}
}

func parseGetOutput(r *strings.Reader) map[int]PinState {
	scanner := bufio.NewScanner(r)
	results := make(map[int]PinState)

	for scanner.Scan() {
		line := scanner.Text()
		matches := pinLineRegex.FindStringSubmatch(line)
		if len(matches) == 7 {
			pin := mustAtoi(matches[1])
			ps := PinState{
				Pin:     pin,
				Mode:    matches[2],
				Level:   matches[4],
				Comment: matches[5],
			}
			opts := strings.Fields(matches[3])
			for _, opt := range opts {
				if ps.Pull == "" && (opt == "pu" || opt == "pd" || opt == "pn") {
					ps.Pull = opt
				} else if ps.Drive == "" && (opt == "dh" || opt == "dl") {
					ps.Drive = opt
				}
			}
			results[pin] = ps
		}
	}
	return results
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
