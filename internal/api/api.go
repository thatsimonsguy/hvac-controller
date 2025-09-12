package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/thatsimonsguy/hvac-controller/db"
	"github.com/thatsimonsguy/hvac-controller/internal/config"
	"github.com/thatsimonsguy/hvac-controller/internal/model"
	"github.com/thatsimonsguy/hvac-controller/internal/temperature"
)

type Server struct {
	db          *sql.DB
	tempService *temperature.Service
	config      *config.Config
}

type SystemModeResponse struct {
	Mode       string  `json:"mode"`
	BufferTemp float64 `json:"buffer_temp"`
}

type SystemModeRequest struct {
	Mode string `json:"mode"`
}

type ZoneResponse struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Setpoint    float64 `json:"setpoint"`
	Mode        string  `json:"mode"`
	CurrentTemp float64 `json:"current_temp"`
}

type ZoneSetpointRequest struct {
	Setpoint float64 `json:"setpoint"`
}

type ZoneModeRequest struct {
	Mode string `json:"mode"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer(database *sql.DB, tempService *temperature.Service, cfg *config.Config) *Server {
	return &Server{
		db:          database,
		tempService: tempService,
		config:      cfg,
	}
}

func (s *Server) Start(port int) error {
	mux := http.NewServeMux()
	
	// Add CORS middleware
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		// Handle preflight OPTIONS requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		mux.ServeHTTP(w, r)
	})
	
	// System mode endpoints
	mux.HandleFunc("/api/system/mode", s.handleSystemMode)
	
	// Zone endpoints
	mux.HandleFunc("/api/zones", s.handleZones)
	mux.HandleFunc("/api/zones/", s.handleZoneOperations)
	
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Info().Str("address", addr).Msg("Starting REST API server")
	
	return http.ListenAndServe(addr, corsHandler)
}

func (s *Server) handleSystemMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getSystemMode(w, r)
	case http.MethodPut:
		s.setSystemMode(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleZones(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/api/zones" {
		s.getZones(w, r)
	} else {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleZoneOperations(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/zones/")
	parts := strings.Split(path, "/")
	
	if len(parts) < 1 || parts[0] == "" {
		s.writeError(w, http.StatusNotFound, "Zone ID required")
		return
	}
	
	zoneID := parts[0]
	
	if len(parts) == 1 {
		// /api/zones/{id}
		if r.Method == http.MethodGet {
			s.getZone(w, r, zoneID)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	} else if len(parts) == 2 {
		// /api/zones/{id}/mode or /api/zones/{id}/setpoint
		operation := parts[1]
		if r.Method == http.MethodPut {
			switch operation {
			case "mode":
				s.setZoneMode(w, r, zoneID)
			case "setpoint":
				s.setZoneSetpoint(w, r, zoneID)
			default:
				s.writeError(w, http.StatusNotFound, "Unknown operation")
			}
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	} else {
		s.writeError(w, http.StatusNotFound, "Invalid path")
	}
}

func (s *Server) getSystemMode(w http.ResponseWriter, r *http.Request) {
	mode, err := db.GetSystemMode(s.db)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get system mode")
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// Get buffer tank temperature
	bufferTemp, _ := s.tempService.GetTemperature("buffer_tank")
	
	response := SystemModeResponse{
		Mode:       string(mode),
		BufferTemp: bufferTemp,
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) setSystemMode(w http.ResponseWriter, r *http.Request) {
	var req SystemModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	
	// Validate mode
	systemMode := model.SystemMode(req.Mode)
	if !isValidSystemMode(systemMode) {
		s.writeError(w, http.StatusBadRequest, "Invalid system mode. Valid modes: off, heating, cooling, circulate")
		return
	}
	
	if err := db.UpdateSystemMode(s.db, systemMode); err != nil {
		log.Error().Err(err).Str("mode", req.Mode).Msg("Failed to update system mode")
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	log.Info().Str("mode", req.Mode).Msg("System mode updated via API")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) getZones(w http.ResponseWriter, r *http.Request) {
	zones, err := db.GetAllZones(s.db)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get zones")
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	var response []ZoneResponse
	for _, zone := range zones {
		temp, _ := s.tempService.GetTemperature(zone.Sensor.ID)
		response = append(response, ZoneResponse{
			ID:          zone.ID,
			Label:       zone.Label,
			Setpoint:    zone.Setpoint,
			Mode:        string(zone.Mode),
			CurrentTemp: temp,
		})
	}
	
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) getZone(w http.ResponseWriter, r *http.Request, zoneID string) {
	zone, err := db.GetZoneByID(s.db, zoneID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
			s.writeError(w, http.StatusNotFound, "Zone not found")
		} else {
			log.Error().Err(err).Str("zone_id", zoneID).Msg("Failed to get zone")
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	
	temp, _ := s.tempService.GetTemperature(zone.Sensor.ID)
	response := ZoneResponse{
		ID:          zone.ID,
		Label:       zone.Label,
		Setpoint:    zone.Setpoint,
		Mode:        string(zone.Mode),
		CurrentTemp: temp,
	}
	
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) setZoneMode(w http.ResponseWriter, r *http.Request, zoneID string) {
	var req ZoneModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	
	// Validate mode
	zoneMode := model.SystemMode(req.Mode)
	if !isValidSystemMode(zoneMode) {
		s.writeError(w, http.StatusBadRequest, "Invalid zone mode. Valid modes: off, heating, cooling, circulate")
		return
	}
	
	// Check if zone exists
	if _, err := db.GetZoneByID(s.db, zoneID); err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
			s.writeError(w, http.StatusNotFound, "Zone not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	
	if err := db.UpdateZoneMode(s.db, zoneID, zoneMode); err != nil {
		log.Error().Err(err).Str("zone_id", zoneID).Str("mode", req.Mode).Msg("Failed to update zone mode")
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	log.Info().Str("zone_id", zoneID).Str("mode", req.Mode).Msg("Zone mode updated via API")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) setZoneSetpoint(w http.ResponseWriter, r *http.Request, zoneID string) {
	var req ZoneSetpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	
	// Validate setpoint range using config values
	if req.Setpoint < s.config.ZoneMinTemp || req.Setpoint > s.config.ZoneMaxTemp {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid setpoint. Must be between %.1f°F and %.1f°F", s.config.ZoneMinTemp, s.config.ZoneMaxTemp))
		return
	}
	
	// Check if zone exists
	if _, err := db.GetZoneByID(s.db, zoneID); err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
			s.writeError(w, http.StatusNotFound, "Zone not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	
	if err := db.UpdateZoneSetpoint(s.db, zoneID, req.Setpoint); err != nil {
		log.Error().Err(err).Str("zone_id", zoneID).Float64("setpoint", req.Setpoint).Msg("Failed to update zone setpoint")
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	log.Info().Str("zone_id", zoneID).Float64("setpoint", req.Setpoint).Msg("Zone setpoint updated via API")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

func isValidSystemMode(mode model.SystemMode) bool {
	switch mode {
	case model.ModeOff, model.ModeHeating, model.ModeCooling, model.ModeCirculate:
		return true
	default:
		return false
	}
}