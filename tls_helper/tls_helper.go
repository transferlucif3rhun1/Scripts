package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/gin-gonic/gin"
)

type TLSRequest struct {
	RequestURL            string            `json:"requestUrl"`
	RequestMethod         string            `json:"requestMethod"`
	RequestBody           string            `json:"requestBody,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	HeaderOrder           []string          `json:"headerOrder,omitempty"`
	RequestCookies        []Cookie          `json:"requestCookies,omitempty"`
	TlsClientIdentifier   string            `json:"tlsClientIdentifier"`
	SessionID             string            `json:"sessionId,omitempty"`
	ProxyURL              string            `json:"proxyUrl,omitempty"`
	FollowRedirects       bool              `json:"followRedirects"`
	ForceHttp1            bool              `json:"forceHttp1"`
	WithDefaultCookieJar  bool              `json:"withDefaultCookieJar"`
	WithRandomTLSExtOrder bool              `json:"withRandomTLSExtensionOrder"`
	TimeoutSeconds        int               `json:"timeoutSeconds"`
	InsecureSkipVerify    bool              `json:"insecureSkipVerify"`
	IsRotatingProxy       bool              `json:"isRotatingProxy"`
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TLSResponse struct {
	Status    int                 `json:"status"`
	Headers   map[string][]string `json:"headers"`
	Cookies   map[string]string   `json:"cookies"`
	Body      string              `json:"body"`
	SessionID string              `json:"sessionId,omitempty"`
	Success   bool                `json:"success"`
	Error     string              `json:"error,omitempty"`
}

type SessionManager struct {
	sessions sync.Map
	cleanup  *time.Ticker
	stopChan chan struct{}
	logger   *log.Logger
}

type TLSService struct {
	sessionManager *SessionManager
	stats          *ServiceStats
	logger         *log.Logger
	instanceID     int
}

type ServiceStats struct {
	RequestCount     int64     `json:"requestCount"`
	ActiveSessions   int64     `json:"activeSessions"`
	ActiveGoroutines int       `json:"activeGoroutines"`
	StartTime        time.Time `json:"startTime"`
	InstanceID       int       `json:"instanceId"`
	Port             int       `json:"port"`
	mutex            sync.RWMutex
}

type InstanceManager struct {
	instances []*ServiceInstance
	logger    *log.Logger
	wg        sync.WaitGroup
	stopChan  chan struct{}
}

type ServiceInstance struct {
	ID       int
	Port     int
	Service  *TLSService
	Server   *http.Server
	Logger   *log.Logger
	StopChan chan struct{}
}

func NewLogger(prefix string) *log.Logger {
	return log.New(os.Stdout, fmt.Sprintf("[%s] ", prefix), log.LstdFlags|log.Lmicroseconds)
}

func NewSessionManager(instanceID int) *SessionManager {
	logger := NewLogger(fmt.Sprintf("SESSION-MANAGER-%d", instanceID))
	sm := &SessionManager{
		cleanup:  time.NewTicker(2 * time.Minute),
		stopChan: make(chan struct{}),
		logger:   logger,
	}
	go sm.startCleanup()
	return sm
}

func (sm *SessionManager) startCleanup() {
	defer sm.cleanup.Stop()
	for {
		select {
		case <-sm.cleanup.C:
			sm.performCleanup()
		case <-sm.stopChan:
			return
		}
	}
}

func (sm *SessionManager) performCleanup() {
	cleanedCount := 0
	sm.sessions.Range(func(key, value interface{}) bool {
		if sessionData, ok := value.(map[string]interface{}); ok {
			if lastUsed, ok := sessionData["lastUsed"].(time.Time); ok {
				if time.Since(lastUsed) > 10*time.Minute {
					sm.sessions.Delete(key)
					cleanedCount++
				}
			}
		}
		return true
	})
	if cleanedCount > 0 {
		sm.logger.Printf("Cleaned up %d expired sessions", cleanedCount)
	}
}

func (sm *SessionManager) Stop() {
	close(sm.stopChan)
}

func (sm *SessionManager) GetOrCreateClient(sessionID string, profileStr string, options *TLSRequest) (tls_client.HttpClient, error) {
	if sessionID == "" {
		return sm.createNewClient(profileStr, options)
	}

	if sessionInterface, exists := sm.sessions.Load(sessionID); exists {
		if sessionData, ok := sessionInterface.(map[string]interface{}); ok {
			if client, ok := sessionData["client"].(tls_client.HttpClient); ok {
				sessionData["lastUsed"] = time.Now()
				sm.sessions.Store(sessionID, sessionData)
				return client, nil
			}
		}
	}

	client, err := sm.createNewClient(profileStr, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create new client: %w", err)
	}

	sessionData := map[string]interface{}{
		"client":   client,
		"lastUsed": time.Now(),
		"created":  time.Now(),
	}

	sm.sessions.Store(sessionID, sessionData)
	sm.logger.Printf("Created new session: %s", sessionID)
	return client, nil
}

func (sm *SessionManager) createNewClient(profileStr string, options *TLSRequest) (tls_client.HttpClient, error) {
	profile, exists := profiles.MappedTLSClients[profileStr]
	if !exists {
		sm.logger.Printf("Unknown profile '%s', using default", profileStr)
		profile = profiles.DefaultClientProfile
	}

	clientOptions := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(options.TimeoutSeconds),
		tls_client.WithClientProfile(profile),
		tls_client.WithCatchPanics(),
	}

	if options.WithDefaultCookieJar {
		jar := tls_client.NewCookieJar()
		clientOptions = append(clientOptions, tls_client.WithCookieJar(jar))
	}

	if options.WithRandomTLSExtOrder {
		clientOptions = append(clientOptions, tls_client.WithRandomTLSExtensionOrder())
	}

	if options.InsecureSkipVerify {
		clientOptions = append(clientOptions, tls_client.WithInsecureSkipVerify())
	}

	if options.ForceHttp1 {
		clientOptions = append(clientOptions, tls_client.WithForceHttp1())
	}

	if options.ProxyURL != "" {
		clientOptions = append(clientOptions, tls_client.WithProxyUrl(options.ProxyURL))
	}

	if !options.FollowRedirects {
		clientOptions = append(clientOptions, tls_client.WithNotFollowRedirects())
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	return client, nil
}

func (sm *SessionManager) FreeSession(sessionID string) bool {
	if sessionID != "" {
		if _, exists := sm.sessions.LoadAndDelete(sessionID); exists {
			sm.logger.Printf("Freed session: %s", sessionID)
			return true
		}
	}
	return false
}

func (sm *SessionManager) GetSessionCount() int64 {
	count := int64(0)
	sm.sessions.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func NewTLSService(instanceID int, port int) *TLSService {
	logger := NewLogger(fmt.Sprintf("TLS-SERVICE-%d", instanceID))
	return &TLSService{
		sessionManager: NewSessionManager(instanceID),
		logger:         logger,
		instanceID:     instanceID,
		stats: &ServiceStats{
			StartTime:  time.Now(),
			InstanceID: instanceID,
			Port:       port,
		},
	}
}

func (s *TLSService) ProcessRequest(ctx context.Context, req *TLSRequest) *TLSResponse {
	s.updateStats()

	if req.RequestURL == "" {
		return &TLSResponse{
			Success: false,
			Error:   "requestUrl is required",
			Status:  400,
		}
	}

	client, err := s.sessionManager.GetOrCreateClient(req.SessionID, req.TlsClientIdentifier, req)
	if err != nil {
		s.logger.Printf("Failed to create TLS client: %v", err)
		return &TLSResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create TLS client: %v", err),
			Status:  500,
		}
	}

	httpReq, err := s.buildHTTPRequest(req)
	if err != nil {
		s.logger.Printf("Failed to build HTTP request: %v", err)
		return &TLSResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to build request: %v", err),
			Status:  400,
		}
	}

	requestStart := time.Now()
	resp, err := client.Do(httpReq)
	requestDuration := time.Since(requestStart)

	if err != nil {
		s.logger.Printf("Request failed after %v: %v", requestDuration, err)
		return &TLSResponse{
			Success: false,
			Error:   fmt.Sprintf("Request failed: %v", err),
			Status:  500,
		}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.Printf("Failed to read response body: %v", err)
		return &TLSResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to read response body: %v", err),
			Status:  500,
		}
	}

	cookies := make(map[string]string)
	for _, cookie := range resp.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	s.logger.Printf("Request completed in %v: %s %s -> %d",
		requestDuration, req.RequestMethod, req.RequestURL, resp.StatusCode)

	return &TLSResponse{
		Status:    resp.StatusCode,
		Headers:   resp.Header,
		Cookies:   cookies,
		Body:      string(bodyBytes),
		SessionID: req.SessionID,
		Success:   true,
	}
}

func (s *TLSService) updateStats() {
	s.stats.mutex.Lock()
	s.stats.RequestCount++
	s.stats.ActiveGoroutines = runtime.NumGoroutine()
	s.stats.ActiveSessions = s.sessionManager.GetSessionCount()
	s.stats.mutex.Unlock()
}

func (s *TLSService) buildHTTPRequest(req *TLSRequest) (*fhttp.Request, error) {
	var body io.Reader
	if req.RequestBody != "" {
		body = strings.NewReader(req.RequestBody)
	}

	httpReq, err := fhttp.NewRequest(req.RequestMethod, req.RequestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if len(req.HeaderOrder) > 0 {
		for _, headerName := range req.HeaderOrder {
			if value, exists := req.Headers[headerName]; exists {
				httpReq.Header.Set(headerName, value)
			}
		}
		httpReq.Header[fhttp.HeaderOrderKey] = req.HeaderOrder
	} else {
		for name, value := range req.Headers {
			httpReq.Header.Set(name, value)
		}
	}

	for _, cookie := range req.RequestCookies {
		httpReq.AddCookie(&fhttp.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		})
	}

	return httpReq, nil
}

func (s *TLSService) handleForward(c *gin.Context) {
	var req TLSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Printf("Invalid request format: %v", err)
		c.JSON(400, TLSResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request format: %v", err),
			Status:  400,
		})
		return
	}

	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 30
	}
	if req.TlsClientIdentifier == "" {
		req.TlsClientIdentifier = "chrome_133"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	response := s.ProcessRequest(ctx, &req)
	c.JSON(200, response)
}

func (s *TLSService) handleFreeSession(c *gin.Context) {
	var freeReq struct {
		SessionID string `json:"sessionId"`
	}

	if err := c.ShouldBindJSON(&freeReq); err != nil {
		s.logger.Printf("Invalid free session request: %v", err)
		c.JSON(400, gin.H{"success": false, "error": "Invalid request format"})
		return
	}

	success := s.sessionManager.FreeSession(freeReq.SessionID)
	c.JSON(200, gin.H{"success": success})
}

func (s *TLSService) handleHealth(c *gin.Context) {
	s.stats.mutex.RLock()
	stats := *s.stats
	stats.ActiveGoroutines = runtime.NumGoroutine()
	stats.ActiveSessions = s.sessionManager.GetSessionCount()
	s.stats.mutex.RUnlock()

	profileCount := len(profiles.MappedTLSClients)

	c.JSON(200, gin.H{
		"status":    "healthy",
		"stats":     stats,
		"uptime":    time.Since(stats.StartTime).String(),
		"profiles":  profileCount,
		"timestamp": time.Now().Unix(),
	})
}

func (s *TLSService) handleStats(c *gin.Context) {
	s.stats.mutex.RLock()
	stats := *s.stats
	stats.ActiveGoroutines = runtime.NumGoroutine()
	stats.ActiveSessions = s.sessionManager.GetSessionCount()
	s.stats.mutex.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	availableProfiles := make([]string, 0, len(profiles.MappedTLSClients))
	for profileName := range profiles.MappedTLSClients {
		availableProfiles = append(availableProfiles, profileName)
	}

	c.JSON(200, gin.H{
		"service": stats,
		"memory": gin.H{
			"alloc":      memStats.Alloc,
			"totalAlloc": memStats.TotalAlloc,
			"sys":        memStats.Sys,
			"gcRuns":     memStats.NumGC,
		},
		"runtime": gin.H{
			"goroutines": runtime.NumGoroutine(),
			"cpus":       runtime.NumCPU(),
			"goVersion":  runtime.Version(),
		},
		"profiles": gin.H{
			"available": availableProfiles,
			"count":     len(availableProfiles),
		},
	})
}

func (s *TLSService) handleProfiles(c *gin.Context) {
	availableProfiles := make([]string, 0, len(profiles.MappedTLSClients))
	for profileName := range profiles.MappedTLSClients {
		availableProfiles = append(availableProfiles, profileName)
	}

	c.JSON(200, gin.H{
		"profiles": availableProfiles,
		"count":    len(availableProfiles),
		"default":  "chrome_133",
	})
}

func (s *TLSService) Stop() {
	s.logger.Println("Stopping TLS service...")
	s.sessionManager.Stop()
}

func setupRouter(service *TLSService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Output: os.Stdout,
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[GIN-%d] %v | %3d | %13v | %15s | %-7s %#v\n",
				service.instanceID,
				param.TimeStamp.Format("2006/01/02 15:04:05"),
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
			)
		},
	}))

	router.Use(gin.Recovery())

	api := router.Group("/api")
	{
		api.POST("/forward", service.handleForward)
		api.POST("/free-session", service.handleFreeSession)
		api.GET("/health", service.handleHealth)
		api.GET("/stats", service.handleStats)
		api.GET("/profiles", service.handleProfiles)
	}

	return router
}

func NewServiceInstance(id, port int) *ServiceInstance {
	logger := NewLogger(fmt.Sprintf("INSTANCE-%d", id))
	service := NewTLSService(id, port)
	router := setupRouter(service)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &ServiceInstance{
		ID:       id,
		Port:     port,
		Service:  service,
		Server:   server,
		Logger:   logger,
		StopChan: make(chan struct{}),
	}
}

func (si *ServiceInstance) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	si.Logger.Printf("Starting on port %d", si.Port)

	go func() {
		if err := si.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			si.Logger.Printf("Server failed: %v", err)
		}
	}()

	<-si.StopChan
	si.Logger.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := si.Server.Shutdown(ctx); err != nil {
		si.Logger.Printf("Shutdown error: %v", err)
	}

	si.Service.Stop()
	si.Logger.Println("Stopped")
}

func (si *ServiceInstance) Stop() {
	close(si.StopChan)
}

func NewInstanceManager() *InstanceManager {
	return &InstanceManager{
		logger:   NewLogger("INSTANCE-MANAGER"),
		stopChan: make(chan struct{}),
	}
}

func (im *InstanceManager) StartInstances(instanceCount, basePort int) error {
	im.logger.Printf("Starting %d instances from port %d", instanceCount, basePort)

	im.instances = make([]*ServiceInstance, instanceCount)

	for i := 0; i < instanceCount; i++ {
		port := basePort + i
		instance := NewServiceInstance(i+1, port)
		im.instances[i] = instance

		im.wg.Add(1)
		go instance.Start(&im.wg)

		time.Sleep(100 * time.Millisecond)
	}

	im.logger.Printf("All %d instances started successfully", instanceCount)
	return nil
}

func (im *InstanceManager) Stop() {
	im.logger.Println("Stopping all instances...")

	for _, instance := range im.instances {
		instance.Stop()
	}

	done := make(chan struct{})
	go func() {
		im.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		im.logger.Println("All instances stopped gracefully")
	case <-time.After(45 * time.Second):
		im.logger.Println("Force shutdown after timeout")
	}
}

func (im *InstanceManager) GetInstanceInfo() []map[string]interface{} {
	info := make([]map[string]interface{}, len(im.instances))
	for i, instance := range im.instances {
		info[i] = map[string]interface{}{
			"id":   instance.ID,
			"port": instance.Port,
		}
	}
	return info
}

func main() {
	instances := flag.Int("instances", 1, "Number of instances to run")
	basePort := flag.Int("port", 8001, "Base port number")
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	logger := NewLogger("MAIN")
	logger.Printf("Go TLS Service Manager starting...")
	logger.Printf("Runtime: %s on %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	logger.Printf("CPUs: %d, GOMAXPROCS: %d", runtime.NumCPU(), runtime.GOMAXPROCS(0))

	manager := NewInstanceManager()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Printf("Received signal: %v", sig)
		manager.Stop()
		os.Exit(0)
	}()

	if err := manager.StartInstances(*instances, *basePort); err != nil {
		logger.Printf("Failed to start instances: %v", err)
		os.Exit(1)
	}

	instanceInfo := manager.GetInstanceInfo()
	infoJSON, _ := json.MarshalIndent(instanceInfo, "", "  ")
	logger.Printf("Instance configuration:\n%s", string(infoJSON))

	logger.Printf("All instances running. Available profiles: %d", len(profiles.MappedTLSClients))
	logger.Println("Press Ctrl+C to stop")

	select {}
}
