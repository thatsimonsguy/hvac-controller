package startup

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func WriteStartupScript(dbConn *sql.DB) error {
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

	heatPumps, err := db.GetHeatPumps(dbConn)
	if err != nil {
		return err
	}
	systemMode, err := db.GetSystemMode(dbConn)
	if err != nil {
		return err
	}
	for _, hp := range heatPumps {
		write(hp.Name, hp.Pin, false)

		modeActive := contains(hp.Device.ActiveModes, string(systemMode)) &&
			systemMode == model.ModeCooling && hp.Device.Online
		write(hp.Name+".mode_pin", hp.ModePin, modeActive)
	}
	airHandlers, err := db.GetAirHandlers(dbConn)
	if err != nil {
		return err
	}
	for _, ah := range airHandlers {
		write(ah.Name, ah.Pin, false)
		write(ah.Name+".circ_pump", ah.CircPumpPin, false)
	}
	boilers, err := db.GetBoilers(dbConn)
	if err != nil {
		return err
	}
	for _, b := range boilers {
		write(b.Name, b.Pin, false)
	}
	radiantLoops, err := db.GetRadiantLoops(dbConn)
	if err != nil {
		return err
	}
	for _, rf := range radiantLoops {
		write(rf.Name, rf.Pin, false)
	}
	mainPower, err := db.GetMainPowerPin(dbConn)
	if err != nil {
		return err
	}
	write("main_power", mainPower, false)

	contents := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(env.Cfg.BootScriptFilePath, []byte(contents), 0755)
}

func InstallStartupService() error {
	unitContents := fmt.Sprintf(`[Unit]
Description=Configure GPIO pins at boot
After=network.target

[Service]
Type=oneshot
Environment=PATH=/usr/local/bin:/usr/bin:/bin
ExecStart=%s
RemainAfterExit=true

[Install]
WantedBy=multi-user.target
`, env.Cfg.BootScriptFilePath)

	return os.WriteFile(env.Cfg.OSServicePath, []byte(unitContents), 0644)
}

func RunStartupScript() error {
	cmd := exec.Command("/bin/bash", env.Cfg.BootScriptFilePath)
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

// Add to startup.go:
func InstallHVACService() error {
	gpioUnitName := filepath.Base(env.Cfg.OSServicePath)

	// Consider adding these to your config, too:
	user := "oebus"
	workdir := "/home/oebus/hvac-controller"
	execCmd := "go run ./cmd/hvac-controller/main.go"

	unit := fmt.Sprintf(`[Unit]
Description=HVAC Controller main service
After=%s
Requires=%s

[Service]
Type=simple
User=%s
WorkingDirectory=%s
Environment=PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
ExecStart=/bin/bash -lc '%s'
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, gpioUnitName, gpioUnitName, user, workdir, execCmd)

	return os.WriteFile(env.Cfg.MainServicePath, []byte(unit), 0644)
}
