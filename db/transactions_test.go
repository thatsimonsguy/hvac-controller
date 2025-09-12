package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/mattn/go-sqlite3"
)

func TestRecirculationMigration(t *testing.T) {
	// Create in-memory test database without recirculation columns (simulates old DB)
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create old system table without recirculation columns
	_, err = db.Exec(`CREATE TABLE system (
		id INTEGER PRIMARY KEY CHECK(id=1),
		system_mode TEXT NOT NULL,
		override_active BOOLEAN DEFAULT FALSE,
		prior_system_mode TEXT DEFAULT NULL
	)`)
	require.NoError(t, err)

	// Insert system record
	_, err = db.Exec(`INSERT INTO system (id, system_mode) VALUES (1, 'off')`)
	require.NoError(t, err)

	// Verify columns don't exist initially
	rows, err := db.Query("PRAGMA table_info(system)")
	require.NoError(t, err)
	defer rows.Close()
	
	hasRecircActive := false
	hasRecircStarted := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull bool
		var defaultValue *string
		var pk int
		
		err = rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		require.NoError(t, err)
		
		if name == "recirculation_active" {
			hasRecircActive = true
		}
		if name == "recirculation_started_at" {
			hasRecircStarted = true
		}
	}
	
	assert.False(t, hasRecircActive, "recirculation_active should not exist before migration")
	assert.False(t, hasRecircStarted, "recirculation_started_at should not exist before migration")

	// Simulate applying migrations by manually adding columns (since we can't call ApplyMigrations with in-memory DB)
	_, err = db.Exec("ALTER TABLE system ADD COLUMN recirculation_active BOOLEAN DEFAULT FALSE")
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE system ADD COLUMN recirculation_started_at TEXT")
	require.NoError(t, err)

	// Now test that the functions work with the migrated schema
	active, startedAt, err := GetRecirculationStatus(db)
	require.NoError(t, err)
	assert.False(t, active)
	assert.True(t, startedAt.IsZero())

	// Test setting active
	now := time.Now()
	err = SetRecirculationActive(db, true, now)
	require.NoError(t, err)

	active, startedAt, err = GetRecirculationStatus(db)
	require.NoError(t, err)
	assert.True(t, active)
	assert.WithinDuration(t, now, startedAt, time.Second)
}

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