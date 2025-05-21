package env

import (
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

var (
	Cfg         *config.Config
	SystemState *state.SystemState
)
