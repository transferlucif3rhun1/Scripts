package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	ModemID      string
	PhoneNumber  string
	OriginalPort string
	Port         int
	Tags         string
}

type PortManager struct {
	data       []Entry
	assigned   map[int]string
	portAssign map[int]int
	busyPorts  map[int]bool
	portMap    map[int]map[string]bool
	portTimers map[int]*time.Timer
	dataLock   sync.RWMutex
	assignLock sync.Mutex
	wg         sync.WaitGroup
	needsSave  bool
	csvFile    string
}

const portTimeout = 5 * time.Minute

var portManager *PortManager

// NewPortManager creates a new instance of PortManager
func NewPortManager() *PortManager {
	return &PortManager{
		assigned:   make(map[int]string),
		portAssign: make(map[int]int),
		busyPorts:  make(map[int]bool),
		portMap:    make(map[int]map[string]bool),
		portTimers: make(map[int]*time.Timer),
	}
}

// LoadCSV reads entries from a CSV file into memory
func (pm *PortManager) LoadCSV(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Printf("ERROR: Unable to open CSV file '%s': %v", filename, err)
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("ERROR: Unable to read CSV file '%s': %v", filename, err)
		return err
	}

	var tempData []Entry
	for i, record := range records {
		if i == 0 || len(record) < 4 {
			continue
		}
		port, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			continue
		}
		portInt := int(math.Floor(port))
		tempData = append(tempData, Entry{
			ModemID:      record[0],
			PhoneNumber:  record[1],
			OriginalPort: record[2],
			Port:         portInt,
			Tags:         record[3],
		})
	}

	pm.dataLock.Lock()
	pm.data = tempData
	pm.initializeData()
	pm.dataLock.Unlock()

	log.Printf("INFO: Loaded CSV file '%s' with %d entries and %d unique ports.", filename, len(pm.data), len(pm.portMap))
	pm.csvFile = filename
	return nil
}

// initializeData prepares the port map from loaded data
func (pm *PortManager) initializeData() {
	pm.portMap = make(map[int]map[string]bool)
	for _, entry := range pm.data {
		port := entry.Port
		if pm.portMap[port] == nil {
			pm.portMap[port] = make(map[string]bool)
		}
		pm.portMap[port][entry.PhoneNumber] = false
	}
}

// SaveCSV writes the current data to a CSV file
func (pm *PortManager) SaveCSV(filename string) error {
	pm.dataLock.RLock()
	defer pm.dataLock.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"modemId", "Phone Number", "port", "tags"}); err != nil {
		return err
	}

	for _, entry := range pm.data {
		record := []string{
			entry.ModemID,
			entry.PhoneNumber,
			entry.OriginalPort,
			entry.Tags,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// AssignPhoneNumber assigns a phone number to a port
func (pm *PortManager) AssignPhoneNumber(number int) (string, int, bool) {
	pm.assignLock.Lock()
	defer pm.assignLock.Unlock()

	// If the number is already assigned to a port and hasn't been deleted, return the same phone number
	if port, alreadyAssigned := pm.portAssign[number]; alreadyAssigned {
		if phoneNumber, exists := pm.assigned[number]; exists {
			pm.resetPortTimer(port)
			return phoneNumber, port, true
		} else {
			// Try to assign a new phone number from any available port
			return pm.findNextAvailableNumber(number, port)
		}
	}

	// Try to assign a new phone number from any available port
	return pm.findNextAvailableNumber(number, -1)
}

// findNextAvailableNumber looks for the next available phone number across ports
func (pm *PortManager) findNextAvailableNumber(number int, currentPort int) (string, int, bool) {
	// First, try to use the current port if provided
	if currentPort != -1 {
		phoneNumber, ok := pm.getNextAvailableNumber(currentPort)
		if ok {
			pm.portAssign[number] = currentPort
			pm.assigned[number] = phoneNumber
			pm.busyPorts[currentPort] = true
			pm.resetPortTimer(currentPort)
			log.Printf("INFO: Assigned number %d -> phone %s (port %d).", number, phoneNumber, currentPort)
			pm.needsSave = true
			return phoneNumber, currentPort, true
		}
	}

	// If the current port is exhausted or not provided, loop through other ports
	ports := pm.getSortedAvailablePorts()
	for _, port := range ports {
		phoneNumber, ok := pm.getNextAvailableNumber(port)
		if ok {
			pm.portAssign[number] = port
			pm.assigned[number] = phoneNumber
			pm.busyPorts[port] = true
			pm.resetPortTimer(port)
			log.Printf("INFO: Assigned number %d -> phone %s (port %d).", number, phoneNumber, port)
			pm.needsSave = true
			return phoneNumber, port, true
		}
	}

	return "", 0, false
}

// getSortedAvailablePorts returns the list of available ports sorted by port number
func (pm *PortManager) getSortedAvailablePorts() []int {
	ports := make([]int, 0, len(pm.portMap))
	for port := range pm.portMap {
		if !pm.busyPorts[port] {
			ports = append(ports, port)
		}
	}
	sort.Ints(ports)
	return ports
}

// getNextAvailableNumber returns the next available phone number for a port
func (pm *PortManager) getNextAvailableNumber(port int) (string, bool) {
	for phoneNumber, used := range pm.portMap[port] {
		if !used {
			pm.portMap[port][phoneNumber] = true
			return phoneNumber, true
		}
	}
	return "", false
}

// DeletePhoneNumber deletes a phone number from the assignment but keeps the port assigned to the number
func (pm *PortManager) DeletePhoneNumber(number int) (string, int, bool) {
	pm.assignLock.Lock()
	defer pm.assignLock.Unlock()

	port, exists := pm.portAssign[number]
	if !exists {
		return "", 0, false
	}

	phoneNumber, assigned := pm.assigned[number]
	if !assigned {
		return "", 0, false
	}

	delete(pm.assigned, number)

	if portPhones, exists := pm.portMap[port]; exists {
		if _, exists := portPhones[phoneNumber]; exists {
			delete(portPhones, phoneNumber)
			pm.data = pm.removeEntryFromData(phoneNumber, port)
			if len(portPhones) == 0 {
				log.Printf("INFO: Port %d has been exhausted and freed.", port)
				pm.freePort(port)
			}
		}
	}

	pm.resetPortTimer(port)
	pm.needsSave = true
	log.Printf("INFO: Deleted number %d -> phone %s (port %d).", number, phoneNumber, port)
	return phoneNumber, port, true
}

// removeEntryFromData removes the entry with the given phone number and port from the data slice
func (pm *PortManager) removeEntryFromData(phoneNumber string, port int) []Entry {
	updatedData := make([]Entry, 0, len(pm.data))
	for _, entry := range pm.data {
		if entry.PhoneNumber == phoneNumber && entry.Port == port {
			continue
		}
		updatedData = append(updatedData, entry)
	}
	return updatedData
}

// resetPortTimer resets the timer for a port, or clears it if the port has been freed
func (pm *PortManager) resetPortTimer(port int) {
	pm.clearPortTimer(port)
	if _, busy := pm.busyPorts[port]; busy {
		pm.portTimers[port] = time.AfterFunc(portTimeout, func() {
			pm.assignLock.Lock()
			defer pm.assignLock.Unlock()
			pm.freePort(port)
			log.Printf("INFO: Port %d has been freed due to timeout.", port)
			pm.reinitializePort(port)
		})
	}
}

// clearPortTimer clears the timer for a port
func (pm *PortManager) clearPortTimer(port int) {
	if timer, exists := pm.portTimers[port]; exists {
		timer.Stop()
		delete(pm.portTimers, port)
	}
}

// freePort frees a port, including marking it as exhausted if no phone numbers are left
func (pm *PortManager) freePort(port int) {
	delete(pm.busyPorts, port)
	delete(pm.portAssign, port)
	if !pm.hasAvailableNumbers(port) {
		delete(pm.portMap, port)
	}
	pm.needsSave = true
}

// reinitializePort re-adds the port to the available list if it has available phone numbers
func (pm *PortManager) reinitializePort(port int) {
	if pm.hasAvailableNumbers(port) {
		pm.busyPorts[port] = false
	}
}

// hasAvailableNumbers checks if a port still has available phone numbers
func (pm *PortManager) hasAvailableNumbers(port int) bool {
	for _, used := range pm.portMap[port] {
		if !used {
			return true
		}
	}
	return false
}

// FreeAllPorts frees all ports
func (pm *PortManager) FreeAllPorts() {
	pm.assignLock.Lock()
	defer pm.assignLock.Unlock()

	for port := range pm.busyPorts {
		pm.freePort(port)
	}
	pm.assigned = make(map[int]string)
	pm.portAssign = make(map[int]int)
	pm.portTimers = make(map[int]*time.Timer)
	pm.needsSave = true
	log.Println("INFO: All ports have been freed.")
}

// respondWithError sends an error response in JSON format
func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	response := map[string]string{
		"status":  "error",
		"message": message,
	}
	_ = json.NewEncoder(w).Encode(response)
}

// respondWithJSON sends a success response in JSON format
func respondWithJSON(w http.ResponseWriter, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

// validateNumberField validates the number field in an API request
func validateNumberField(request map[string]interface{}) (int, bool) {
	numberFloat, ok := request["number"].(float64)
	if !ok || numberFloat < 0 {
		return 0, false
	}
	return int(numberFloat), true
}

// AssignHandler handles requests to assign a phone number to a port
func AssignHandler(w http.ResponseWriter, r *http.Request) {
	portManager.wg.Add(1)
	defer portManager.wg.Done()

	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Invalid request method")
		return
	}

	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	number, valid := validateNumberField(request)
	if !valid {
		respondWithError(w, http.StatusBadRequest, "Invalid 'number' field")
		return
	}

	phoneNumber, port, assigned := portManager.AssignPhoneNumber(number)
	if assigned {
		respondWithJSON(w, map[string]interface{}{
			"status":  "success",
			"message": "Phone number assigned successfully",
			"data": map[string]interface{}{
				"number":      number,
				"phone":       phoneNumber,
				"port":        port,
				"action":      "assigned",
				"csv_updated": portManager.needsSave,
			},
		})
	} else {
		respondWithError(w, http.StatusNotFound, "No available phone number")
	}
}

// DeleteHandler handles requests to delete a phone number assignment
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	portManager.wg.Add(1)
	defer portManager.wg.Done()

	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Invalid request method")
		return
	}

	var request map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	number, valid := validateNumberField(request)
	if !valid {
		respondWithError(w, http.StatusBadRequest, "Invalid 'number' field")
		return
	}

	phoneNumber, port, deleted := portManager.DeletePhoneNumber(number)
	if deleted {
		if portManager.needsSave {
			if err := portManager.SaveCSV(portManager.csvFile); err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to save updated CSV")
				return
			}
		}
		respondWithJSON(w, map[string]interface{}{
			"status":  "success",
			"message": "Phone number deleted successfully",
			"data": map[string]interface{}{
				"number":      number,
				"phone":       phoneNumber,
				"port":        port,
				"action":      "deleted",
				"csv_updated": portManager.needsSave,
			},
		})
	} else {
		respondWithError(w, http.StatusNotFound, "Phone number not found")
	}
}

// FreeAllHandler handles requests to free all ports
func FreeAllHandler(w http.ResponseWriter, r *http.Request) {
	portManager.wg.Add(1)
	defer portManager.wg.Done()

	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Invalid request method")
		return
	}

	go portManager.FreeAllPorts()

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "All ports are being freed",
	})
}

func main() {
	portManager = NewPortManager()

	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("ERROR: Failed to get current working directory: %v", err)
	}

	var csvFile string
	var updatedFileFound bool

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(info.Name(), "updated_") && strings.HasSuffix(info.Name(), ".csv") {
			csvFile = path
			updatedFileFound = true
			return filepath.SkipDir
		}
		if !updatedFileFound && csvFile == "" && strings.HasSuffix(info.Name(), ".csv") {
			csvFile = path
		}
		return nil
	})

	if csvFile == "" {
		log.Fatalf("ERROR: No CSV file found in directory: %s", dir)
	}

	if err := portManager.LoadCSV(csvFile); err != nil {
		log.Fatalf("ERROR: Failed to load CSV file '%s': %v", csvFile, err)
	}

	if !updatedFileFound {
		updatedFile := "updated_" + filepath.Base(csvFile)
		csvFile = filepath.Join(filepath.Dir(csvFile), updatedFile)
		portManager.csvFile = csvFile
	}

	http.HandleFunc("/assign", AssignHandler)
	http.HandleFunc("/delete", DeleteHandler)
	http.HandleFunc("/freeall", FreeAllHandler)

	server := &http.Server{
		Addr: ":3055",
	}

	go func() {
		log.Printf("INFO: Starting server on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ERROR: Server failed to listen on %s: %v", server.Addr, err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	gracefulShutdown(server, stop)

	portManager.wg.Wait()

	if portManager.needsSave {
		if err := portManager.SaveCSV(portManager.csvFile); err != nil {
			log.Printf("ERROR: Failed to save updated CSV: %v", err)
		}
	}

	log.Println("INFO: Server exited gracefully")
}

// gracefulShutdown handles server shutdown
func gracefulShutdown(server *http.Server, stop <-chan os.Signal) {
	<-stop
	log.Println("INFO: Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("ERROR: Server shutdown error: %v", err)
	}
	log.Println("INFO: Server shutdown complete")
}
