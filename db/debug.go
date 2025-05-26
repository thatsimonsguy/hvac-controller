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
	tx, err := StartTransaction(dbConn)
	if err != nil {
		return err
	}
	if err := SetSystemModeWithTx(tx, model.SystemMode(mode)); err != nil {
		RollbackTransaction(tx)
		return err
	}
	return CommitTransaction(tx)
}

func SetZoneModeCLI(dbPath, zoneID, mode string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := StartTransaction(db)
	if err != nil {
		return err
	}
	if err := UpdateZoneModeWithTx(tx, zoneID, model.SystemMode(mode)); err != nil {
		RollbackTransaction(tx)
		return err
	}
	return CommitTransaction(tx)
}

func SetZoneSetpointCLI(dbPath, zoneID string, setpoint float64) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := StartTransaction(db)
	if err != nil {
		return err
	}
	if err := UpdateZoneSetpointWithTx(tx, zoneID, setpoint); err != nil {
		RollbackTransaction(tx)
		return err
	}
	return CommitTransaction(tx)
}
