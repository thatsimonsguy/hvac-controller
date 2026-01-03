package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/thatsimonsguy/hvac-controller/internal/env"
)

var client *http.Client
var topic string
var initialized bool

// Init initializes the notification client
func Init() {
	if env.Cfg.NtfyTopic == "" {
		log.Warn().Msg("Ntfy topic not configured - notifications disabled")
		return
	}

	client = &http.Client{
		Timeout: 10 * time.Second,
	}
	topic = env.Cfg.NtfyTopic
	initialized = true

	log.Info().
		Str("topic", topic).
		Msg("Ntfy notifications initialized")
}

// Send sends a notification to ntfy.sh
func Send(title, message string) error {
	if !initialized {
		return fmt.Errorf("notifications not initialized")
	}

	url := fmt.Sprintf("https://ntfy.sh/%s", topic)

	payload := map[string]interface{}{
		"topic":   topic,
		"title":   title,
		"message": message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned non-success status: %d", resp.StatusCode)
	}

	log.Debug().
		Str("title", title).
		Int("status", resp.StatusCode).
		Msg("Notification sent successfully")

	return nil
}
