package db

import (
	"database/sql"

	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

func SetSystemModeCLI(dbPath, mode string) error {
	dbConn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer dbConn.Close()
	return UpdateSystemMode(dbConn, model.SystemMode(mode))
}

func SetZoneModeCLI(dbPath, zoneID, mode string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return UpdateZoneMode(db, zoneID, model.SystemMode(mode))
}

func SetZoneSetpointCLI(dbPath, zoneID string, setpoint float64) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return UpdateZoneSetpoint(db, zoneID, setpoint)
}
