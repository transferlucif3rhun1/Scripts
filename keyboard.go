package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"time"
)

// Configuration
type Config struct {
	TrackingURL      string
	TelegramBotToken string
	TelegramChatID   string
	OrderNumber      string
	PollInterval     time.Duration
}

// API Response structures
type Checkpoint struct {
	City   *string `json:"city"`
	Date   string  `json:"date"`
	Remark string  `json:"remark"`
	State  *string `json:"state"`
}

type TrackingResponse struct {
	Checkpoints          []Checkpoint `json:"checkpoints"`
	DeliveryDate         *string      `json:"delivery_date"`
	ExpectedDeliveryDate *string      `json:"expected_delivery_date"`
	OrderID              string       `json:"order_id"`
	PODLink              *string      `json:"pod_link"`
	Slug                 *string      `json:"slug"`
	Tag                  string       `json:"tag"`
	TrackingNumber       *string      `json:"tracking_number"`
}

type TrackingRequest struct {
	QNum string `json:"q_num"`
}

// Telegram message structure
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// OrderTracker manages the tracking state
type OrderTracker struct {
	config       Config
	lastResponse *TrackingResponse
	httpClient   *http.Client
}

// NewOrderTracker creates a new order tracker instance
func NewOrderTracker(config Config) *OrderTracker {
	return &OrderTracker{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// trackOrder makes a request to the tracking API
func (ot *OrderTracker) trackOrder() (*TrackingResponse, error) {
	payload := TrackingRequest{QNum: ot.config.OrderNumber}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", ot.config.TrackingURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OrderTracker/1.0")

	resp, err := ot.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var trackingResp TrackingResponse
	if err := json.Unmarshal(body, &trackingResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &trackingResp, nil
}

// sendTelegramMessage sends a message via Telegram Bot API
func (ot *OrderTracker) sendTelegramMessage(message string) error {
	telegramURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ot.config.TelegramBotToken)

	telegramMsg := TelegramMessage{
		ChatID:    ot.config.TelegramChatID,
		Text:      message,
		ParseMode: "Markdown",
	}

	jsonPayload, err := json.Marshal(telegramMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram message: %w", err)
	}

	req, err := http.NewRequest("POST", telegramURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := ot.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API returned status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// formatTrackingUpdate creates a formatted message for Telegram
func (ot *OrderTracker) formatTrackingUpdate(response *TrackingResponse, isInitial bool) string {
	var message string

	if isInitial {
		message = fmt.Sprintf("🔍 *Order Tracking Started*\n\n")
	} else {
		message = fmt.Sprintf("📦 *Order Status Updated*\n\n")
	}

	message += fmt.Sprintf("*Order ID:* `%s`\n", response.OrderID)
	message += fmt.Sprintf("*Current Status:* `%s`\n\n", response.Tag)

	if response.TrackingNumber != nil {
		message += fmt.Sprintf("*Tracking Number:* `%s`\n", *response.TrackingNumber)
	}

	if response.ExpectedDeliveryDate != nil {
		message += fmt.Sprintf("*Expected Delivery:* `%s`\n", *response.ExpectedDeliveryDate)
	}

	if response.DeliveryDate != nil {
		message += fmt.Sprintf("*Delivered On:* `%s`\n", *response.DeliveryDate)
	}

	if len(response.Checkpoints) > 0 {
		message += "\n*Latest Updates:*\n"
		// Show last 3 checkpoints
		start := 0
		if len(response.Checkpoints) > 3 {
			start = len(response.Checkpoints) - 3
		}

		for i := start; i < len(response.Checkpoints); i++ {
			checkpoint := response.Checkpoints[i]
			parsedTime, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", checkpoint.Date)
			var timeStr string
			if err != nil {
				timeStr = checkpoint.Date
			} else {
				timeStr = parsedTime.Format("Jan 02, 2006 15:04")
			}

			message += fmt.Sprintf("• `%s` - %s\n", timeStr, checkpoint.Remark)
			if checkpoint.City != nil || checkpoint.State != nil {
				location := ""
				if checkpoint.City != nil {
					location += *checkpoint.City
				}
				if checkpoint.State != nil {
					if location != "" {
						location += ", "
					}
					location += *checkpoint.State
				}
				if location != "" {
					message += fmt.Sprintf("  📍 %s\n", location)
				}
			}
		}
	}

	return message
}

// hasStatusChanged compares two tracking responses to detect changes
func (ot *OrderTracker) hasStatusChanged(current, previous *TrackingResponse) bool {
	if previous == nil {
		return true // First time tracking
	}

	// Check if tag changed
	if current.Tag != previous.Tag {
		return true
	}

	// Check if checkpoints changed
	if len(current.Checkpoints) != len(previous.Checkpoints) {
		return true
	}

	// Check if any checkpoint details changed
	if !reflect.DeepEqual(current.Checkpoints, previous.Checkpoints) {
		return true
	}

	// Check if delivery dates changed
	if !reflect.DeepEqual(current.DeliveryDate, previous.DeliveryDate) ||
		!reflect.DeepEqual(current.ExpectedDeliveryDate, previous.ExpectedDeliveryDate) {
		return true
	}

	return false
}

// start begins the tracking loop
func (ot *OrderTracker) start() {
	log.Printf("Starting order tracker for order: %s", ot.config.OrderNumber)

	// Send initial tracking message
	if err := ot.sendTelegramMessage(fmt.Sprintf("🚀 Order tracking started for order: `%s`", ot.config.OrderNumber)); err != nil {
		log.Printf("Failed to send initial message: %v", err)
	}

	ticker := time.NewTicker(ot.config.PollInterval)
	defer ticker.Stop()

	// Initial check
	ot.checkForUpdates(true)

	// Periodic checks
	for range ticker.C {
		ot.checkForUpdates(false)
	}
}

// checkForUpdates polls the API and sends notifications if needed
func (ot *OrderTracker) checkForUpdates(isInitial bool) {
	response, err := ot.trackOrder()
	if err != nil {
		log.Printf("Failed to track order: %v", err)
		return
	}

	if ot.hasStatusChanged(response, ot.lastResponse) {
		message := ot.formatTrackingUpdate(response, isInitial)
		if err := ot.sendTelegramMessage(message); err != nil {
			log.Printf("Failed to send Telegram notification: %v", err)
		} else {
			log.Printf("Status update sent for order %s: %s", response.OrderID, response.Tag)
		}
	} else {
		log.Printf("No changes detected for order %s", response.OrderID)
	}

	ot.lastResponse = response
}

func main() {
	// Configuration from environment variables
	config := Config{
		TrackingURL:      "https://app.eshipz.com/track-widget/f98e4531-2efb-4b78-86eb-053388a87a0c?order-fallback=y&cos=n",
		TelegramBotToken: "6558694781:AAGD8ir-a_Mr0n9SrkCa6SfmFVt2IGQ8Ets",
		TelegramChatID:   "5016416878",
		OrderNumber:      "416153",
		PollInterval:     5 * time.Minute, // Check every 5 minutes
	}

	// Validate configuration
	if config.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}
	if config.TelegramChatID == "" {
		log.Fatal("TELEGRAM_CHAT_ID environment variable is required")
	}
	if config.OrderNumber == "" {
		log.Fatal("ORDER_NUMBER environment variable is required")
	}

	// Override poll interval if specified
	if pollEnv := os.Getenv("POLL_INTERVAL"); pollEnv != "" {
		if duration, err := time.ParseDuration(pollEnv); err == nil {
			config.PollInterval = duration
		}
	}

	tracker := NewOrderTracker(config)
	tracker.start()
}
