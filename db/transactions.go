package db

import (
	"database/sql"
	"fmt"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
)

// StartTransaction starts a new database transaction.
func StartTransaction(db *sql.DB) (*sql.Tx, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	return tx, nil
}

// CommitTransaction commits the given transaction.
func CommitTransaction(tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// RollbackTransaction rolls back the given transaction.
func RollbackTransaction(tx *sql.Tx) {
	tx.Rollback()
}

// SetSystemModeWithTx sets the system mode within a transaction.
func SetSystemModeWithTx(tx *sql.Tx, mode model.SystemMode) error {
	_, err := tx.Exec(`UPDATE system SET system_mode = ? WHERE id = 1`, string(mode))
	if err != nil {
		return fmt.Errorf("failed to set system mode: %w", err)
	}
	return nil
}

// UpdateZoneSetpointWithTx updates a zone's setpoint within a transaction.
func UpdateZoneSetpointWithTx(tx *sql.Tx, id string, setpoint float64) error {
	_, err := tx.Exec(`UPDATE zones SET setpoint = ? WHERE id = ?`, setpoint, id)
	if err != nil {
		return fmt.Errorf("failed to update zone setpoint for %s: %w", id, err)
	}
	return nil
}

// UpdateZoneModeWithTx updates a zone's mode within a transaction.
func UpdateZoneModeWithTx(tx *sql.Tx, id string, mode model.SystemMode) error {
	_, err := tx.Exec(`UPDATE zones SET mode = ? WHERE id = ?`, string(mode), id)
	if err != nil {
		return fmt.Errorf("failed to update zone mode for %s: %w", id, err)
	}
	return nil
}

// UpdateDeviceOnlineStatusWithTx updates a device's online status within a transaction.
func UpdateDeviceOnlineStatusWithTx(tx *sql.Tx, id int, online bool) error {
	_, err := tx.Exec(`UPDATE devices SET online = ? WHERE id = ?`, online, id)
	if err != nil {
		return fmt.Errorf("failed to update device online status for %d: %w", id, err)
	}
	return nil
}
