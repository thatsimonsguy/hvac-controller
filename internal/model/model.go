package model

type SystemMode string

const (
	ModeOff       SystemMode = "off"
	ModeHeating   SystemMode = "heating"
	ModeCooling   SystemMode = "cooling"
	ModeCirculate SystemMode = "circulate"
)

type Zone struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	Setpoint     float64   `json:"setpoint"`
	Capabilities []string  `json:"capabilities"` // e.g. ["heating", "cooling"]
}

type SystemState struct {
	SystemMode SystemMode `json:"system_mode"`
	Zones      []Zone     `json:"zones"`
}
