package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/thatsimonsguy/hvac-controller/db"
)

func main() {
	DebugCLI()
}

func DebugCLI() {
	var dbPath, command, zoneID, mode, deviceName string
	var setpoint float64
	flag.StringVar(&dbPath, "db", "data/hvac.db", "Path to the SQLite database file")
	flag.StringVar(&command, "cmd", "", "Command to run: set-system-mode, set-zone-mode, set-zone-setpoint, reset-air-handler-timestamps, set-device-offline, set-device-online")
	flag.StringVar(&zoneID, "zone", "", "Zone ID for zone commands")
	flag.StringVar(&mode, "mode", "", "Mode for system or zone")
	flag.StringVar(&deviceName, "device", "", "Device name for device commands (e.g. main_floor_air_handler)")
	flag.Float64Var(&setpoint, "setpoint", 0, "Setpoint value for zone")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help || command == "" {
		fmt.Println("\nUsage of hvac-debug:")
		fmt.Println("  -db string\tPath to the SQLite database file (default 'hvac.db')")
		fmt.Println("  -cmd string\tCommand to run: set-system-mode, set-zone-mode, set-zone-setpoint, reset-air-handler-timestamps, set-device-offline, set-device-online")
		fmt.Println("  -zone string\tZone ID for zone commands")
		fmt.Println("  -mode string\tMode for system or zone")
		fmt.Println("  -device string\tDevice name for device commands (e.g. main_floor_air_handler)")
		fmt.Println("  -setpoint float\tSetpoint value for zone")
		fmt.Println("  -help\tShow this help message")
		fmt.Println("\nCommands:")
		fmt.Println("  reset-air-handler-timestamps\tReset basement and main_floor air handler timestamps to 13+ hours ago (triggers recirculation)")
		fmt.Println("  set-device-offline\tPut a device into maintenance mode (online=false) so controllers never energize it. Requires -device")
		fmt.Println("  set-device-online\tReturn a device to service (online=true). Requires -device")
		os.Exit(0)
	}

	var err error
	switch command {
	case "set-system-mode":
		err = db.SetSystemModeCLI(dbPath, mode)
	case "set-zone-mode":
		if zoneID == "" {
			fmt.Println("Error: zone ID is required")
			os.Exit(1)
		}
		err = db.SetZoneModeCLI(dbPath, zoneID, mode)
	case "set-zone-setpoint":
		if zoneID == "" {
			fmt.Println("Error: zone ID is required")
			os.Exit(1)
		}
		err = db.SetZoneSetpointCLI(dbPath, zoneID, setpoint)
	case "reset-air-handler-timestamps":
		err = db.ResetAirHandlerTimestampsCLI(dbPath)
	case "set-device-offline":
		if deviceName == "" {
			fmt.Println("Error: device name is required")
			os.Exit(1)
		}
		err = db.SetDeviceOnlineCLI(dbPath, deviceName, false)
	case "set-device-online":
		if deviceName == "" {
			fmt.Println("Error: device name is required")
			os.Exit(1)
		}
		err = db.SetDeviceOnlineCLI(dbPath, deviceName, true)
	default:
		fmt.Println("Invalid command")
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Command %s failed: %v\n", command, err)
		os.Exit(1)
	}
	fmt.Printf("Command %s completed successfully\n", command)
}
