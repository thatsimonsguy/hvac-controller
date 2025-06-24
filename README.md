# HVAC Controller

A custom-built HVAC control system designed to manage a multi-zone heating and cooling setup using a Raspberry Pi 5. This project supports zone-based control of heat pumps, air handlers, radiant pumps, and more, with GPIO integration and state persistence.

## Features

- Multi-zone temperature management
- Support for heating, cooling, and fan-only modes
- GPIO-based relay control via `pinctrl`
- System state persistence with sqlite-backed storage
- Configurable min/max zone temperatures
- Runtime-safe shutdown handling
- Designed for indoor residential use (min exterior temp 55°F)

## Tech Stack

- Go (controller logic and API)
- Raspberry Pi GPIO via `pinctrl`
- JSON config and SQLite state files

## Project Structure

- `internal/` - Controller logic, GPIO wrappers, and device management
- `system/` - Startup scripts for the Pi and safe shutdown methods
- `cmd/` - Entrypoint for main controller loop
- `config.json` - System configuration file

## Setup

1. Flash Raspberry Pi OS and enable SSH
2. Clone the repo and install Go
3. Run ```go mod init github.com/thatsimonsguy/hvac-controller``` and ```go mod tidy```
4. Create your logging file and set permissions. Optionally configure 3rd-party log exporter.
5. Configure `config/config.json` with system layout
6. Run the controller:

   ```bash
   go run ./cmd/hvac-controller/main.go
