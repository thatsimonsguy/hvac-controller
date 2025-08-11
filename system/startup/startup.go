package startup

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

type ServiceStatus struct {
	Exists  bool
	Enabled bool
	Active  bool
}

type ServicesStatus struct {
	GPIO ServiceStatus
	HVAC ServiceStatus
}

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

// isPermissionError checks if an error is related to permissions
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common permission error patterns
	errStr := strings.ToLower(err.Error())
	permissionKeywords := []string{
		"permission denied",
		"operation not permitted",
		"access denied",
		"insufficient privileges",
	}

	for _, keyword := range permissionKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	// Check for specific syscall permission errors
	if pathErr, ok := err.(*os.PathError); ok {
		if errno, ok := pathErr.Err.(syscall.Errno); ok {
			return errno == syscall.EACCES || errno == syscall.EPERM
		}
	}

	return false
}

// printSudoGuidance prints helpful guidance for running with sudo
func printSudoGuidance() {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  PERMISSION ERROR: Service creation requires elevated privileges")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("To create the required systemd services, please run:")
	fmt.Println()
	fmt.Println("  sudo ./hvac-controller")
	fmt.Println()
	fmt.Println("This will:")
	fmt.Println("  • Create the GPIO pin configuration service")
	fmt.Println("  • Create the HVAC controller service")
	fmt.Println("  • Enable both services to start on boot")
	fmt.Println()
	fmt.Println("After running once with sudo, you can run normally without sudo.")
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
}

// Add to startup.go:
func InstallHVACService() error {
	gpioUnitName := filepath.Base(env.Cfg.OSServicePath)

	// Consider adding these to your config, too:
	user := "oebus"
	workdir := "/home/oebus/hvac-controller"
	execCmd := "/home/oebus/hvac-controller/hvac-controller"

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

// CheckServicesStatus checks the existence and status of both services
func CheckServicesStatus() (ServicesStatus, error) {
	status := ServicesStatus{}

	// Check GPIO service
	gpioStatus, err := checkSingleService(env.Cfg.OSServicePath)
	if err != nil {
		return status, fmt.Errorf("error checking GPIO service: %w", err)
	}
	status.GPIO = gpioStatus

	// Check HVAC service
	hvacStatus, err := checkSingleService(env.Cfg.MainServicePath)
	if err != nil {
		return status, fmt.Errorf("error checking HVAC service: %w", err)
	}
	status.HVAC = hvacStatus

	return status, nil
}

// checkSingleService checks if a service file exists and its systemd status
func checkSingleService(servicePath string) (ServiceStatus, error) {
	status := ServiceStatus{}

	// Check if service file exists
	if _, err := os.Stat(servicePath); err == nil {
		status.Exists = true
	} else if !os.IsNotExist(err) {
		return status, err
	}

	if !status.Exists {
		return status, nil
	}

	// Get service name from path
	serviceName := filepath.Base(servicePath)

	// Check if service is enabled
	cmd := exec.Command("systemctl", "is-enabled", serviceName)
	if err := cmd.Run(); err == nil {
		status.Enabled = true
	}

	// Check if service is active
	cmd = exec.Command("systemctl", "is-active", serviceName)
	if err := cmd.Run(); err == nil {
		status.Active = true
	}

	return status, nil
}

// LogServicesStatus logs the current status of both services using zerolog
func LogServicesStatus() error {
	status, err := CheckServicesStatus()
	if err != nil {
		log.Error().Err(err).Msg("Failed to check services status")
		return err
	}

	gpioServiceName := filepath.Base(env.Cfg.OSServicePath)
	hvacServiceName := filepath.Base(env.Cfg.MainServicePath)

	log.Info().
		Str("service", gpioServiceName).
		Bool("exists", status.GPIO.Exists).
		Bool("enabled", status.GPIO.Enabled).
		Bool("active", status.GPIO.Active).
		Msg("GPIO service status")

	log.Info().
		Str("service", hvacServiceName).
		Bool("exists", status.HVAC.Exists).
		Bool("enabled", status.HVAC.Enabled).
		Bool("active", status.HVAC.Active).
		Msg("HVAC service status")

	return nil
}

// EnsureServicesReady checks service status and installs/enables services as needed
func EnsureServicesReady(dbConn *sql.DB) error {
	log.Info().Msg("Checking HVAC services status...")

	status, err := CheckServicesStatus()
	if err != nil {
		log.Error().Err(err).Msg("Failed to check services status")
		return err
	}

	var needsReload bool

	// Handle GPIO service
	if !status.GPIO.Exists {
		log.Info().Msg("GPIO service not found, installing...")

		// Write the startup script first
		if err := WriteStartupScript(dbConn); err != nil {
			log.Error().Err(err).Msg("Failed to write startup script")
			return err
		}

		// Install the GPIO service
		if err := InstallStartupService(); err != nil {
			if isPermissionError(err) {
				printSudoGuidance()
				return fmt.Errorf("service creation requires elevated privileges")
			}
			log.Error().Err(err).Msg("Failed to install GPIO service")
			return err
		}

		needsReload = true
		log.Info().Msg("GPIO service installed successfully")
	}

	// Handle HVAC service
	if !status.HVAC.Exists {
		log.Info().Msg("HVAC service not found, installing...")

		if err := InstallHVACService(); err != nil {
			if isPermissionError(err) {
				printSudoGuidance()
				return fmt.Errorf("service creation requires elevated privileges")
			}
			log.Error().Err(err).Msg("Failed to install HVAC service")
			return err
		}

		needsReload = true
		log.Info().Msg("HVAC service installed successfully")
	}

	// Reload systemd if we installed any new services
	if needsReload {
		log.Info().Msg("Reloading systemd daemon...")
		if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
			if isPermissionError(err) {
				printSudoGuidance()
				return fmt.Errorf("service management requires elevated privileges")
			}
			log.Error().Err(err).Msg("Failed to reload systemd daemon")
			return fmt.Errorf("failed to reload systemd daemon: %w", err)
		}
	}

	// Re-check status after potential installations
	status, err = CheckServicesStatus()
	if err != nil {
		return err
	}

	// Enable GPIO service if not enabled
	if status.GPIO.Exists && !status.GPIO.Enabled {
		gpioServiceName := filepath.Base(env.Cfg.OSServicePath)
		log.Info().Str("service", gpioServiceName).Msg("Enabling GPIO service...")

		if err := exec.Command("systemctl", "enable", gpioServiceName).Run(); err != nil {
			if isPermissionError(err) {
				printSudoGuidance()
				return fmt.Errorf("service management requires elevated privileges")
			}
			log.Error().Err(err).Str("service", gpioServiceName).Msg("Failed to enable GPIO service")
			return fmt.Errorf("failed to enable GPIO service: %w", err)
		}

		log.Info().Str("service", gpioServiceName).Msg("GPIO service enabled successfully")
	}

	// Enable HVAC service if not enabled
	if status.HVAC.Exists && !status.HVAC.Enabled {
		hvacServiceName := filepath.Base(env.Cfg.MainServicePath)
		log.Info().Str("service", hvacServiceName).Msg("Enabling HVAC service...")

		if err := exec.Command("systemctl", "enable", hvacServiceName).Run(); err != nil {
			if isPermissionError(err) {
				printSudoGuidance()
				return fmt.Errorf("service management requires elevated privileges")
			}
			log.Error().Err(err).Str("service", hvacServiceName).Msg("Failed to enable HVAC service")
			return fmt.Errorf("failed to enable HVAC service: %w", err)
		}

		log.Info().Str("service", hvacServiceName).Msg("HVAC service enabled successfully")
	}

	// Log final status
	if err := LogServicesStatus(); err != nil {
		log.Warn().Err(err).Msg("Failed to log final services status")
	}

	log.Info().Msg("Services ready - all required services are installed and enabled")
	return nil
}
