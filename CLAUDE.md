# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a custom HVAC controller written in Go that manages a multi-zone heating/cooling system on a Raspberry Pi 5. The system controls heat pumps, air handlers, boilers, and radiant floor loops using GPIO-based relay control via `pinctrl`. State is persisted in SQLite with JSON configuration.

## Development Commands

### Building and Running
```bash
# Build the main controller
go build ./cmd/hvac-controller

# Run the main controller
go run ./cmd/hvac-controller/main.go

# Run the debug version  
go run ./cmd/debug/main.go

# Install dependencies
go mod tidy
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/controllers/buffercontroller
go test ./internal/controllers/zonecontroller

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -run TestSpecificFunction ./internal/package
```

### Code Quality
```bash
# Format code
go fmt ./...

# Vet code for issues
go vet ./...

# Clean build cache
go clean
```

## Architecture

### Core Components

- **Zone Controller** (`internal/controllers/zonecontroller/`): Manages individual zone temperature control, handles heating/cooling decisions per zone
- **Buffer Controller** (`internal/controllers/buffercontroller/`): Manages buffer tank temperature and coordinates heat sources (heat pumps, boilers)
- **System Mode Controller** (`internal/controllers/buffercontroller/systemmode.go`): Handles system-wide mode transitions (off/heating/cooling/circulate)

### Key Packages

- `db/`: SQLite database operations, schema management, and queries
- `internal/model/`: Core data models (Zone, Device, HeatPump, Boiler, AirHandler, etc.)
- `internal/config/`: Configuration loading and validation 
- `internal/gpio/`: GPIO operations via pinctrl, sensor reading, relay control
- `internal/device/`: Device state management and timing constraints
- `system/`: Startup/shutdown scripts and systemd service management

### Data Flow

1. Main controller starts zone controllers (one per zone) and buffer controller as goroutines
2. Controllers poll temperature sensors every `poll_interval_seconds` (default 30s)
3. Zone controllers make local heating/cooling decisions based on setpoints
4. Buffer controller coordinates heat sources based on buffer tank temperature
5. All state changes persist to SQLite database
6. GPIO operations control physical relays

### Configuration

- `config.json`: System configuration including zones, devices, GPIO pins, thresholds
- Database: Runtime state persistence (device states, timings, sensor readings)
- Temperature sensors accessed via 1-wire bus at `/sys/bus/w1/devices/`

### Device Types and Constraints

Each device type has minimum on/off times to protect equipment:
- Heat pumps: 10min on, 5min off (with mode switching pins)
- Air handlers: 3min on, 1min off (with circulation pumps)
- Radiant loops: 5min on, 3min off
- Boilers: 2min on, 5min off

### Testing Strategy

The codebase uses standard Go testing with testify for assertions. Key test files:
- Controller logic tests use dependency injection for database/GPIO operations
- System mode transitions are thoroughly tested
- Pin control operations are mocked for testing

### GPIO Safety

- Main power pin (25) controls relay board activation
- Pin states validated on startup to prevent unsafe conditions
- Startup script sets initial pin states via systemd service
- Graceful shutdown handling to ensure safe device states