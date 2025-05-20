package startup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/state"
)

func WriteStartupScript(cfg *config.Config, state *state.SystemState) error {
	var lines []string
	lines = append(lines, "#!/bin/bash", "", "# HVAC GPIO pin configuration at boot", "")

	write := func(label string, pin model.GPIOPin, active bool) {
		drive := "dl"
		if pin.ActiveHigh == active {
			drive = "dh"
		}
		lines = append(lines, fmt.Sprintf("# %s", label))
		lines = append(lines, fmt.Sprintf("pinctrl set %d op pn %s", pin.Number, drive))
		lines = append(lines, "")
	}

	for _, hp := range state.HeatPumps {
		write(hp.Name, hp.Pin, false)

		modeActive := contains(hp.Device.ActiveModes, string(state.SystemMode)) &&
			state.SystemMode == model.ModeCooling && hp.Device.Online
		write(hp.Name+".mode_pin", hp.ModePin, modeActive)
	}

	for _, ah := range state.AirHandlers {
		write(ah.Name, ah.Pin, false)
		write(ah.Name+".circ_pump", ah.CircPumpPin, false)
	}
	for _, b := range state.Boilers {
		write(b.Name, b.Pin, false)
	}
	for _, rf := range state.RadiantLoops {
		write(rf.Name, rf.Pin, false)
	}

	write("main_power", state.MainPowerPin, false)

	contents := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(cfg.BootScriptFilePath, []byte(contents), 0755)
}

func InstallStartupService(cfg *config.Config) error {
	unitContents := fmt.Sprintf(`[Unit]
Description=Configure GPIO pins at boot
After=network.target

[Service]
Type=oneshot
ExecStart=%s
RemainAfterExit=true

[Install]
WantedBy=multi-user.target
`, cfg.BootScriptFilePath)

	return os.WriteFile(cfg.OSServicePath, []byte(unitContents), 0644)
}

func RunStartupScript(scriptPath string) error {
	cmd := exec.Command("/bin/bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func contains(list []string, val string) bool {
	for _, s := range list {
		if s == val {
			return true
		}
	}
	return false
}
