package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Manager for handling the auth and status caching
type Manager struct {
	autoDestruct        bool
	hwid                string
	authServerURL       string
	statuses            map[string]string
	statusesMutex       sync.RWMutex
	cancelFuncs         map[string]context.CancelFunc
	flaggedNames        map[string]bool
	flaggedNamesMu      sync.RWMutex
	activeRoutines      sync.WaitGroup
	shutdownChan        chan struct{}
	loggedRequests      map[string]bool // Track whether a request has already been logged
	loggedRequestsMu    sync.RWMutex
	finalStatusLogged   map[string]bool // Track if final status (authenticated/flagged) is logged
	finalStatusLoggedMu sync.RWMutex
}

// NewManager creates a new centralized manager
func NewManager(autoDestruct bool, authServerURL string) *Manager {
	return &Manager{
		autoDestruct:      autoDestruct,
		authServerURL:     authServerURL,
		statuses:          make(map[string]string),
		cancelFuncs:       make(map[string]context.CancelFunc),
		flaggedNames:      make(map[string]bool),
		shutdownChan:      make(chan struct{}),
		loggedRequests:    make(map[string]bool),  // Track logged requests to avoid repeated logs
		finalStatusLogged: make(map[string]bool),  // Track final status logs
	}
}

// logMessage logs messages with a timestamp
func (m *Manager) logMessage(level, message string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [%s]: %s\n", timestamp, level, message)
}

// generateHWID generates an HWID for the system
func (m *Manager) generateHWID() string {
	hostID, err := os.Hostname()
	if err != nil {
		m.logMessage("ERROR", "Failed to retrieve system information.")
		return ""
	}
	hash := sha256.Sum256([]byte(hostID))
	hwid := fmt.Sprintf("%x", hash)
	m.logMessage("INFO", "System identifier generated.")
	return hwid
}

// sanitizeInput sanitizes input strings by replacing spaces, special characters, and removing unwanted characters
func (m *Manager) sanitizeInput(input string) string {
	input = strings.ReplaceAll(input, " ", "_")
	input = strings.ReplaceAll(input, "-", "_")
	input = strings.ReplaceAll(input, "(", "")
	input = strings.ReplaceAll(input, ")", "")
	input = strings.ReplaceAll(input, "[", "")
	input = strings.ReplaceAll(input, "]", "")
	input = strings.ReplaceAll(input, "@", "")

	allowedPattern := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	return allowedPattern.ReplaceAllString(input, "")
}

// requestAuth sends a request to the auth server and returns the response
func (m *Manager) requestAuth(sanitizedName, hwid string, whitelist bool) (map[string]string, error) {
	url := fmt.Sprintf("%s/auth?name=%s&hwid=%s", m.authServerURL, sanitizedName, hwid)
	if whitelist {
		url += "&whitelist=true"
	}

	resp, err := http.Get(url)
	if err != nil {
		m.logMessage("ERROR", "Failed to contact server.")
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		m.logMessage("ERROR", "Failed to read server response.")
		return nil, err
	}

	var response map[string]string
	if err := json.Unmarshal(body, &response); err != nil {
		m.logMessage("ERROR", fmt.Sprintf("Failed to process server response: %s", string(body)))
		return nil, err
	}

	return response, nil
}

// validateHWID validates the HWID by interacting with the auth server, and updates the status
func (m *Manager) validateHWID(ctx context.Context, decodedName, sanitizedName, hwid string, whitelist bool) {
	defer m.activeRoutines.Done()

	// Make a single request to validate and handle the response
	authResp, err := m.requestAuth(sanitizedName, hwid, whitelist)
	if err != nil {
		m.updateStatus(sanitizedName, "failed")
		return
	}

	// Handle the response and update the status
	m.handleAuthResponse(authResp, decodedName, sanitizedName)
}

// handleAuthResponse processes the response from the auth server and updates the status
func (m *Manager) handleAuthResponse(authResp map[string]string, decodedName, sanitizedName string) {
	if _, exists := authResp["error"]; exists {
		m.updateStatus(sanitizedName, "flagged")
		m.logFinalStatusOnce(sanitizedName, fmt.Sprintf("System identifier flagged for: %s. Terminating process.", decodedName))
		m.handleFlaggedHWID(sanitizedName)
	} else {
		switch authResp["status"] {
		case "User registered", "User authenticated", "HWID moved to whitelist":
			m.updateStatus(sanitizedName, "authenticated")
			m.logFinalStatusOnce(sanitizedName, fmt.Sprintf("Auth successful for: %s", decodedName))
		case "Multiple HWID detected and flagged":
			m.updateStatus(sanitizedName, "flagged")
			m.logFinalStatusOnce(sanitizedName, fmt.Sprintf("Multiple identifiers detected for: %s. Terminating process.", decodedName))
			m.handleFlaggedHWID(sanitizedName)
		default:
			m.updateStatus(sanitizedName, "failed")
			m.logFinalStatusOnce(sanitizedName, fmt.Sprintf("Unexpected server response for: %s", decodedName))
		}
	}
}

// logRequestOnce logs the request only once for each unique sanitizedName
func (m *Manager) logRequestOnce(sanitizedName, message string) {
	m.loggedRequestsMu.Lock()
	defer m.loggedRequestsMu.Unlock()

	// Log once per request name
	if _, alreadyLogged := m.loggedRequests[sanitizedName]; !alreadyLogged {
		m.logMessage("INFO", message)
		m.loggedRequests[sanitizedName] = true
	}
}

// logFinalStatusOnce logs the final status (authenticated/flagged) only once for each unique sanitizedName
func (m *Manager) logFinalStatusOnce(sanitizedName, message string) {
	m.finalStatusLoggedMu.Lock()
	defer m.finalStatusLoggedMu.Unlock()

	// Log once per request name only for final status
	if _, alreadyLogged := m.finalStatusLogged[sanitizedName]; !alreadyLogged {
		m.logMessage("INFO", message)
		m.finalStatusLogged[sanitizedName] = true
	}
}

// handleFlaggedHWID handles the case where an HWID is flagged, and cancels further validation for that name
func (m *Manager) handleFlaggedHWID(sanitizedName string) {
	m.flaggedNamesMu.Lock()
	m.flaggedNames[sanitizedName] = true
	m.flaggedNamesMu.Unlock()

	m.cancelFuncs[sanitizedName]()

	// Check if all names are flagged
	if m.areAllNamesFlagged() {
		m.logMessage("INFO", "All names flagged. Initiating shutdown.")
		m.shutdown()
	}
}

// updateStatus safely updates the status for a given name
func (m *Manager) updateStatus(sanitizedName, status string) {
	m.statusesMutex.Lock()
	defer m.statusesMutex.Unlock()

	// Only update status and log for significant transitions
	if m.statuses[sanitizedName] != status {
		m.statuses[sanitizedName] = status
	}
}

// getStatus safely retrieves the current status for a given name
func (m *Manager) getStatus(sanitizedName string) string {
	m.statusesMutex.RLock()
	defer m.statusesMutex.RUnlock()
	return m.statuses[sanitizedName]
}

// areAllNamesFlagged checks if all names have been flagged
func (m *Manager) areAllNamesFlagged() bool {
	m.flaggedNamesMu.RLock()
	defer m.flaggedNamesMu.RUnlock()
	return len(m.flaggedNames) == len(m.cancelFuncs)
}

// statusHandler handles requests to the /hwidauth endpoint, initiates HWID validation and updates status
func (m *Manager) statusHandler(w http.ResponseWriter, r *http.Request) {
	decodedName, err := url.QueryUnescape(r.URL.Query().Get("name"))
	if err != nil {
		m.logMessage("ERROR", fmt.Sprintf("Failed to decode name: %v", err))
		http.Error(w, "Invalid name parameter", http.StatusBadRequest)
		return
	}

	whitelist := r.URL.Query().Get("whitelist") == "true"

	sanitizedName := m.sanitizeInput(decodedName)

	m.flaggedNamesMu.RLock()
	if m.flaggedNames[sanitizedName] {
		http.Error(w, "Request blocked due to flagged identifier", http.StatusForbidden)
		m.flaggedNamesMu.RUnlock()
		return
	}
	m.flaggedNamesMu.RUnlock()

	currentStatus := m.getStatus(sanitizedName)

	// If the current status is not yet authenticated or flagged, proceed with validation
	if currentStatus == "" || currentStatus == "pending" {
		m.logRequestOnce(sanitizedName, fmt.Sprintf("Received request for: %s", decodedName))

		// Initialize the status to "pending" and start validation
		m.updateStatus(sanitizedName, "pending")

		ctx, cancel := context.WithCancel(context.Background())
		m.cancelFuncs[sanitizedName] = cancel

		m.activeRoutines.Add(1)
		go m.validateHWID(ctx, decodedName, sanitizedName, m.hwid, whitelist)
	}

	currentStatus = m.getStatus(sanitizedName)

	response := map[string]string{
		"status": currentStatus,
	}
	jsonResp, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)

	// After the status is logged, stop further processing for this name
	if currentStatus == "authenticated" || currentStatus == "flagged" {
		m.logFinalStatusOnce(sanitizedName, fmt.Sprintf("Final status for %s: %s", decodedName, currentStatus))
	}
}

// shutdown gracefully shuts down all processes and the server
func (m *Manager) shutdown() {
	// Signal shutdown
	close(m.shutdownChan)

	// Cancel all running goroutines
	for _, cancel := range m.cancelFuncs {
		cancel()
	}

	// Wait for all goroutines to finish
	m.activeRoutines.Wait()

	// Keep the script running without exiting
	m.logMessage("INFO", "All processes have been shut down. Server has stopped.")
	select {} // Block indefinitely to prevent the script from exiting
}

func main() {
	// Create a new centralized manager
	manager := NewManager(false, "http://45.92.1.127:8030") // Set to server address

	// Generate HWID
	manager.hwid = manager.generateHWID()
	if manager.hwid == "" {
		return // Exit if HWID generation failed
	}

	// Set up and run the server
	server := &http.Server{Addr: ":3069", Handler: http.DefaultServeMux}
	http.HandleFunc("/hwidauth", manager.statusHandler)

	// Run the server in a separate goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			manager.logMessage("ERROR", fmt.Sprintf("Server failed to start: %v", err))
			manager.shutdown()
		}
	}()

	// Block until shutdown is signaled
	<-manager.shutdownChan

	// Cleanup before exit
	manager.shutdown()
}
