package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Cookie represents a cookie with a name and value.
type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// convertCookies converts a map of cookies to a slice of Cookie structs, filtering out empty cookies.
func convertCookies(cookies map[string]string) []Cookie {
	result := make([]Cookie, 0, len(cookies))
	for name, value := range cookies {
		if name != "" && value != "" {
			result = append(result, Cookie{Name: name, Value: value})
		}
	}
	return result
}

// convertCookiesHandler handles the /convertCookies endpoint.
func convertCookiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var cookies map[string]string
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&cookies)
	if err != nil {
		http.Error(w, "Failed to decode JSON", http.StatusBadRequest)
		log.Printf("Failed to decode JSON: %v", err)
		return
	}

	result := convertCookies(cookies)
	response, err := json.Marshal(result)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		log.Printf("Failed to encode JSON: %v", err)
		return
	}

	// Remove the enclosing brackets from the JSON array
	responseStr := string(response)
	responseStr = responseStr[1 : len(responseStr)-1]

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(responseStr))
}

func main() {
	http.HandleFunc("/convertCookies", convertCookiesHandler)

	server := &http.Server{
		Addr:           ":3000",
		Handler:        nil,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Using a WaitGroup to handle concurrency gracefully
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Server is running on http://localhost:3000")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on :3000: %v\n", err)
		}
	}()

	<-stop // Wait for interrupt signal

	// Gracefully shutdown the server
	fmt.Println("Shutting down the server...")
	if err := server.Close(); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}

	// Wait for the server to finish
	wg.Wait()
	fmt.Println("Server stopped gracefully")
}
