package db

import (
	"database/sql"
	"fmt"
	"time"

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

func UpdateSystemMode(db *sql.DB, mode model.SystemMode) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	_, err = tx.Exec(`UPDATE system SET system_mode = ? WHERE id = 1`, string(mode))
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("update system mode: %w", err)
	}
	return tx.Commit()
}

func UpdateZoneSetpoint(db *sql.DB, id string, setpoint float64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	_, err = tx.Exec(`UPDATE zones SET setpoint = ? WHERE id = ?`, setpoint, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("update zone setpoint: %w", err)
	}
	return tx.Commit()
}

func UpdateZoneMode(db *sql.DB, id string, mode model.SystemMode) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	_, err = tx.Exec(`UPDATE zones SET mode = ? WHERE id = ?`, string(mode), id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("update zone mode: %w", err)
	}
	return tx.Commit()
}

func UpdateDeviceOnlineStatus(db *sql.DB, id int, online bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	_, err = tx.Exec(`UPDATE devices SET online = ? WHERE id = ?`, online, id)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("update device online status: %w", err)
	}
	return tx.Commit()
}

func SwapPrimaryHeatPump(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}

	// 1. Get current primary heat pump
	row := tx.QueryRow(`SELECT id FROM devices WHERE device_type = 'heat_pump' AND is_primary = true`)
	var currentID int
	err = row.Scan(&currentID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find current primary: %w", err)
	}

	// 2. Get next (non-primary) heat pump
	row = tx.QueryRow(`SELECT id FROM devices WHERE device_type = 'heat_pump' AND id != ? ORDER BY last_rotated ASC LIMIT 1`, currentID)
	var newID int
	err = row.Scan(&newID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find new primary: %w", err)
	}

	// 3. Unset current primary and update last_rotated
	_, err = tx.Exec(`UPDATE devices SET is_primary = false, last_rotated = ? WHERE id = ?`, time.Now().Format(time.RFC3339), currentID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("unset current primary: %w", err)
	}

	// 4. Set new primary and update last_rotated
	_, err = tx.Exec(`UPDATE devices SET is_primary = true, last_rotated = ? WHERE id = ?`, time.Now().Format(time.RFC3339), newID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("set new primary: %w", err)
	}

	return tx.Commit()
}
