package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
    // Store the blocked state of numbers along with the time they were blocked
    NumberState = make(map[int64]bool)
    BlockTime   = make(map[int64]time.Time)
    mutex       = &sync.Mutex{}
)

func blockNumber(w http.ResponseWriter, r *http.Request) {
    mutex.Lock()
    defer mutex.Unlock()

    query := r.URL.Query()
    numberStr := query.Get("number")
    if numberStr == "" {
        log.Println("Error: Number parameter is missing")
        http.Error(w, "Number parameter is missing", http.StatusBadRequest)
        return
    }

    number, err := strconv.ParseInt(numberStr, 10, 64)
    if err != nil {
        log.Println("Error: Invalid number format")
        http.Error(w, "Invalid number format", http.StatusBadRequest)
        return
    }

    if blocked, exists := NumberState[number]; exists && blocked {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"error": "Number already blocked."})
        return
    }

    NumberState[number] = true
    BlockTime[number] = time.Now()
    log.Printf("Number blocked: %d\n", number)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"success": "Number Blocked: " + strconv.FormatInt(number, 10)})
}

func releaseNumber(w http.ResponseWriter, r *http.Request) {
    mutex.Lock()
    defer mutex.Unlock()

    query := r.URL.Query()
    numberStr := query.Get("number")
    if numberStr == "" {
        log.Println("Error: Number parameter is missing")
        http.Error(w, "Number parameter is missing", http.StatusBadRequest)
        return
    }

    number, err := strconv.ParseInt(numberStr, 10, 64)
    if err != nil {
        log.Println("Error: Invalid number format")
        http.Error(w, "Invalid number format", http.StatusBadRequest)
        return
    }

    if blocked, exists := NumberState[number]; !exists || !blocked {
        log.Printf("Error: Number not found or not blocked: %d\n", number)
        http.Error(w, "Number not found or not blocked", http.StatusBadRequest)
        return
    }

    NumberState[number] = false
    delete(BlockTime, number) // Remove the timestamp when releasing a number
    log.Printf("Number released: %d\n", number)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"success": "Number Released: " + strconv.FormatInt(number, 10)})
}

func releaseAll(w http.ResponseWriter, r *http.Request) {
    mutex.Lock()
    defer mutex.Unlock()

    for number, blocked := range NumberState {
        if blocked {
            NumberState[number] = false
            delete(BlockTime, number) // Remove the timestamp for each number being released
        }
    }

    log.Println("All numbers released")
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"success": "All numbers released"})
}

func autoRelease() {
    for {
        time.Sleep(10 * time.Second) // Check every 10 seconds

        mutex.Lock()
        for number, timestamp := range BlockTime {
            if time.Since(timestamp).Seconds() > 60 && NumberState[number] {
                NumberState[number] = false
                delete(BlockTime, number)
                log.Printf("Number auto-released: %d\n", number)
            }
        }
        mutex.Unlock()
    }
}

func main() {
    // Initialize logging to stdout
    log.SetOutput(os.Stdout)

    go autoRelease() // Start the auto-release routine

    http.HandleFunc("/blockNumber", blockNumber)
    http.HandleFunc("/releaseNumber", releaseNumber)
    http.HandleFunc("/releaseAll", releaseAll)

    log.Println("Server starting on :3030")
    if err := http.ListenAndServe(":3030", nil); err != nil {
        log.Fatalf("Error starting server: %v\n", err)
    }
}
