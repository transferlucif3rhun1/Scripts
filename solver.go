package main

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Define the rate limit and keys structure
var rateLimits = map[string]int{
	"Liang": 1, // Liang key is allowed 2000 requests.
}

// Global map to keep track of the current request count
var requestCount = make(map[string]int)

// Structs for handling incoming request payload and making API requests
type Payload struct {
	Key        string `json:"key"`
	WebsiteURL string `json:"websiteurl"`
	WebsiteKey string `json:"websitekey"`
}

type Task struct {
	Type        string `json:"type"`
	WebsiteURL  string `json:"websiteURL"`
	WebsiteKey  string `json:"websiteKey"`
}

type TokenRequest struct {
	ClientKey string `json:"clientKey"`
	AppId     string `json:"appId"`
	Task      Task   `json:"task"`
}

// Response struct to parse the response from capsolver
type TokenResponse struct {
	ErrorId     int    `json:"errorId"`
	TaskId      string `json:"taskId"`
	Solution    struct {
		GRecaptchaResponse string `json:"gRecaptchaResponse"`
	} `json:"solution"`
	Status string `json:"status"`
}

// Check if the key is valid and within request limits
func isKeyValid(key string) bool {
	limit, exists := rateLimits[key]
	if !exists {
		return false
	}

	// Check if the key has exceeded the allowed limit
	if requestCount[key] >= limit {
		return false
	}
	return true
}

// Increment request count for the given key
func incrementRequestCount(key string) {
	requestCount[key]++
}

// Function to make request to the external API (POST https://api.capsolver.com/getToken)
func makeExternalRequest(clientKey, appId string, task Task) (*TokenResponse, error) {
	url := "https://api.capsolver.com/getToken"

	// Create the request payload with clientKey, appId, and task data
	reqBody := TokenRequest{
		ClientKey: clientKey,
		AppId:     appId,
		Task:      task,
	}

	// Marshal the request to JSON
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	// Make the POST request
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decode the response into a TokenResponse
	var respBody TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, err
	}

	// Return the parsed response
	return &respBody, nil
}

// Gin handler for the request
func handleRequest(c *gin.Context) {
	// Parse the incoming JSON payload
	var payload Payload
	err := c.ShouldBindJSON(&payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Check if key is valid and not exceeded its limit
	if !isKeyValid(payload.Key) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or rate-limited key"})
		return
	}

	// Increment the request count for the key
	incrementRequestCount(payload.Key)

	// Prepare the task to send to the external API
	task := Task{
		Type:        "ReCaptchaV3TaskProxyLess",
		WebsiteURL:  payload.WebsiteURL,
		WebsiteKey:  payload.WebsiteKey,
	}

	// Define your clientKey and appId (replace with your actual values)
	clientKey := "CAP-D7C32401576D5CF6C598FD919A4EF67E" // Replace this with your actual API key
	appId := "0763DBE0-862C-4631-B5D7-4CEF70D6F375"      // Replace this with your actual appId

	// Make the external request
	respBody, err := makeExternalRequest(clientKey, appId, task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch token", "details": err.Error()})
		return
	}

	// If the status is "processing", return an error indicating that the CAPTCHA is still being solved
	if respBody.Status == "processing" {
		c.JSON(http.StatusOK, gin.H{"error": "Captcha not solvable at this moment. Try again later."})
		return
	}

	// If the response contains a solution and the status is "ready", return the gRecaptchaResponse
	if respBody.ErrorId == 0 && respBody.Status == "ready" {
		c.JSON(http.StatusOK, gin.H{"token": respBody.Solution.GRecaptchaResponse})
	} else {
		// If there's any issue with the response (errorId != 0 or status != ready)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "Captcha not solvable",
			"details": respBody,
		})
	}
}

func main() {
	// Set up the Gin router
	r := gin.Default()

	// Create a route for handling requests
	r.POST("/request", handleRequest)

	// Start the server
	r.Run(":8080")
}
