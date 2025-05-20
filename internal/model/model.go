package model

import "time"

type SystemMode string

const (
	ModeOff       SystemMode = "off"
	ModeHeating   SystemMode = "heating"
	ModeCooling   SystemMode = "cooling"
	ModeCirculate SystemMode = "circulate"
)

type Zone struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Setpoint     float64  `json:"setpoint"`
	Capabilities []string `json:"capabilities"` // e.g. ["heating", "cooling"]
	Sensor       Sensor   `json:"sensor"`
}

type Device struct {
	Name        string
	Pin         GPIOPin
	MinOn       time.Duration // TODO: populate from config
	MinOff      time.Duration // TODO: populate from config
	Online      bool
	LastChanged time.Time
	ActiveModes []string
}

type HeatPump struct {
	Device
	ModePin     GPIOPin
	IsPrimary   bool
	LastRotated time.Time
}

type Boiler struct {
	Device
}

type RadiantFloorLoop struct {
	Device
	Zone *Zone
}

type AirHandler struct {
	Device
	Zone        *Zone
	CircPumpPin GPIOPin
}

type GPIOPin struct {
	Number     int
	ActiveHigh bool
}

type Sensor struct {
	ID  string `json:"id"`
	Bus string `json:"bus"`
}
