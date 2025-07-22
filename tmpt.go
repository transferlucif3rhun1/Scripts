package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

type Config struct {
	APIKeyFile        string
	RemoteAPIKey      string
	RemoteServer      string
	ListenPort        string
	NextCaptchaURL    string
	SessionThreshold  int
	SessionTTL        time.Duration
	RequestTimeout    time.Duration
	EnableVerboseLog  bool
	NextCaptchaAPIKey string
}

type Logger struct {
	Verbose bool
}

func (l *Logger) Print(format string, v ...interface{}) {
	fmt.Printf(format+"\n", v...)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	if l.Verbose {
		log.Printf("DEBUG: "+format, v...)
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	if l.Verbose {
		log.Printf("INFO: "+format, v...)
	}
}

func (l *Logger) Warning(format string, v ...interface{}) {
	if l.Verbose {
		log.Printf("WARNING: "+format, v...)
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	if l.Verbose {
		log.Printf("ERROR: "+format, v...)
	}
}

var (
	ErrProxyMissing        = errors.New("proxy parameter is missing")
	ErrInvalidProxy        = errors.New("invalid proxy configuration")
	ErrRemoteServerFailure = errors.New("remote server returned non-OK status")
	ErrTokenNotFound       = errors.New("token not found in response")
	ErrCaptchaFailed       = errors.New("captcha solving failed")
	ErrRequestTimeout      = errors.New("request timed out")
	ErrEmptyResponse       = errors.New("empty response from server")
	ErrCookiesNotFound     = errors.New("required cookies not found in response")
)

type SessionRecord struct {
	CompositeKey string
	Count        int
	LastAccess   time.Time
}

type SessionManager struct {
	cache       map[string]*SessionRecord
	mutex       *sync.RWMutex
	threshold   int
	ttl         time.Duration
	stopCleanup context.CancelFunc
}

func NewSessionManager(threshold int, ttl time.Duration) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		cache:       make(map[string]*SessionRecord),
		mutex:       &sync.RWMutex{},
		threshold:   threshold,
		ttl:         ttl,
		stopCleanup: cancel,
	}

	go sm.periodicCleanup(ctx)
	return sm
}

func (sm *SessionManager) UpdateRecord(sessionID, compositeKey string) string {
	now := time.Now()
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	record, exists := sm.cache[sessionID]
	if exists {
		if now.Sub(record.LastAccess) > sm.ttl {
			record.CompositeKey = compositeKey
			record.Count = 1
			record.LastAccess = now
			return "retry"
		}
		if record.CompositeKey != compositeKey {
			record.CompositeKey = compositeKey
			record.Count = 1
			record.LastAccess = now
			return "retry"
		}
		record.Count++
		record.LastAccess = now
		if record.Count >= sm.threshold {
			return "ban"
		}
		return "retry"
	}

	sm.cache[sessionID] = &SessionRecord{
		CompositeKey: compositeKey,
		Count:        1,
		LastAccess:   now,
	}
	return "retry"
}

func (sm *SessionManager) GetCacheSize() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return len(sm.cache)
}

func (sm *SessionManager) periodicCleanup(ctx context.Context) {
	ticker := time.NewTicker(sm.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanupExpiredSessions()
		case <-ctx.Done():
			return
		}
	}
}

func (sm *SessionManager) cleanupExpiredSessions() {
	now := time.Now()
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for id, record := range sm.cache {
		if now.Sub(record.LastAccess) > sm.ttl {
			delete(sm.cache, id)
		}
	}
}

func (sm *SessionManager) Stop() {
	sm.stopCleanup()
}

type HTTPClient struct {
	client *resty.Client
	logger *Logger
	config *Config
}

func NewHTTPClient(config *Config, logger *Logger) *HTTPClient {
	client := resty.New().
		SetTimeout(config.RequestTimeout)

	return &HTTPClient{
		client: client,
		logger: logger,
		config: config,
	}
}

func (c *HTTPClient) SendRemoteRequest(ctx context.Context, sessionID string, proxy *url.URL, requestURL string, method string, requestBody interface{}) (string, string, error) {
	c.logger.Debug("Sending remote request to %s with session %s", requestURL, sessionID)

	var requestBodyJSON interface{} = nil
	if requestBody != nil {
		// Marshal the request body to ensure proper JSON escaping
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			c.logger.Error("Failed to marshal request body: %v", err)
			return "", "", fmt.Errorf("failed to marshal request body: %w", err)
		}

		// Set the marshaled JSON string directly
		// This ensures proper escaping when the entire request is marshaled
		requestBodyJSON = string(bodyBytes)
	}

	requestData := map[string]interface{}{
		"tlsClientIdentifier":         "chrome_117",
		"followRedirects":             false,
		"insecureSkipVerify":          false,
		"withoutCookieJar":            false,
		"withDefaultCookieJar":        true,
		"isByteRequest":               false,
		"forceHttp1":                  false,
		"withRandomTLSExtensionOrder": true,
		"timeoutSeconds":              30,
		"timeoutMilliseconds":         0,
		"sessionId":                   sessionID,
		"proxyUrl":                    proxy.String(),
		"certificatePinningHosts":     map[string]interface{}{},
		"headers":                     remoteHeaders,
		"requestUrl":                  requestURL,
		"requestMethod":               method,
		"requestBody":                 requestBodyJSON,
		"requestCookies":              []string{},
	}

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("x-api-key", c.config.RemoteAPIKey).
		SetBody(requestData).
		Post(fmt.Sprintf("http://%s/api/forward", c.config.RemoteServer))

	if err != nil {
		c.logger.Error("Remote request failed: %v", err)
		return "", "", fmt.Errorf("remote request failed: %w", err)
	}

	if !isSuccessfulStatus(resp.StatusCode()) {
		c.logger.Error("Remote server returned status %d", resp.StatusCode())
		return "", "", fmt.Errorf("%w: %d", ErrRemoteServerFailure, resp.StatusCode())
	}

	respBody := string(resp.Body())
	c.logger.Debug("Remote server response: %s", truncateString(respBody, 200))

	if strings.Contains(requestURL, "/eps-mgr") {
		// For the eps-mgr endpoint, parse the JSON and extract the body field
		var jsonResp map[string]interface{}
		if err := json.Unmarshal([]byte(respBody), &jsonResp); err != nil {
			c.logger.Error("Failed to parse eps-mgr response JSON: %v", err)
			return "", "", fmt.Errorf("failed to parse eps-mgr response: %w", err)
		}

		bodyField, ok := jsonResp["body"].(string)
		if !ok {
			c.logger.Error("Body field not found in eps-mgr response")
			return "", "", errors.New("body field not found in eps-mgr response")
		}

		c.logger.Info("Successfully extracted body field as epsfToken: %s", bodyField)
		return bodyField, "", nil
	}

	var finalRespData FinalResponse
	if err := json.Unmarshal(resp.Body(), &finalRespData); err != nil {
		c.logger.Error("Failed to parse final response: %v", err)
		return "", "", fmt.Errorf("failed to parse final response: %w", err)
	}

	c.logger.Info("Successfully extracted cookies")
	c.logger.Debug("TMPT: %s, EPS_SID: %s", finalRespData.Cookies.Tmpt, finalRespData.Cookies.EpsSID)
	return finalRespData.Cookies.Tmpt, finalRespData.Cookies.EpsSID, nil
}

func (c *HTTPClient) SolveCaptcha(ctx context.Context, sessionID, apiKey string) (string, error) {
	c.logger.Info("Attempting to solve CAPTCHA for session %s", sessionID)

	captchaRequestBody := NextCaptchaRequestBody{
		ClientKey: apiKey,
		Task: NextCaptchaTaskRequestBody{
			Type:       "RecaptchaV3TaskProxyless",
			WebsiteURL: "https://auth.ticketmaster.com",
			WebsiteKey: "6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1",
			PageAction: "LoginPage",
		},
	}

	c.logger.Debug("Sending CAPTCHA request to %s", c.config.NextCaptchaURL)
	captchaResp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(captchaRequestBody).
		Post(c.config.NextCaptchaURL)

	if err != nil {
		c.logger.Error("CAPTCHA request failed: %v", err)
		return "", fmt.Errorf("%w: %v", ErrCaptchaFailed, err)
	}

	captchaRespBody := strings.TrimSpace(string(captchaResp.Body()))
	c.logger.Debug("CAPTCHA response: %s", captchaRespBody)

	if !strings.HasPrefix(captchaRespBody, "0|") {
		c.logger.Error("Unexpected CAPTCHA response format: %s", captchaRespBody)
		return "", fmt.Errorf("%w: unexpected format", ErrCaptchaFailed)
	}

	captchaToken := strings.TrimPrefix(captchaRespBody, "0|")
	if captchaToken == "" {
		c.logger.Error("Empty CAPTCHA token received")
		return "", fmt.Errorf("%w: empty token", ErrCaptchaFailed)
	}

	c.logger.Info("Successfully obtained CAPTCHA token")
	return captchaToken, nil
}

type AppServer struct {
	router         *gin.Engine
	httpClient     *HTTPClient
	sessionManager *SessionManager
	logger         *Logger
	config         *Config
}

func NewAppServer(config *Config, logger *Logger) *AppServer {
	gin.SetMode(gin.ReleaseMode)

	httpClient := NewHTTPClient(config, logger)
	sessionManager := NewSessionManager(config.SessionThreshold, config.SessionTTL)

	router := gin.New()
	router.Use(gin.Recovery())

	server := &AppServer{
		router:         router,
		httpClient:     httpClient,
		sessionManager: sessionManager,
		logger:         logger,
		config:         config,
	}

	server.registerRoutes()
	return server
}

func (s *AppServer) registerRoutes() {
	s.router.GET("/cookies", s.HandleCookiesRequest)
	s.router.POST("/check", s.CheckHandler)
}

func (s *AppServer) Start() error {
	s.logger.Print("Starting server on port %s...", s.config.ListenPort)
	return s.router.Run(":" + s.config.ListenPort)
}

func (s *AppServer) GracefulShutdown(ctx context.Context) {
	if s.logger.Verbose {
		s.logger.Info("Shutting down server...")
	}
	s.sessionManager.Stop()
	if s.logger.Verbose {
		s.logger.Info("Server shutdown complete")
	}
}

func (s *AppServer) HandleCookiesRequest(c *gin.Context) {
	proxy := c.Query("proxy")
	if proxy == "" {
		s.logger.Warning("Proxy parameter missing in request")
		c.JSON(http.StatusOK, ResponseToUser{Status: "retry", Error: ErrProxyMissing.Error()})
		return
	}

	parsedProxy, err := parseProxy(proxy)
	if err != nil {
		s.logger.Warning("Invalid proxy format: %v", err)
		c.JSON(http.StatusOK, ResponseToUser{Status: "retry", Error: err.Error()})
		return
	}

	sessionID := generateRandomHex(10)
	s.logger.Info("Processing new cookies request with session %s and proxy %s", sessionID, proxy)

	ctx, cancel := context.WithTimeout(c.Request.Context(), s.config.RequestTimeout)
	defer cancel()

	resultChan := make(chan ResponseToUser)
	go func() {
		tmpt, epsSID, err := s.processSession(ctx, sessionID, parsedProxy)
		if err != nil {
			s.logger.Error("Session processing failed: %v", err)
			resultChan <- ResponseToUser{Status: "retry", Error: err.Error()}
			return
		}
		s.logger.Info("Session processing succeeded for %s", sessionID)
		resultChan <- ResponseToUser{Status: "success", Tmpt: tmpt, EpsSID: epsSID}
	}()

	select {
	case res := <-resultChan:
		c.JSON(http.StatusOK, res)
	case <-ctx.Done():
		s.logger.Error("Request timed out for session %s", sessionID)
		c.JSON(http.StatusOK, ResponseToUser{Status: "retry", Error: ErrRequestTimeout.Error()})
	}
}

func (s *AppServer) processSession(ctx context.Context, sessionID string, proxy *url.URL) (string, string, error) {
	s.logger.Debug("Starting session processing for %s", sessionID)

	// Step 1: First request to get EPS token
	firstRequestURL := "https://auth.ticketmaster.com/eps-mgr?epsf-token=renew"
	epsfToken, _, err := s.httpClient.SendRemoteRequest(ctx, sessionID, proxy, firstRequestURL, "GET", nil)
	if err != nil {
		return "", "", fmt.Errorf("first request failed: %w", err)
	}

	// Step 2: Solve CAPTCHA
	captchaToken, err := s.httpClient.SolveCaptcha(ctx, sessionID, s.config.NextCaptchaAPIKey)
	if err != nil {
		return "", "", fmt.Errorf("captcha solving failed: %w", err)
	}

	// Step 3: Final request with token and payload
	finalRequestURL := fmt.Sprintf("https://auth.ticketmaster.com/epsf/gec/v3/%s/LoginPage", epsfToken)

	payload := map[string]string{
		"hostname": "auth.ticketmaster.com",
		"key":      "6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1",
		"token":    captchaToken,
	}

	tmpt, epsSID, err := s.httpClient.SendRemoteRequest(ctx, sessionID, proxy, finalRequestURL, "POST", payload)
	if err != nil {
		return "", "", fmt.Errorf("final request failed: %w", err)
	}

	if tmpt == "" || epsSID == "" {
		return "", "", ErrCookiesNotFound
	}

	return tmpt, epsSID, nil
}

func (s *AppServer) CheckHandler(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		s.logger.Warning("Invalid request method: %s", c.Request.Method)
		c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "invalid request method"})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		s.logger.Error("Failed to read request body: %v", err)
		c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "failed to read request body"})
		return
	}

	if len(bodyBytes) == 0 {
		s.logger.Warning("Empty request body")
		c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "empty request body"})
		return
	}

	trimmedBody := strings.TrimSpace(string(bodyBytes))
	if trimmedBody == "" {
		s.logger.Warning("Empty request body after trimming")
		c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "empty request body after trimming"})
		return
	}

	s.logger.Debug("Processing check request: %s", truncateString(trimmedBody, 200))

	parsedPayload, parseErr := parseJSONPayload([]byte(trimmedBody))

	var textToCheck string
	if parseErr == nil {
		if b, exists := parsedPayload["body"]; exists {
			textToCheck = strings.TrimSpace(b)
			s.logger.Debug("Found 'body' field for keyword matching: %s", truncateString(textToCheck, 100))
		}
	}

	if textToCheck == "" {
		textToCheck = trimmedBody
		s.logger.Debug("Using entire payload for keyword matching")
	}

	lowerText := strings.ToLower(textToCheck)

	track := false
	var customAction string

	for kw, action := range keywordActions {
		if strings.Contains(lowerText, strings.ToLower(kw)) {
			s.logger.Info("Keyword match found: %s -> %s", kw, action)
			if action != "retry" {
				customAction = action
				break
			} else {
				track = true
			}
		}
	}

	if customAction != "" {
		s.logger.Info("Sending custom action: %s", customAction)
		c.JSON(http.StatusOK, CheckResponse{Status: customAction, Error: fmt.Sprintf("custom status '%s' triggered", customAction)})
		return
	}

	if track {
		if parseErr == nil {
			sessionID := strings.TrimSpace(parsedPayload["sessionId"])
			if sessionID != "" {
				statusVal := strings.TrimSpace(parsedPayload["status"])
				target := strings.TrimSpace(parsedPayload["target"])
				compositeKey := sessionID + "_" + statusVal + "_" + target

				s.logger.Debug("Tracking session with composite key: %s", compositeKey)
				result := s.sessionManager.UpdateRecord(sessionID, compositeKey)

				if result == "ban" {
					s.logger.Warning("Session %s banned due to repeated retries", sessionID)
					c.JSON(http.StatusOK, CheckResponse{Status: "ban", Error: "repeated retry request detected"})
					return
				}

				s.logger.Info("Retry recorded for session %s", sessionID)
				c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "retry request recorded"})
				return
			}
		}

		s.logger.Info("Retry keyword detected without session info")
		c.JSON(http.StatusOK, CheckResponse{Status: "retry", Error: "retry keyword detected without session info"})
		return
	}

	s.logger.Debug("No special handling needed, passing through response")
	c.Data(http.StatusOK, "application/json", []byte(trimmedBody))
}

type FinalResponse struct {
	Cookies struct {
		Tmpt   string `json:"tmpt"`
		EpsSID string `json:"eps_sid"`
	} `json:"cookies"`
}

type NextCaptchaRequestBody struct {
	ClientKey string                     `json:"clientKey"`
	Task      NextCaptchaTaskRequestBody `json:"task"`
}

type NextCaptchaTaskRequestBody struct {
	Type       string `json:"type"`
	WebsiteURL string `json:"websiteURL"`
	WebsiteKey string `json:"websiteKey"`
	PageAction string `json:"pageAction"`
}

type ResponseToUser struct {
	Status string `json:"status"`
	Tmpt   string `json:"tmpt,omitempty"`
	EpsSID string `json:"eps_sid,omitempty"`
	Error  string `json:"error,omitempty"`
}

type CheckResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

var keywordActions = map[string]string{
	"\"block":                           "delete",
	"\"captcha":                         "ban",
	"failed to do request":              "ban",
	"failed to parse the response":      "ban",
	"get your identity verified":        "delete",
	"response pending":                  "ban",
	"correlationid":                     "ban",
	"proxy responded with non 200 code": "ban",
}

var remoteHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36",
	"Pragma":          "no-cache",
	"Accept":          "*/*",
	"Accept-Language": "en-US,en;q=0.8",
	"Content-Type":    "application/json",
}

func ensureAPIKey(filepath string) (string, error) {
	fileInfo, err := os.Stat(filepath)

	if os.IsNotExist(err) || (err == nil && fileInfo.Size() == 0) {
		fmt.Print("Enter your NextCaptcha API key: ")
		reader := bufio.NewReader(os.Stdin)
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read API key: %w", err)
		}

		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return "", errors.New("API key cannot be empty")
		}

		if err := os.WriteFile(filepath, []byte(apiKey), 0600); err != nil {
			return "", fmt.Errorf("failed to save API key to %s: %w", filepath, err)
		}

		fmt.Printf("NextCaptcha API key saved to %s\n", filepath)
		return apiKey, nil
	}

	if err != nil {
		return "", fmt.Errorf("failed to access API key file: %w", err)
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to read API key file: %w", err)
	}

	apiKey := strings.TrimSpace(string(data))
	if apiKey == "" {
		fmt.Print("API key file is empty. Enter your NextCaptcha API key: ")
		reader := bufio.NewReader(os.Stdin)
		apiKey, err = reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read API key: %w", err)
		}

		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			return "", errors.New("API key cannot be empty")
		}

		if err := os.WriteFile(filepath, []byte(apiKey), 0600); err != nil {
			return "", fmt.Errorf("failed to save API key to %s: %w", filepath, err)
		}

		fmt.Printf("NextCaptcha API key saved to %s\n", filepath)
	}

	return apiKey, nil
}

func generateRandomHex(length int) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, length)
	for i := range result {
		result[i] = hexChars[rand.Intn(len(hexChars))]
	}
	return string(result)
}

func isSuccessfulStatus(statusCode int) bool {
	return statusCode == 200 || statusCode == 201 || statusCode == 202
}

func parseProxy(proxy string) (*url.URL, error) {
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}

	parsed, err := url.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidProxy, err)
	}

	host, port := parsed.Hostname(), parsed.Port()
	if host == "" || port == "" {
		return nil, fmt.Errorf("%w: proxy must include host and port", ErrInvalidProxy)
	}

	if net.ParseIP(host) == nil && !isValidHostname(host) {
		return nil, fmt.Errorf("%w: invalid host", ErrInvalidProxy)
	}

	if _, err := net.LookupPort("tcp", port); err != nil {
		return nil, fmt.Errorf("%w: invalid port: %v", ErrInvalidProxy, err)
	}

	if parsed.User != nil {
		username := parsed.User.Username()
		password, hasPassword := parsed.User.Password()
		if username == "" || !hasPassword || password == "" {
			return nil, fmt.Errorf("%w: proxy requires both username and password", ErrInvalidProxy)
		}
	}

	return parsed, nil
}

func isValidHostname(hostname string) bool {
	const hostnameRegex = `^([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*\.)+[a-zA-Z]{2,}$`
	re := regexp.MustCompile(hostnameRegex)
	return re.MatchString(hostname)
}

func parseJSONPayload(data []byte) (map[string]string, error) {
	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for k, v := range generic {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}

	return result, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	verbose := true
	if os.Getenv("VERBOSE_LOGGING") == "true" {
		verbose = true
	}

	logger := &Logger{Verbose: verbose}

	apiKeyFile := "apikey.txt"
	apiKey, err := ensureAPIKey(apiKeyFile)
	if err != nil {
		logger.Print("Failed to get API key: %v", err)
		os.Exit(1)
	}

	config := &Config{
		APIKeyFile:        apiKeyFile,
		RemoteAPIKey:      "my-auth-key-1",
		RemoteServer:      "45.92.1.127:8080",
		ListenPort:        "3081",
		NextCaptchaURL:    "https://api-v2.nextcaptcha.com/getToken",
		SessionThreshold:  2,
		SessionTTL:        1 * time.Minute,
		RequestTimeout:    60 * time.Second,
		EnableVerboseLog:  verbose,
		NextCaptchaAPIKey: apiKey,
	}

	if verbose {
		logger.Info("Starting application with verbose logging enabled")
	}

	server := NewAppServer(config, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		if verbose {
			logger.Info("Received shutdown signal")
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.GracefulShutdown(shutdownCtx)
		os.Exit(0)
	}()

	if err := server.Start(); err != nil {
		logger.Print("Failed to start server: %v", err)
		os.Exit(1)
	}
}
