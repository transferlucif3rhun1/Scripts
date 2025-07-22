// main.go

package main

import (
    "context"
    "crypto/rand"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/gin-gonic/gin"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "go.mongodb.org/mongo-driver/mongo/readpref"
)

// Config holds the server configuration
type Config struct {
    ServerPort        string `json:"serverPort"`
    MongoURI          string `json:"mongoURI"`
    DatabaseName      string `json:"databaseName"`
    ApiKeysCollection string `json:"apiKeysCollection"`
    ReadTimeout       int    `json:"readTimeout"`
    WriteTimeout      int    `json:"writeTimeout"`
    IdleTimeout       int    `json:"idleTimeout"`
}

// APIKey represents an API key document in MongoDB
type APIKey struct {
    ID             string    `bson:"_id" json:"key"`
    Name           string    `bson:"Name,omitempty" json:"name,omitempty"`
    Expiration     time.Time `bson:"Expiration" json:"expiration"`
    RPM            int       `bson:"RPM" json:"rpm"`                       // Requests Per Minute
    ThreadsLimit   int       `bson:"ThreadsLimit,omitempty" json:"tl"`     // Threads Limit
    TotalRequests  int64     `bson:"TotalRequests,omitempty" json:"limit"` // Total Requests Limit
}

// Changes returns the differences between two APIKey instances
func (api *APIKey) Changes(updated *APIKey) []string {
    var changes []string
    if api.Name != updated.Name {
        changes = append(changes, fmt.Sprintf("Name: %s -> %s", api.Name, updated.Name))
    }
    if !api.Expiration.Equal(updated.Expiration) {
        changes = append(changes, fmt.Sprintf("Expiration: %s -> %s", api.Expiration.Format(time.RFC3339), updated.Expiration.Format(time.RFC3339)))
    }
    if api.RPM != updated.RPM {
        changes = append(changes, fmt.Sprintf("RPM: %d -> %d", api.RPM, updated.RPM))
    }
    if api.ThreadsLimit != updated.ThreadsLimit {
        changes = append(changes, fmt.Sprintf("TL: %d -> %d", api.ThreadsLimit, updated.ThreadsLimit))
    }
    if api.TotalRequests != updated.TotalRequests {
        changes = append(changes, fmt.Sprintf("Limit: %d -> %d", api.TotalRequests, updated.TotalRequests))
    }
    return changes
}

// Cache manages the in-memory cache of API keys
type Cache struct {
    keyToAPIKey sync.Map
}

// GetAPIKey retrieves an API key from the cache
func (c *Cache) GetAPIKey(key string) (*APIKey, bool) {
    value, exists := c.keyToAPIKey.Load(key)
    if !exists {
        return nil, false
    }
    apiKey, ok := value.(*APIKey)
    return apiKey, ok
}

// SetAPIKey adds or updates an API key in the cache
func (c *Cache) SetAPIKey(apiKey *APIKey) {
    c.keyToAPIKey.Store(apiKey.ID, apiKey)
}

// DeleteAPIKey removes an API key from the cache
func (c *Cache) DeleteAPIKey(key string) {
    c.keyToAPIKey.Delete(key)
}

// APIKeyManager manages API keys and related operations
type APIKeyManager struct {
    mongoClient       *mongo.Client
    apiKeysCollection *mongo.Collection
    cache             *Cache
    config            *Config
    configMutex       sync.RWMutex
    rateLimiters      sync.Map
    httpClient        *http.Client
    stopChan          chan struct{}
    wg                sync.WaitGroup
}

// NewAPIKeyManager initializes a new APIKeyManager
func NewAPIKeyManager(config *Config) *APIKeyManager {
    return &APIKeyManager{
        cache:      &Cache{},
        config:     config,
        httpClient: &http.Client{Timeout: 30 * time.Second},
        stopChan:   make(chan struct{}),
    }
}

// loadConfig loads the configuration from a JSON file
func loadConfig(filePath string) (*Config, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    decoder := json.NewDecoder(file)
    config := &Config{}
    if err := decoder.Decode(config); err != nil {
        return nil, err
    }
    return config, nil
}

// watchConfigAndReload watches the configuration file for changes and reloads it
func (m *APIKeyManager) watchConfigAndReload(filePath string) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Printf("Failed to create file watcher: %v", err)
        return
    }
    defer watcher.Close()
    if err := watcher.Add(filePath); err != nil {
        log.Printf("Failed to add file to watcher: %v", err)
        return
    }
    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                newConfig, err := loadConfig(filePath)
                if err == nil {
                    m.configMutex.Lock()
                    m.config = newConfig
                    m.configMutex.Unlock()
                    log.Printf("Configuration reloaded successfully")
                    // Update any settings dependent on the config
                } else {
                    log.Printf("Failed to reload configuration: %v", err)
                }
            }
        case err := <-watcher.Errors:
            log.Printf("Watcher error: %v", err)
        case <-m.stopChan:
            return
        }
    }
}

// connectMongo connects to MongoDB
func (m *APIKeyManager) connectMongo() error {
    clientOptions := options.Client().
        ApplyURI(m.config.MongoURI).
        SetMaxPoolSize(100).
        SetMinPoolSize(10).
        SetRetryWrites(true)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    var err error
    m.mongoClient, err = mongo.Connect(ctx, clientOptions)
    if err != nil {
        return fmt.Errorf("failed to connect to MongoDB: %v", err)
    }

    ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancelPing()

    err = m.mongoClient.Ping(ctxPing, readpref.Primary())
    if err != nil {
        return fmt.Errorf("failed to ping MongoDB: %v", err)
    }

    m.apiKeysCollection = m.mongoClient.Database(m.config.DatabaseName).Collection(m.config.ApiKeysCollection)
    log.Println("Connected to MongoDB")
    return nil
}

// loadAPIKeysToCache loads all API keys from MongoDB into the cache
func (m *APIKeyManager) loadAPIKeysToCache() error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    cursor, err := m.apiKeysCollection.Find(ctx, bson.M{})
    if err != nil {
        return err
    }
    defer cursor.Close(ctx)
    for cursor.Next(ctx) {
        var apiKey APIKey
        if err := cursor.Decode(&apiKey); err != nil {
            continue
        }
        m.cache.SetAPIKey(&apiKey)
        if apiKey.RPM > 0 {
            m.rateLimiters.Store(apiKey.ID, NewFixedWindowRateLimiter(apiKey.RPM))
        }
    }
    if err := cursor.Err(); err != nil {
        return err
    }
    return nil
}

// updateExistingKeys sets default values for new fields in existing keys
func (m *APIKeyManager) updateExistingKeys() error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    filter := bson.M{}
    update := bson.M{
        "$set": bson.M{
            "RPM":           0,
            "ThreadsLimit":  0,
            "TotalRequests": 0,
        },
    }
    _, err := m.apiKeysCollection.UpdateMany(ctx, filter, update)
    if err != nil {
        return fmt.Errorf("failed to update existing API keys: %v", err)
    }
    return nil
}

// listAPIKeys lists all API keys separated into active and inactive
func (m *APIKeyManager) listAPIKeys() {
    activeKeys := []APIKey{}
    inactiveKeys := []APIKey{}
    now := time.Now().UTC()
    m.cache.keyToAPIKey.Range(func(key, value interface{}) bool {
        apiKey, ok := value.(*APIKey)
        if !ok {
            return true
        }
        if apiKey.Expiration.After(now) {
            activeKeys = append(activeKeys, *apiKey)
        } else {
            inactiveKeys = append(inactiveKeys, *apiKey)
        }
        return true
    })

    if len(activeKeys) > 0 {
        log.Println("Active API Keys:")
        for _, key := range activeKeys {
            log.Printf("Name: %s | Key: %s | Expiration: %s | RPM: %d | TL: %d | Limit: %d",
                key.Name, key.ID, key.Expiration.Format(time.RFC3339), key.RPM, key.ThreadsLimit, key.TotalRequests)
        }
    } else {
        log.Println("No active API keys found.")
    }

    if len(inactiveKeys) > 0 {
        log.Println("Inactive API Keys:")
        for _, key := range inactiveKeys {
            log.Printf("Name: %s | Key: %s | Expiration: %s | RPM: %d | TL: %d | Limit: %d",
                key.Name, key.ID, key.Expiration.Format(time.RFC3339), key.RPM, key.ThreadsLimit, key.TotalRequests)
        }
    } else {
        log.Println("No inactive API keys found.")
    }
}

// FixedWindowRateLimiter implements a fixed window rate limiter
type FixedWindowRateLimiter struct {
    windowStart int64
    count       int64
    limit       int64
    mutex       sync.Mutex
}

// NewFixedWindowRateLimiter initializes a new FixedWindowRateLimiter
func NewFixedWindowRateLimiter(limit int) *FixedWindowRateLimiter {
    return &FixedWindowRateLimiter{
        windowStart: time.Now().UTC().Unix(),
        limit:       int64(limit),
    }
}

// Allow checks if a request is allowed under the rate limit
func (fw *FixedWindowRateLimiter) Allow() bool {
    fw.mutex.Lock()
    defer fw.mutex.Unlock()

    now := time.Now().UTC().Unix()
    if now-fw.windowStart >= 60 {
        fw.windowStart = now
        fw.count = 1
        return true
    }

    if fw.count < fw.limit {
        fw.count++
        return true
    }

    return false
}

// generateRandomKey generates a random alphanumeric string of the specified length
func generateRandomKey(length int) (string, error) {
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    b := make([]byte, length)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    for i := range b {
        b[i] = charset[int(b[i])%len(charset)]
    }
    return string(b), nil
}

// parseExpiration parses the expiration string into a time.Duration
func parseExpiration(expirationStr string) (time.Duration, error) {
    var numericPart string
    var unit string
    for i := 0; i < len(expirationStr); i++ {
        if expirationStr[i] < '0' || expirationStr[i] > '9' {
            numericPart = expirationStr[:i]
            unit = expirationStr[i:]
            break
        }
    }
    if numericPart == "" || unit == "" {
        return 0, errors.New("invalid expiration format")
    }
    value, err := strconv.Atoi(numericPart)
    if err != nil {
        return 0, errors.New("invalid numeric value in expiration")
    }
    var duration time.Duration
    switch unit {
    case "m":
        duration = time.Duration(value) * time.Minute
    case "h":
        duration = time.Duration(value) * time.Hour
    case "d":
        duration = time.Duration(value) * 24 * time.Hour
    case "w":
        duration = time.Duration(value) * 7 * 24 * time.Hour
    case "mo":
        duration = time.Duration(value) * 30 * 24 * time.Hour
    case "y":
        duration = time.Duration(value) * 365 * 24 * time.Hour
    default:
        return 0, errors.New("invalid expiration unit")
    }
    return duration, nil
}

// generateAPIKey creates a new API key with the specified parameters
func (m *APIKeyManager) generateAPIKey(customKey string, rpm int, expirationStr string, name string, threadsLimit int, totalRequests int64) (*APIKey, error) {
    expirationDuration, err := parseExpiration(expirationStr)
    if err != nil {
        return nil, err
    }

    if customKey != "" {
        if _, exists := m.cache.GetAPIKey(customKey); exists {
            return nil, errors.New("custom API key already exists")
        }
    } else {
        for i := 0; i < 5; i++ {
            customKey, err = generateRandomKey(32)
            if err != nil {
                return nil, err
            }
            if _, exists := m.cache.GetAPIKey(customKey); !exists {
                break
            }
            customKey = ""
        }
        if customKey == "" {
            return nil, errors.New("failed to generate a unique API key")
        }
    }

    apiKey := &APIKey{
        ID:            customKey,
        Name:          name,
        Expiration:    time.Now().UTC().Add(expirationDuration),
        RPM:           rpm,
        ThreadsLimit:  threadsLimit,
        TotalRequests: totalRequests,
    }

    err = m.SaveAPIKey(apiKey)
    if err != nil {
        return nil, err
    }

    m.cache.SetAPIKey(apiKey)
    if rpm > 0 {
        m.rateLimiters.Store(apiKey.ID, NewFixedWindowRateLimiter(rpm))
    }
    return apiKey, nil
}

// SaveAPIKey saves or updates an API key in MongoDB
func (m *APIKeyManager) SaveAPIKey(apiKey *APIKey) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _, err := m.apiKeysCollection.ReplaceOne(ctx, bson.M{"_id": apiKey.ID}, apiKey, options.Replace().SetUpsert(true))
    if err != nil {
        return fmt.Errorf("failed to save API key: %v", err)
    }
    return nil
}

// Handler functions

// generateAPIKeyHandler handles the generation of new API keys
func (m *APIKeyManager) generateAPIKeyHandler(c *gin.Context) {
    expiration := c.Query("expiration")
    if expiration == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Expiration is mandatory"})
        return
    }

    rpm := parseQueryParam(c, "rpm")
    threadsLimit := parseQueryParam(c, "tl")
    totalRequests := parseQueryParamInt64(c, "limit")
    customAPIKey := c.Query("apikey")
    name := c.Query("name")

    apiKey, err := m.generateAPIKey(customAPIKey, rpm, expiration, name, threadsLimit, totalRequests)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("API Key Generated | Name: %s | Key: %s | Expiration: %s | RPM: %d | TL: %d | Limit: %d",
        apiKey.Name, apiKey.ID, apiKey.Expiration.Format(time.RFC3339), apiKey.RPM, apiKey.ThreadsLimit, apiKey.TotalRequests)

    response := gin.H{
        "message":    "API Key Generated Successfully!",
        "key":        apiKey.ID,
        "expiration": apiKey.Expiration.Format(time.RFC3339),
        "rpm":        apiKey.RPM,
        "tl":         apiKey.ThreadsLimit,
        "limit":      apiKey.TotalRequests,
    }
    if name != "" {
        response["name"] = apiKey.Name
    }
    c.JSON(http.StatusOK, response)
}

// updateAPIKeyHandler handles the updating of existing API keys
func (m *APIKeyManager) updateAPIKeyHandler(c *gin.Context) {
    apiKeyStr := c.Query("apikey")
    if apiKeyStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "API key is mandatory"})
        return
    }

    apiKey, exists := m.cache.GetAPIKey(apiKeyStr)
    if !exists {
        c.JSON(http.StatusBadRequest, gin.H{"error": "API key not found"})
        return
    }

    original := *apiKey

    newExpiration := c.Query("expiration")
    if newExpiration != "" {
        expirationDuration, err := parseExpiration(newExpiration)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
        if apiKey.Expiration.Before(time.Now().UTC()) {
            apiKey.Expiration = time.Now().UTC().Add(expirationDuration)
        } else {
            apiKey.Expiration = apiKey.Expiration.Add(expirationDuration)
        }
    }

    rpm := parseQueryParam(c, "rpm")
    if c.Query("rpm") != "" {
        apiKey.RPM = rpm
    }

    threadsLimit := parseQueryParam(c, "tl")
    if c.Query("tl") != "" {
        apiKey.ThreadsLimit = threadsLimit
    }

    totalRequests := parseQueryParamInt64(c, "limit")
    if c.Query("limit") != "" {
        apiKey.TotalRequests = totalRequests
    }

    newName := c.Query("name")
    if newName != "" {
        apiKey.Name = newName
    }

    m.cache.SetAPIKey(apiKey)
    if apiKey.RPM > 0 {
        m.rateLimiters.Store(apiKey.ID, NewFixedWindowRateLimiter(apiKey.RPM))
    } else {
        m.rateLimiters.Delete(apiKey.ID)
    }

    err := m.SaveAPIKey(apiKey)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update API key"})
        return
    }

    changes := original.Changes(apiKey)
    if len(changes) > 0 {
        log.Printf("API Key Updated | Name: %s | Key: %s | Changes: %s",
            apiKey.Name, apiKey.ID, strings.Join(changes, "; "))
    } else {
        log.Printf("API Key Synchronized without changes | Name: %s | Key: %s",
            apiKey.Name, apiKey.ID)
    }

    response := gin.H{
        "message":    "API Key Updated Successfully!",
        "key":        apiKey.ID,
        "expiration": apiKey.Expiration.Format(time.RFC3339),
        "rpm":        apiKey.RPM,
        "tl":         apiKey.ThreadsLimit,
        "limit":      apiKey.TotalRequests,
    }
    if apiKey.Name != "" {
        response["name"] = apiKey.Name
    }
    c.JSON(http.StatusOK, response)
}

// getAPIKeyInfoHandler retrieves information about a specific API key
func (m *APIKeyManager) getAPIKeyInfoHandler(c *gin.Context) {
    apiKeyStr := c.Query("apikey")
    if apiKeyStr == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "API key is mandatory"})
        return
    }

    apiKey, exists := m.cache.GetAPIKey(apiKeyStr)
    if !exists {
        c.JSON(http.StatusBadRequest, gin.H{"error": "API key not found"})
        return
    }

    response := gin.H{
        "key":        apiKey.ID,
        "expiration": apiKey.Expiration.Format(time.RFC3339),
        "rpm":        apiKey.RPM,
        "tl":         apiKey.ThreadsLimit,
        "limit":      apiKey.TotalRequests,
        "name":       apiKey.Name,
    }
    c.JSON(http.StatusOK, response)
}

// cleanExpiredAPIKeysHandler cleans expired API keys or deletes a specific API key
func (m *APIKeyManager) cleanExpiredAPIKeysHandler(c *gin.Context) {
    apiKeyStr := c.Query("apikey")
    now := time.Now().UTC()
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if apiKeyStr != "" {
        res, err := m.apiKeysCollection.DeleteOne(ctx, bson.M{"_id": apiKeyStr})
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
            return
        }
        if res.DeletedCount > 0 {
            m.cache.DeleteAPIKey(apiKeyStr)
            m.rateLimiters.Delete(apiKeyStr)
            log.Printf("API Key Deleted | Key: %s", apiKeyStr)
            c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Deleted API key: %s", apiKeyStr)})
            return
        }
        c.JSON(http.StatusOK, gin.H{"message": "No API key deleted"})
        return
    }

    filter := bson.M{"Expiration": bson.M{"$lt": now}}
    cursor, err := m.apiKeysCollection.Find(ctx, filter)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clean expired API keys"})
        return
    }
    defer cursor.Close(ctx)

    expiredKeys := []string{}
    for cursor.Next(ctx) {
        var apiKey APIKey
        if err := cursor.Decode(&apiKey); err != nil {
            continue
        }
        expiredKeys = append(expiredKeys, apiKey.ID)
    }

    if len(expiredKeys) == 0 {
        c.JSON(http.StatusOK, gin.H{"message": "No expired API keys to clean"})
        return
    }

    res, err := m.apiKeysCollection.DeleteMany(ctx, filter)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clean expired API keys"})
        return
    }

    for _, key := range expiredKeys {
        m.cache.DeleteAPIKey(key)
        m.rateLimiters.Delete(key)
    }

    log.Printf("Cleaned %d expired API keys | Deleted Keys: %v", res.DeletedCount, expiredKeys)
    c.JSON(http.StatusOK, gin.H{
        "message":      fmt.Sprintf("Cleaned %d expired API key(s)", res.DeletedCount),
        "deleted_keys": expiredKeys,
    })
}

// Helper functions

func parseQueryParam(c *gin.Context, param string) int {
    valueStr := c.Query(param)
    if valueStr != "" {
        value, err := strconv.Atoi(valueStr)
        if err == nil {
            return value
        }
    }
    return 0
}

func parseQueryParamInt64(c *gin.Context, param string) int64 {
    valueStr := c.Query(param)
    if valueStr != "" {
        value, err := strconv.ParseInt(valueStr, 10, 64)
        if err == nil {
            return value
        }
    }
    return 0
}

// shutdown gracefully shuts down the server and background workers
func (m *APIKeyManager) shutdown() {
    close(m.stopChan)
    m.wg.Wait()
    if m.mongoClient != nil {
        m.mongoClient.Disconnect(context.Background())
    }
    log.Println("APIKeyManager shutdown complete. Server exited gracefully.")
}

func main() {
    // Utilize all CPU cores
    runtime.GOMAXPROCS(runtime.NumCPU())

    // Set Gin to release mode
    gin.SetMode(gin.ReleaseMode)

    // Load configuration
    config, err := loadConfig("server.json")
    if err != nil {
        log.Fatalf("Error loading config: %v", err)
    }

    // Create a new APIKeyManager
    manager := NewAPIKeyManager(config)

    // Start watching for changes in the configuration file
    go manager.watchConfigAndReload("server.json")

    // Connect to MongoDB
    if err := manager.connectMongo(); err != nil {
        log.Fatalf("Error connecting to MongoDB: %v", err)
    }

    // Update existing keys with default values for new fields
    if err := manager.updateExistingKeys(); err != nil {
        log.Printf("Error updating existing API keys: %v", err)
    }

    // Load API keys into the cache
    if err := manager.loadAPIKeysToCache(); err == nil {
        manager.listAPIKeys()
    } else {
        log.Printf("Error loading API keys to cache: %v", err)
    }

    // Initialize Gin router
    router := gin.New()

    // Define routes
    router.GET("/generate-api-key", manager.generateAPIKeyHandler)
    router.GET("/update-api-key", manager.updateAPIKeyHandler)
    router.GET("/info-api-key", manager.getAPIKeyInfoHandler)
    router.GET("/clean-api-key", manager.cleanExpiredAPIKeysHandler)

    // Create the HTTP server
    server := &http.Server{
        Addr:         ":" + manager.config.ServerPort,
        Handler:      router,
        ReadTimeout:  time.Duration(manager.config.ReadTimeout) * time.Second,
        WriteTimeout: time.Duration(manager.config.WriteTimeout) * time.Second,
        IdleTimeout:  time.Duration(manager.config.IdleTimeout) * time.Second,
    }

    // Start the server in a separate goroutine
    go func() {
        log.Printf("Server is running on port %s", manager.config.ServerPort)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server error: %v", err)
        }
    }()

    // Wait for interrupt signal to gracefully shut down the server
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt)
    <-stop

    // Initiate shutdown
    log.Println("Shutting down the server...")
    manager.shutdown()

    // Create a context with timeout for the shutdown
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    // Attempt graceful shutdown
    if err := server.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }

    log.Println("Server exited gracefully.")
}
