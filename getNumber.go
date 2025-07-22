package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type RequestPayload struct {
	Number int `json:"number"`
}

type ResponsePayload struct {
	PhoneNumber string `json:"phone_number"`
}

var (
	phoneNumbers []string
	once         sync.Once
)

func main() {
	http.HandleFunc("/get-number", handleRequest)
	fmt.Println("Server listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8010", nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var payload RequestPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		http.Error(w, "Error parsing JSON request", http.StatusBadRequest)
		log.Printf("Error parsing JSON request: %v", err)
		return
	}

	loadPhoneNumbers("numbers.txt")

	var responsePayload ResponsePayload
	if payload.Number >= 0 && payload.Number < len(phoneNumbers) {
		responsePayload.PhoneNumber = phoneNumbers[payload.Number]
	} else {
		responsePayload.PhoneNumber = "Invalid index"
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(responsePayload)
	if err != nil {
		http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func loadPhoneNumbers(filename string) {
	once.Do(func() {
		var err error
		phoneNumbers, err = readPhoneNumbers(filename)
		if err != nil {
			log.Fatalf("Error reading phone numbers file: %v", err)
		}
	})
}

func readPhoneNumbers(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	var phoneNumbers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 {
			phoneNumbers = append(phoneNumbers, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return phoneNumbers, nil
}
