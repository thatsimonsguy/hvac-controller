package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

func TestRecirculationOverrideFunctions(t *testing.T) {
	// Create in-memory test database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create system table
	_, err = db.Exec(`CREATE TABLE system (
		id INTEGER PRIMARY KEY CHECK(id=1),
		system_mode TEXT NOT NULL,
		override_active BOOLEAN DEFAULT FALSE,
		recirculation_active BOOLEAN DEFAULT FALSE,
		recirculation_started_at TEXT
	)`)
	require.NoError(t, err)

	// Insert system record
	_, err = db.Exec(`INSERT INTO system (id, system_mode) VALUES (1, 'off')`)
	require.NoError(t, err)

	t.Run("initially inactive", func(t *testing.T) {
		active, startedAt, err := GetRecirculationStatus(db)
		require.NoError(t, err)
		assert.False(t, active)
		assert.True(t, startedAt.IsZero())
	})

	t.Run("set active", func(t *testing.T) {
		now := time.Now()
		err := SetRecirculationActive(db, true, now)
		require.NoError(t, err)

		active, startedAt, err := GetRecirculationStatus(db)
		require.NoError(t, err)
		assert.True(t, active)
		assert.WithinDuration(t, now, startedAt, time.Second)
	})

	t.Run("clear active", func(t *testing.T) {
		err := SetRecirculationActive(db, false, time.Time{})
		require.NoError(t, err)

		active, startedAt, err := GetRecirculationStatus(db)
		require.NoError(t, err)
		assert.False(t, active)
		assert.True(t, startedAt.IsZero())
	})
}