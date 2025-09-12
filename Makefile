# Basic build and run targets
build:
	go build -o hvac-controller ./cmd/hvac-controller/main.go

run:
	./hvac-controller

# System mode control targets
system-off:
	go run ./cmd/debug/main.go -cmd set-system-mode -mode off

system-cool:
	go run ./cmd/debug/main.go -cmd set-system-mode -mode cooling

system-heat:
	go run ./cmd/debug/main.go -cmd set-system-mode -mode heating

# Zone control targets
zones-off:
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone main_floor -mode off
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone basement -mode off
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone garage -mode off

zones-cool:
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone main_floor -mode cooling
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone basement -mode cooling
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone garage -mode off

zones-heat:
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone main_floor -mode heating
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone basement -mode heating
	go run ./cmd/debug/main.go -cmd set-zone-mode -zone garage -mode heating

# Zone setpoint control targets (usage: make main-floor-temp TEMP=72)
main-floor-temp:
	go run ./cmd/debug/main.go -cmd set-zone-setpoint -zone main_floor -setpoint $(TEMP)

basement-temp:
	go run ./cmd/debug/main.go -cmd set-zone-setpoint -zone basement -setpoint $(TEMP)

garage-temp:
	go run ./cmd/debug/main.go -cmd set-zone-setpoint -zone garage -setpoint $(TEMP)

# Debug utility targets
reset-recirc-timers:
	go run ./cmd/debug/main.go -cmd reset-air-handler-timestamps