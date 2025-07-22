package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const bufferSize = 10
const expireAfter = 5 * time.Minute // expiration time for auto release

// Global flag for detailed logging
var detailedLogging = false // Set to 'false' to turn off detailed logging

// Manager handles all assign and delete operations.
type Manager struct {
	Available      []string
	Assigned       map[int]*NumberInfo
	AssignChan     chan AssignRequest
	DeleteChan     chan DeleteRequest
	FileWriteChan  chan WriteRequest
	FilePath       string
	UpdatedFile    string
	mu             sync.Mutex
}

// NumberInfo holds the assignment status and expiration.
type NumberInfo struct {
	PhoneNumber string
	Expires     time.Time
}

// AssignRequest represents a request to assign a phone number to an ID.
type AssignRequest struct {
	ID       int
	Response chan AssignResponse
}

// AssignResponse represents the response to an assign request.
type AssignResponse struct {
	PhoneNumber string `json:"assigned_phone_number,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DeleteRequest represents a request to delete a phone number from an ID.
type DeleteRequest struct {
	ID       int
	Response chan DeleteResponse
}

// DeleteResponse represents the response to a delete request.
type DeleteResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WriteRequest represents a request to write data to a file.
type WriteRequest struct {
	FilePath string
	Data     []string
	Done     chan error
}

func main() {
	// Set Gin to release mode
	gin.SetMode(gin.ReleaseMode)

	// Initialize logger with timestamp
	log.SetFlags(log.LstdFlags)

	// Create a new Gin engine without the default logger
	router := gin.New()

	// Use only the recovery middleware to handle panics and log them
	router.Use(gin.Recovery())

	// Initialize Manager
	manager := &Manager{
		Available:     []string{},
		Assigned:      make(map[int]*NumberInfo),
		AssignChan:    make(chan AssignRequest, bufferSize),
		DeleteChan:    make(chan DeleteRequest, bufferSize),
		FileWriteChan: make(chan WriteRequest, bufferSize),
		FilePath:      "numbers.txt",
		UpdatedFile:   "updated_numbers.txt",
	}

	// Load phone numbers
	if err := manager.loadPhoneNumbers(); err != nil {
		log.Fatalf("Error loading phone numbers: %v", err)
	}

	// Log the file that was used to load numbers and the count of numbers loaded
	logImportant("Loaded %d phone numbers from file: %s", len(manager.Available), manager.FilePath)

	// Start the Manager
	go manager.run()

	// Start the file writer goroutine
	go manager.fileWriter()

	// Start auto-release monitoring
	go manager.monitorExpirations()

	// Setup routes
	router.POST("/assign", manager.handleAssign)
	router.POST("/delete", manager.handleDelete)
	router.GET("/releaseAll", manager.handleReleaseAll)

	// Log the server startup on a specific port
	serverPort := ":3060"
	logImportant("Server started on port %s", serverPort)

	// Start the HTTP server without logging each request
	if err := router.Run(serverPort); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// loadPhoneNumbers loads available phone numbers from updated_numbers.txt or phone_numbers.txt.
func (m *Manager) loadPhoneNumbers() error {
	fileUsed := m.FilePath

	// Check if updated_numbers.txt exists
	if _, err := os.Stat(m.UpdatedFile); err == nil {
		fileUsed = m.UpdatedFile
	}

	file, err := os.Open(fileUsed)
	if err != nil {
		return fmt.Errorf("failed to open phone numbers file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		phoneNumber := scanner.Text()
		if phoneNumber != "" {
			m.Available = append(m.Available, phoneNumber)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading phone numbers file: %v", err)
	}

	// Update the FilePath to the one that was used
	m.FilePath = fileUsed

	return nil
}


// run starts the Manager's main loop to handle assign and delete requests.
func (m *Manager) run() {
	for {
		select {
		case assignReq := <-m.AssignChan:
			go m.processAssign(assignReq) // Process each assign request in its own goroutine
		case deleteReq := <-m.DeleteChan:
			go m.processDelete(deleteReq) // Process each delete request in its own goroutine
		}
	}
}

// processAssign handles assign requests and consolidates logs.
func (m *Manager) processAssign(req AssignRequest) {
	response := AssignResponse{}
	startTime := time.Now()

	m.mu.Lock()

	// Treat the request as a new ID if it is not found in the Assigned map
	if info, exists := m.Assigned[req.ID]; exists {
		// ID is found, treat it as a reassignment (or expiration reset)
		response.PhoneNumber = info.PhoneNumber
		// Reset expiration time
		info.Expires = time.Now().Add(expireAfter)
		logDetailed("Reassigned phone number %s (expires reset).", req.ID, info.PhoneNumber)
	} else {
		// Assign a new phone number if available (new request for this ID)
		if len(m.Available) == 0 {
			response.Error = "No phone numbers available to assign"
			logImportant("No phone numbers available.", req.ID)
		} else {
			// Assign the first available phone number
			phoneNumber := m.Available[0]
			m.Available = m.Available[1:]
			m.Assigned[req.ID] = &NumberInfo{PhoneNumber: phoneNumber, Expires: time.Now().Add(expireAfter)}
			response.PhoneNumber = phoneNumber
			logDetailed("Assigned new phone number %s.", req.ID, phoneNumber)
		}
	}

	m.mu.Unlock()

	logDetailed("Assign process completed in %s", time.Since(startTime))

	req.Response <- response
}

// processDelete handles delete requests and consolidates logs.
func (m *Manager) processDelete(req DeleteRequest) {
	response := DeleteResponse{}
	startTime := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if ID has an assigned phone number
	info, exists := m.Assigned[req.ID]
	if !exists {
		response.Error = "Phone number not assigned to this ID"
		logImportant("Delete request failed (not assigned).", req.ID)
		req.Response <- response
		return
	}

	// Remove the number from Assigned and don't delete it if it's already in the Available list
	delete(m.Assigned, req.ID)
	if contains(m.Available, info.PhoneNumber) {
		logImportant("Phone number %s is already available, skipping deletion.", req.ID, info.PhoneNumber)
		response.Message = fmt.Sprintf("Phone number %s is already available, no deletion performed.", info.PhoneNumber)
		req.Response <- response
		return
	}

	// Permanently delete the phone number from the system
	logImportant("Deleted phone number %s.", info.PhoneNumber)

	// Write the updated list of assigned numbers to the file
	writeDone := make(chan error)
	allNumbers := append(getAssignedNumbers(m.Assigned), m.Available...) // Only writing remaining assigned + available numbers to file
	m.FileWriteChan <- WriteRequest{
		FilePath: m.UpdatedFile,
		Data:     allNumbers,
		Done:     writeDone,
	}

	if err := <-writeDone; err != nil {
		response.Error = "Failed to update the file"
		logImportant("File update failed after deletion of %s. Error: %v", req.ID, info.PhoneNumber, err)
	} else {
		response.Message = fmt.Sprintf("Phone number %s permanently deleted.", info.PhoneNumber)
		logDetailed("File updated successfully after deleting phone number %s.", info.PhoneNumber)
	}

	logDetailed("Delete process completed in %s", time.Since(startTime))

	req.Response <- response
}

// handleAssign is the HTTP handler for the /assign endpoint using Gin.
func (m *Manager) handleAssign(c *gin.Context) {
	var requestData struct {
		Number int `json:"number"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		logImportant("Invalid assign request payload: %v", err)
		c.JSON(400, gin.H{"error": "Invalid JSON payload"})
		return
	}

	responseChan := make(chan AssignResponse)
	assignReq := AssignRequest{
		ID:       requestData.Number,
		Response: responseChan,
	}

	m.AssignChan <- assignReq
	response := <-responseChan

	if response.Error != "" {
		c.JSON(404, gin.H{"error": response.Error})
	} else {
		// Respond with the phone number in the "data.phone" field
		c.JSON(200, gin.H{
			"data": gin.H{
				"phone": response.PhoneNumber,
			},
		})
	}
}


// handleDelete is the HTTP handler for the /delete endpoint using Gin.
func (m *Manager) handleDelete(c *gin.Context) {
	var requestData struct {
		Number int `json:"number"`
	}
	if err := c.ShouldBindJSON(&requestData); err != nil {
		logImportant("Invalid delete request payload: %v", err)
		c.JSON(400, gin.H{"error": "Invalid JSON payload"})
		return
	}

	responseChan := make(chan DeleteResponse)
	deleteReq := DeleteRequest{
		ID:       requestData.Number,
		Response: responseChan,
	}

	m.DeleteChan <- deleteReq
	response := <-responseChan

	if response.Error != "" {
		c.JSON(404, gin.H{"error": response.Error})
	} else {
		// Respond with the delete message in JSON format
		c.JSON(200, gin.H{
			"message": response.Message,
		})
	}
}


// handleReleaseAll handles the /releaseAll HTTP endpoint using Gin.
func (m *Manager) handleReleaseAll(c *gin.Context) {
	logImportant("Manual release of all phone numbers initiated.")

	// Move all assigned numbers to the start of the available list
	m.mu.Lock()
	for id, info := range m.Assigned {
		m.Available = append([]string{info.PhoneNumber}, m.Available...)
		delete(m.Assigned, id)
	}
	m.mu.Unlock()

	logImportant("Manual release completed. All phone numbers are now available.")
	c.String(200, "All phone numbers released successfully.")
}

// monitorExpirations checks for phone numbers that need to be auto-released.
func (m *Manager) monitorExpirations() {
	for {
		time.Sleep(1 * time.Second) // Add delay to prevent excessive CPU usage

		m.mu.Lock()
		for id, info := range m.Assigned {
			if time.Now().After(info.Expires) {
				logImportant("Auto-releasing phone number %s (ID %d).", info.PhoneNumber, id)
				// Add the auto-released phone number to the front of the available list
				m.Available = append([]string{info.PhoneNumber}, m.Available...)
				delete(m.Assigned, id) // Remove from assigned map, treat future requests as new assignments
			}
		}
		m.mu.Unlock()
	}
}

// fileWriter listens to the FileWriteChan and writes data to the file.
func (m *Manager) fileWriter() {
	for {
		writeReq := <-m.FileWriteChan
		file, err := os.OpenFile(writeReq.FilePath, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			writeReq.Done <- fmt.Errorf("failed to open file for writing: %v", err)
			continue
		}

		writer := bufio.NewWriter(file)
		for _, line := range writeReq.Data {
			_, err := writer.WriteString(fmt.Sprintf("%s\n", line))
			if err != nil {
				writeReq.Done <- fmt.Errorf("failed to write to file: %v", err)
				_ = file.Close()
				continue
			}
		}

		if err := writer.Flush(); err != nil {
			writeReq.Done <- fmt.Errorf("failed to flush writer: %v", err)
			_ = file.Close()
			continue
		}

		_ = file.Close()
		writeReq.Done <- nil
	}
}

// contains checks if a string is present in a list
func contains(list []string, phoneNumber string) bool {
	for _, num := range list {
		if num == phoneNumber {
			return true
		}
	}
	return false
}

// getAssignedNumbers retrieves a list of currently assigned numbers.
func getAssignedNumbers(assigned map[int]*NumberInfo) []string {
	assignedList := []string{}
	for _, info := range assigned {
		assignedList = append(assignedList, info.PhoneNumber)
	}
	return assignedList
}

// logDetailed prints a log message if detailedLogging is enabled.
func logDetailed(format string, args ...interface{}) {
	if detailedLogging {
		log.Printf(format, args...)
	}
}

// logImportant prints important log messages regardless of the detailedLogging flag.
func logImportant(format string, args ...interface{}) {
	log.Printf(format, args...)
}
