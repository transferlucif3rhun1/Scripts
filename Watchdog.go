package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// ExeConfig defines the configuration for each executable to monitor
type ExeConfig struct {
	Name           string
	ServiceURL     string
	ExpectedStatus int
	Check          bool // Indicates whether to perform service URL checks
}

// Initialize the list of executables to monitor
var exeConfigs = []ExeConfig{
	{Name: "authServer.exe", ServiceURL: "http://localhost:3001", ExpectedStatus: 404, Check: false},
	{Name: "tmHelper.exe", ServiceURL: "http://localhost:3081", ExpectedStatus: 404, Check: true},
}

// isWindows checks if the OS is Windows
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// isProcessRunning checks if the executable is currently running
func isProcessRunning(processName string) bool {
	processes, err := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", processName)).Output()
	if err != nil {
		log.Printf("[ERROR] Failed to query tasklist for %s: %v", processName, err)
		return false
	}
	return strings.Contains(strings.ToLower(string(processes)), strings.ToLower(processName))
}

// terminateProcess terminates the process with the given PID
func terminateProcess(pid int, processName string) {
	process, err := os.FindProcess(pid)
	if err != nil {
		log.Printf("[ERROR] Failed to find process %s: %v", processName, err)
		return
	}

	if isWindows() {
		// On Windows, use process.Kill()
		if err := process.Kill(); err != nil {
			log.Printf("[ERROR] Failed to kill process %s: %v", processName, err)
		} else {
			log.Printf("[INFO] Process %s terminated successfully.", processName)
		}
	} else {
		// On Unix-like systems, send SIGTERM
		if err := process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("[ERROR] Failed to send SIGTERM to %s: %v", processName, err)
		} else {
			log.Printf("[INFO] SIGTERM sent to %s.", processName)
		}
	}
}

// checkServiceURL makes an HTTP GET request to validate the service URL
func checkServiceURL(serviceURL string, expectedStatus int, processName string) bool {
	resp, err := http.Get(serviceURL)
	if err != nil {
		log.Printf("[ERROR] Failed to reach service for %s: %v", processName, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		log.Printf("[ERROR] Service for %s returned unexpected status code: got %d, expected %d", processName, resp.StatusCode, expectedStatus)
		return false
	}
	return true
}

// StartWorker launches the worker's monitoring routine
func StartWorker(ctx context.Context, wg *sync.WaitGroup, config ExeConfig) {
	defer wg.Done()
	log.Printf("[INFO] Worker started for %s.", config.Name)

	exePath, err := filepath.Abs(config.Name)
	if err != nil {
		log.Printf("[ERROR] Failed to get absolute path for %s: %v", config.Name, err)
		return
	}

	restartCount := 0
	maxRestarts := 5
	backoffFactor := 1 * time.Second
	var cmd *exec.Cmd
	serviceHealthy := false

	var startExecutable func() error
	var handleRestart func()

	handleRestart = func() {
		restartCount++
		if restartCount > maxRestarts {
			log.Printf("[CRITICAL] %s has failed to restart after %d attempts. Giving up.", config.Name, maxRestarts)
			return
		}

		backoffDuration := backoffFactor * time.Duration(restartCount)
		log.Printf("[INFO] Restarting %s (Attempt %d) after %v...", config.Name, restartCount, backoffDuration)
		time.Sleep(backoffDuration)

		// Terminate existing process if running
		if cmd != nil && cmd.Process != nil {
			terminateProcess(cmd.Process.Pid, config.Name)
		}

		if err := startExecutable(); err != nil {
			handleRestart()
		}
	}

	startExecutable = func() error {
		log.Printf("[INFO] Starting %s...", config.Name)
		cmd = exec.Command(exePath)
		if isWindows() {
			cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("[ERROR] Failed to start %s: %v", config.Name, err)
			return err
		}

		log.Printf("[INFO] %s started successfully with PID %d.", config.Name, cmd.Process.Pid)

		// Allow time for the process to initialize
		time.Sleep(10 * time.Second)

		// If service check is enabled, validate after startup
		if config.Check {
			serviceHealthy = checkServiceURL(config.ServiceURL, config.ExpectedStatus, config.Name)
			if serviceHealthy {
				log.Printf("[INFO] Service for %s is healthy.", config.Name)
			} else {
				log.Printf("[ERROR] Service for %s is unhealthy after startup.", config.Name)
				return fmt.Errorf("service URL check failed after startup")
			}
		}

		// Reset restart count on successful start
		restartCount = 0
		return nil
	}

	// Initial start
	if err := startExecutable(); err != nil {
		handleRestart()
	}

	// Monitoring loop
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[INFO] Worker for %s received shutdown signal.", config.Name)
			return
		case <-ticker.C:
			if !isProcessRunning(config.Name) {
				log.Printf("[WARNING] %s is not running. Attempting to restart...", config.Name)
				handleRestart()
			} else if config.Check {
				healthy := checkServiceURL(config.ServiceURL, config.ExpectedStatus, config.Name)
				if healthy != serviceHealthy {
					if healthy {
						log.Printf("[INFO] Service for %s is now healthy.", config.Name)
					} else {
						log.Printf("[ERROR] Service for %s became unhealthy. Restarting...", config.Name)
					}
					serviceHealthy = healthy
					if !healthy {
						handleRestart()
					}
				}
			}
		}
	}
}

func main() {
	// Set log to include timestamps
	log.SetFlags(log.LstdFlags)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Start Workers
	for _, config := range exeConfigs {
		if !config.Check {
			continue
		}
		wg.Add(1)
		go StartWorker(ctx, &wg, config)
	}

	// Setup signal capturing
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Wait for a termination signal
	sig := <-signalChan
	log.Printf("[INFO] Received signal: %s. Initiating shutdown...", sig)
	cancel()

	// Wait for all workers to finish
	wg.Wait()
	log.Println("[INFO] All workers have been gracefully shut down. Exiting.")
}
