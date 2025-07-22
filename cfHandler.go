package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/sync/singleflight"
)

type Proxy struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type RequestPayload struct {
	URL      string `json:"url" binding:"required,url"`
	Proxy    string `json:"proxy" binding:"required"`
	IP       string `json:"ip" binding:"required"`
	Headless bool   `json:"headless"` // Default: false
}

type EnhancedResponse map[string]interface{}

type cacheEntry struct {
	response    EnhancedResponse
	cachedAt    time.Time
	expireAfter time.Duration
}

type Config struct {
	CacheKeyFields []string
}

type CloudflareResponse struct {
	Success      bool        `json:"success"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	Cookies      interface{} `json:"cookies,omitempty"`
	CookieHeader string      `json:"cookieHeader,omitempty"`
	Turnstile    string      `json:"turnstile,omitempty"`
	UserAgent    string      `json:"userAgent,omitempty"`
	ResponseTime int         `json:"responseTime,omitempty"`
}

var (
	cache           *lru.Cache
	cacheMutex      sync.RWMutex
	cacheExpiration = 24 * time.Hour // Cache for 24 hours, until manually deleted
	requestGroup    singleflight.Group
	appConfig       Config
	httpClient      *http.Client
	responsePool    = sync.Pool{
		New: func() interface{} {
			return make(EnhancedResponse)
		},
	}
	bufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}
)

func loadConfig() Config {
	fields := os.Getenv("CACHE_KEY_FIELDS")
	if fields == "" {
		return Config{
			CacheKeyFields: []string{"proxy", "ip"},
		}
	}

	parts := strings.Split(fields, ",")
	var cacheFields []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(part))
		if trimmed == "url" || trimmed == "proxy" || trimmed == "ip" || trimmed == "headless" {
			cacheFields = append(cacheFields, trimmed)
		}
	}

	if len(cacheFields) == 0 {
		cacheFields = []string{"proxy", "ip"}
	}

	return Config{
		CacheKeyFields: cacheFields,
	}
}

func init() {
	appConfig = loadConfig()
	var err error
	cache, err = lru.New(1000)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}

	// Optimized HTTP client with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	httpClient = &http.Client{
		Timeout:   90 * time.Second,
		Transport: transport,
	}
}

func isValidIPOrHostname(addr string) bool {
	// Check if it's a valid IP address
	if net.ParseIP(addr) != nil {
		return true
	}

	// Check for common localhost variants
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "localhost" {
		return true
	}

	// Basic hostname validation (alphanumeric, dots, hyphens)
	if len(addr) > 0 && len(addr) <= 253 {
		for _, char := range addr {
			if !((char >= 'a' && char <= 'z') ||
				(char >= '0' && char <= '9') ||
				char == '.' || char == '-' || char == '_') {
				return false
			}
		}
		return true
	}

	return false
}

func parseProxy(proxyStr string) (Proxy, error) {
	parts := strings.Split(proxyStr, ":")
	if len(parts) != 4 {
		return Proxy{}, errors.New("proxy format invalid; expected ip:port:user:pass")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return Proxy{}, errors.New("port must be an integer")
	}

	return Proxy{
		Host:     parts[0],
		Port:     port,
		Username: parts[2],
		Password: parts[3],
	}, nil
}

func generateCacheKey(payload RequestPayload, config Config) string {
	var keyParts []string
	for _, field := range config.CacheKeyFields {
		switch field {
		case "url":
			keyParts = append(keyParts, payload.URL)
		case "proxy":
			keyParts = append(keyParts, payload.Proxy)
		case "ip":
			keyParts = append(keyParts, payload.IP)
		case "headless":
			keyParts = append(keyParts, strconv.FormatBool(payload.Headless))
		}
	}
	return strings.Join(keyParts, "|")
}

func getCachedResponse(key string) (EnhancedResponse, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	value, ok := cache.Get(key)
	if !ok {
		return nil, false
	}

	entry, ok := value.(cacheEntry)
	if !ok || time.Since(entry.cachedAt) > entry.expireAfter {
		// Remove expired entry in separate goroutine to avoid write lock
		go func() {
			cacheMutex.Lock()
			cache.Remove(key)
			cacheMutex.Unlock()
		}()
		return nil, false
	}

	// Create copy to avoid data races
	response := make(EnhancedResponse)
	for k, v := range entry.response {
		response[k] = v
	}

	return response, true
}

func setCachedResponse(key string, response EnhancedResponse) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Create copy for cache to avoid mutations
	cachedResponse := make(EnhancedResponse)
	for k, v := range response {
		cachedResponse[k] = v
	}

	cache.Add(key, cacheEntry{
		response:    cachedResponse,
		cachedAt:    time.Now(),
		expireAfter: cacheExpiration,
	})
}

func normalizeCookies(cookies interface{}) []map[string]string {
	if cookies == nil {
		return []map[string]string{}
	}

	var result []map[string]string

	switch v := cookies.(type) {
	case map[string]interface{}:
		for name, value := range v {
			if cookieValue, ok := value.(string); ok && name != "" {
				result = append(result, map[string]string{
					"name":  name,
					"value": cookieValue,
				})
			}
		}
	case []interface{}:
		for _, item := range v {
			if cookieMap, ok := item.(map[string]interface{}); ok {
				cookie := make(map[string]string)
				if name, ok := cookieMap["name"].(string); ok && name != "" {
					cookie["name"] = name
				}
				if value, ok := cookieMap["value"].(string); ok {
					cookie["value"] = value
				}
				if domain, ok := cookieMap["domain"].(string); ok && domain != "" {
					cookie["domain"] = domain
				}
				if path, ok := cookieMap["path"].(string); ok && path != "" {
					cookie["path"] = path
				}
				if len(cookie) >= 2 { // Must have at least name and value
					result = append(result, cookie)
				}
			}
		}
	default:
		// Handle unexpected types gracefully
		return []map[string]string{}
	}

	return result
}

func hasCfClearance(response EnhancedResponse) bool {
	// Check if response contains cf_clearance cookie
	if cookies, ok := response["cookies"].([]map[string]string); ok {
		for _, cookie := range cookies {
			if name, exists := cookie["name"]; exists && name == "cf_clearance" {
				if value, hasValue := cookie["value"]; hasValue && value != "" {
					return true
				}
			}
		}
	}

	// Check cookieHeader for cf_clearance
	if cookieHeader, ok := response["cookieHeader"].(string); ok && cookieHeader != "" {
		if strings.Contains(cookieHeader, "cf_clearance=") {
			return true
		}
	}

	return false
}

func sendRequestToCloudflare(ctx context.Context, payload map[string]interface{}, remoteIP string) (EnhancedResponse, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	buffer := bufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	defer bufferPool.Put(buffer)

	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}

	if !isValidIPOrHostname(remoteIP) {
		return nil, errors.New("invalid remote IP address or hostname")
	}

	cloudflareURL := "http://" + remoteIP + ":4200/cloudflare"

	retries := 3
	baseBackoff := 1 * time.Second

	for attempt := 0; attempt < retries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "POST", cloudflareURL, bytes.NewReader(buffer.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connection", "keep-alive")

		resp, err := httpClient.Do(req)
		if err != nil {
			if isRetryableError(err) && attempt < retries-1 {
				backoff := time.Duration(attempt+1) * baseBackoff
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
		resp.Body.Close()

		if err != nil {
			if attempt < retries-1 {
				backoff := time.Duration(attempt+1) * baseBackoff
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			if attempt < retries-1 && resp.StatusCode >= 500 {
				backoff := time.Duration(attempt+1) * baseBackoff
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, errors.New("cloudflare service returned status: " + resp.Status)
		}

		var cloudflareResp CloudflareResponse
		if err := json.Unmarshal(body, &cloudflareResp); err != nil {
			if attempt < retries-1 {
				backoff := time.Duration(attempt+1) * baseBackoff
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		response := responsePool.Get().(EnhancedResponse)
		// Clear the map
		for k := range response {
			delete(response, k)
		}

		response["success"] = cloudflareResp.Success
		if cloudflareResp.ResponseTime > 0 {
			response["responseTime"] = cloudflareResp.ResponseTime
		}

		if cloudflareResp.UserAgent != "" {
			response["userAgent"] = cloudflareResp.UserAgent
		}

		if cloudflareResp.Success {
			if cloudflareResp.Cookies != nil {
				response["cookies"] = normalizeCookies(cloudflareResp.Cookies)
			}

			if cloudflareResp.CookieHeader != "" {
				response["cookieHeader"] = cloudflareResp.CookieHeader
			}

			if cloudflareResp.Turnstile != "" {
				response["turnstile"] = cloudflareResp.Turnstile
			}
		} else {
			errorMsg := cloudflareResp.ErrorMessage
			if errorMsg == "" {
				errorMsg = "cloudflare request failed"
			}
			response["errorMessage"] = errorMsg
		}

		return response, nil
	}

	return nil, errors.New("failed to get response from cloudflare after retries")
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var netErr interface{ Temporary() bool }
	if errors.As(err, &netErr) {
		return netErr.Temporary()
	}

	errStr := strings.ToLower(err.Error())
	retryableKeywords := []string{
		"connection refused", "timeout", "temporary failure",
		"network is unreachable", "connection reset",
		"no route to host", "host is down",
	}

	for _, keyword := range retryableKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

func responseHandler(c *gin.Context) {
	var payload RequestPayload

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Validate IP or hostname
	if !isValidIPOrHostname(payload.IP) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address or hostname"})
		return
	}

	// Ensure headless field is properly handled (explicitly set default)
	// No need for explicit check since Go automatically sets bool to false if not provided

	proxy, err := parseProxy(payload.Proxy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proxy format"})
		return
	}

	cacheKey := generateCacheKey(payload, appConfig)

	if cachedResponse, ok := getCachedResponse(cacheKey); ok {
		cachedResponse["state"] = "cached"
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	response, err, _ := requestGroup.Do(cacheKey, func() (interface{}, error) {
		// Double-check cache within singleflight
		if cachedResp, ok := getCachedResponse(cacheKey); ok {
			return cachedResp, nil
		}

		cloudflarePayload := map[string]interface{}{
			"url":           payload.URL,
			"mode":          "waf",
			"cookiesFormat": "simple",
			"headless":      payload.Headless,
			"proxy": map[string]interface{}{
				"host":     proxy.Host,
				"port":     proxy.Port,
				"username": proxy.Username,
				"password": proxy.Password,
			},
			"timeout": 75000,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 85*time.Second)
		defer cancel()

		cloudflareResponse, err := sendRequestToCloudflare(ctx, cloudflarePayload, payload.IP)
		if err != nil {
			return nil, err
		}

		// Cache successful responses with cf_clearance based on IP + proxy combination only
		// headless is only for solving behavior, not for cache differentiation
		// Only cache if: success=true AND cf_clearance cookie/header is present
		// Once cached, serve until manually deleted via /delete endpoint
		if success, ok := cloudflareResponse["success"].(bool); ok && success && hasCfClearance(cloudflareResponse) {
			setCachedResponse(cacheKey, cloudflareResponse)
		}

		return cloudflareResponse, nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Request failed",
			"success": false,
		})
		return
	}

	responseMap, ok := response.(EnhancedResponse)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Invalid response format",
			"success": false,
		})
		return
	}

	// Create response copy to avoid mutations
	finalResponse := make(EnhancedResponse)
	for k, v := range responseMap {
		finalResponse[k] = v
	}
	finalResponse["state"] = "new"

	// Return response to pool
	responsePool.Put(responseMap)

	c.JSON(http.StatusOK, finalResponse)
}

func deleteHandler(c *gin.Context) {
	var payload RequestPayload

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Validate IP or hostname
	if !isValidIPOrHostname(payload.IP) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address or hostname"})
		return
	}

	cacheKey := generateCacheKey(payload, appConfig)

	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if _, ok := cache.Get(cacheKey); ok {
		cache.Remove(cacheKey)
		c.JSON(http.StatusOK, gin.H{"message": "Deleted successfully"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cache entry not found"})
	}
}

func cookiesHandler(c *gin.Context) {
	var input interface{}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	cookies := normalizeCookies(input)
	c.JSON(http.StatusOK, gin.H{"cookies": cookies})
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Set request size limit
	router.Use(func(c *gin.Context) {
		if c.Request.ContentLength > 10*1024*1024 { // 10MB limit
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request too large"})
			c.Abort()
			return
		}
		c.Next()
	})

	router.POST("/response", responseHandler)
	router.POST("/delete", deleteHandler)
	router.POST("/cookies", cookiesHandler)

	srv := &http.Server{
		Addr:           ":3080",
		Handler:        router,
		ReadTimeout:    95 * time.Second,
		WriteTimeout:   95 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Println("Server started on port 3080")
		serverErrors <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}

	case <-quit:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Shutdown error: %v", err)
		}
	}
}
