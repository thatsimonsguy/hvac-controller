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
	var dbPath, command, zoneID, mode string
	var setpoint float64
	flag.StringVar(&dbPath, "db", "data/hvac.db", "Path to the SQLite database file")
	flag.StringVar(&command, "cmd", "", "Command to run: set-system-mode, set-zone-mode, set-zone-setpoint")
	flag.StringVar(&zoneID, "zone", "", "Zone ID for zone commands")
	flag.StringVar(&mode, "mode", "", "Mode for system or zone")
	flag.Float64Var(&setpoint, "setpoint", 0, "Setpoint value for zone")
	help := flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help || command == "" {
		fmt.Println("\nUsage of hvac-debug:")
		fmt.Println("  -db string\tPath to the SQLite database file (default 'hvac.db')")
		fmt.Println("  -cmd string\tCommand to run: set-system-mode, set-zone-mode, set-zone-setpoint")
		fmt.Println("  -zone string\tZone ID for zone commands")
		fmt.Println("  -mode string\tMode for system or zone")
		fmt.Println("  -setpoint float\tSetpoint value for zone")
		fmt.Println("  -help\tShow this help message")
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
