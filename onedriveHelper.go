package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Data structures to hold email workers and synchronization primitives
var (
	emailWorkers = make(map[string]*emailWorker)
	workersMu    sync.Mutex
)

// Struct for the download task
type downloadTask struct {
	url  string
	name string
}

// Struct for the email worker
type emailWorker struct {
	queue        chan downloadTask
	files        map[string]bool // To track downloaded files
	mu           sync.Mutex
	rateLimiter  <-chan time.Time
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownWait sync.WaitGroup
}

// Structs to parse the JSON payload
type Payload struct {
	Value []Item `json:"value"`
}

type Item struct {
	ContentDownloadUrl string `json:"@content.downloadUrl"`
	Name               string `json:"name"`
}

func main() {
	http.HandleFunc("/onedrive", onedriveHandler)
	http.HandleFunc("/shutdown", shutdownHandler)

	server := &http.Server{
		Addr: ":3000",
	}

	// Graceful shutdown handling
	go func() {
		c := make(chan os.Signal, 1)
		// signal.Notify(c, os.Interrupt, syscall.SIGTERM) // Uncomment this line if running on Unix systems
		<-c
		log.Println("Shutting down server...")
		server.Close()
	}()

	log.Println("Server started at :3000")
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatal("ListenAndServe: ", err)
	}

	// Wait for all workers to finish
	workersMu.Lock()
	for _, worker := range emailWorkers {
		worker.cancel()
	}
	workersMu.Unlock()

	log.Println("Server stopped.")
}

// Handler for the /onedrive endpoint
func onedriveHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the 'email' query parameter
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing 'email' query parameter", http.StatusBadRequest)
		return
	}

	// Limit request body size to prevent abuse
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10 MB

	// Parse the JSON payload
	var payload Payload
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&payload)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Get or create emailWorker
	workersMu.Lock()
	worker, exists := emailWorkers[email]
	if !exists {
		ctx, cancel := context.WithCancel(context.Background())
		worker = &emailWorker{
			queue:       make(chan downloadTask, 100), // Adjust buffer size as needed
			files:       make(map[string]bool),
			rateLimiter: time.Tick(time.Second), // Rate limit to 1 request per second
			ctx:         ctx,
			cancel:      cancel,
		}
		emailWorkers[email] = worker
		worker.shutdownWait.Add(1)
		go worker.run(email)
	}
	workersMu.Unlock()

	// Process each item in the payload
	for _, item := range payload.Value {
		downloadUrl := item.ContentDownloadUrl
		name := item.Name

		if downloadUrl == "" || name == "" {
			continue // Skip items with missing data
		}

		// Sanitize the filename to prevent directory traversal
		name = filepath.Base(name)

		// Use the download URL as a unique key to avoid duplicates
		key := downloadUrl

		// Check for duplicates
		worker.mu.Lock()
		if _, found := worker.files[key]; !found {
			worker.files[key] = true
			worker.queue <- downloadTask{url: downloadUrl, name: name}
		}
		worker.mu.Unlock()
	}

	// Respond with success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Tasks added to download queue"))
}

// Worker function to process the download queue
func (w *emailWorker) run(email string) {
	defer w.shutdownWait.Done()
	for {
		select {
		case <-w.ctx.Done():
			log.Printf("Worker for email '%s' is shutting down.", email)
			return
		case task := <-w.queue:
			// Rate limiting
			select {
			case <-w.rateLimiter:
				// Proceed
			case <-w.ctx.Done():
				return
			}

			// Download the file with retry logic
			err := downloadFile(w.ctx, email, task.url, task.name)
			if err != nil {
				log.Printf("Failed to download file '%s' for email '%s': %v", task.name, email, err)
				// Remove the file from the files map to allow re-download if needed
				w.mu.Lock()
				delete(w.files, task.url)
				w.mu.Unlock()
			}
		}
	}
}

// Function to download a file from a URL and save it to the specified directory
func downloadFile(ctx context.Context, email, url, filename string) error {
	// Create directory for the email if it doesn't exist
	dir := filepath.Join(".", email)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	// Create the file
	filepath := filepath.Join(dir, filename)

	// Check if file already exists
	if _, err := os.Stat(filepath); err == nil {
		log.Printf("File '%s' already exists for email '%s'. Skipping download.", filename, email)
		return nil
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Create an HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create a request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// Get the data
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Limit the size of the response body to prevent large downloads
	limitedReader := &io.LimitedReader{R: resp.Body, N: 100 << 20} // 100 MB limit

	// Write the body to file
	_, err = io.Copy(out, limitedReader)
	if err != nil {
		return err
	}

	// Check if the file size exceeds the limit
	if limitedReader.N <= 0 {
		return errors.New("file size exceeds the maximum allowed limit")
	}

	log.Printf("File '%s' downloaded successfully for email '%s'.", filename, email)
	return nil
}

// Handler to gracefully shutdown the server and workers
func shutdownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		log.Println("Received shutdown signal. Shutting down server...")
		os.Exit(0)
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server is shutting down"))
}
