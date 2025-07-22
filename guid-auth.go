package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// AuthData represents the structure to hold the name and GUID
type AuthData struct {
	Name    string `json:"name"`
	GUID    string `json:"guid,omitempty"`
	Flagged bool   `json:"flagged"`
}

// AuthServer represents the server structure with a mutex for thread-safe access
type AuthServer struct {
	sync.RWMutex
	data     map[string]AuthData
	dataFile string
}

// NewAuthServer initializes a new AuthServer and ensures the data file is ready
func NewAuthServer(dataFile string) *AuthServer {
	server := &AuthServer{
		data:     make(map[string]AuthData),
		dataFile: dataFile,
	}
	server.ensureFileExists()
	server.loadFromFile()
	return server
}

// ensureFileExists checks if the data file exists; if not, it creates an empty one
func (s *AuthServer) ensureFileExists() {
	if _, err := os.Stat(s.dataFile); os.IsNotExist(err) {
		file, err := os.Create(s.dataFile)
		if err != nil {
			log.Fatalf("Failed to create data file: %v", err)
		}
		defer file.Close()

		_, err = file.Write([]byte("{}"))
		if err != nil {
			log.Fatalf("Failed to write to data file: %v", err)
		}
	}
}

// loadFromFile loads the stored data from a file and creates a test user if the file is empty
func (s *AuthServer) loadFromFile() {
	s.Lock()
	defer s.Unlock()

	bytes, err := os.ReadFile(s.dataFile)
	if err != nil {
		log.Fatalf("Failed to read data file: %v", err)
	}

	if len(bytes) == 0 || string(bytes) == "{}" {
		s.data = make(map[string]AuthData)
		// Unlock before calling saveToFile to avoid deadlock
		s.Unlock()
		s.createTestUser()
		s.Lock()
	} else {
		err = json.Unmarshal(bytes, &s.data)
		if err != nil {
			log.Fatalf("Failed to parse data file: %v", err)
		}
	}
}

// createTestUser creates a test user if no data is available
func (s *AuthServer) createTestUser() {
	s.Lock()
	s.data["testuser"] = AuthData{
		Name:    "testuser",
		GUID:    uuid.New().String(),
		Flagged: false,
	}
	s.Unlock()
	s.saveToFile()
}

// saveToFile saves the data map to a file in a readable format
func (s *AuthServer) saveToFile() {
	s.Lock()
	defer s.Unlock()

	bytes, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal data: %v", err)
		return
	}

	err = os.WriteFile(s.dataFile, bytes, 0644)
	if err != nil {
		log.Printf("Failed to write data to file: %v", err)
	}
}

// sanitizeInput sanitizes the input to prevent injection attacks and other malicious data
func sanitizeInput(input string) string {
	return strings.TrimSpace(strings.ToLower(input))
}

// validateName checks if the name provided is valid
func validateName(name string) error {
	if len(name) == 0 {
		return errors.New("name cannot be empty")
	}
	if len(name) > 100 {
		return errors.New("name is too long")
	}
	return nil
}

// generateGUID generates a new GUID or uses the provided one if the name does not exist, otherwise returns an error
func (s *AuthServer) generateGUID(name, guid string) (string, error) {
	s.Lock()
	defer s.Unlock()

	name = sanitizeInput(name)

	if err := validateName(name); err != nil {
		return "", err
	}

	if _, exists := s.data[name]; exists {
		return "", errors.New("name already exists")
	}

	if guid == "" {
		guid = uuid.New().String()
	}

	s.data[name] = AuthData{Name: name, GUID: guid, Flagged: false}

	go s.saveToFile()

	log.Printf("Stored GUID for name %s: %s", name, guid)
	return guid, nil
}

// verifyGUID checks if the name and GUID match the stored data
func (s *AuthServer) verifyGUID(name, guid string) bool {
	s.RLock()
	defer s.RUnlock()

	name = sanitizeInput(name)

	if authData, exists := s.data[name]; exists {
		if authData.GUID == guid && !authData.Flagged {
			return true
		}
	}
	return false
}

// flagGUID flags a specific name and optionally a GUID combination
func (s *AuthServer) flagGUID(name, guid string) error {
	s.Lock()
	defer s.Unlock()

	name = sanitizeInput(name)

	if err := validateName(name); err != nil {
		return err
	}

	authData, exists := s.data[name]
	if !exists {
		return errors.New("name not found")
	}

	if authData.Flagged {
		return nil // Already flagged, do nothing
	}

	if guid == "" || authData.GUID == guid {
		authData.Flagged = true
		s.data[name] = authData
		go s.saveToFile()
		log.Printf("Flagged GUID %s for name %s", guid, name)
		return nil
	}

	return errors.New("GUID does not match")
}

// generateHandler handles requests to the /generate endpoint
func (s *AuthServer) generateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	guid := r.URL.Query().Get("guid")

	if name == "" {
		http.Error(w, "Bad request: name is required", http.StatusBadRequest)
		return
	}

	generatedGUID, err := s.generateGUID(name, guid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict) // HTTP 409 Conflict for existing names or invalid GUIDs
		return
	}

	response := map[string]string{"name": name, "guid": generatedGUID}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// authHandler handles requests to the /auth endpoint
func (s *AuthServer) authHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	guid := r.URL.Query().Get("guid")

	if name == "" || guid == "" {
		http.Error(w, "Bad request: name and guid are required", http.StatusBadRequest)
		return
	}

	if s.verifyGUID(name, guid) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "valid"})
	} else {
		http.Error(w, "Invalid name or GUID", http.StatusBadRequest)
	}
}

// healthHandler handles requests to the /health endpoint
func (s *AuthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// flagHandler handles requests to the /flag endpoint
func (s *AuthServer) flagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	guid := r.URL.Query().Get("guid")

	if name == "" {
		http.Error(w, "Bad request: name is required", http.StatusBadRequest)
		return
	}

	err := s.flagGUID(name, guid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "flagged"})
}

// Start runs the server
func (s *AuthServer) Start() {
	http.HandleFunc("/generate", s.generateHandler)
	http.HandleFunc("/auth", s.authHandler)
	http.HandleFunc("/flag", s.flagHandler)
	http.HandleFunc("/health", s.healthHandler)

	log.Println("Auth server running on 0.0.0.0:8030")
	if err := http.ListenAndServe("0.0.0.0:8030", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func main() {
	dataFile := "datafile.json"
	server := NewAuthServer(dataFile)
	server.Start()
}
