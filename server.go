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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"golang.org/x/sync/singleflight"
)

type Config struct {
	ServerPort        string `json:"serverPort"`
	MongoURI          string `json:"mongoURI"`
	DatabaseName      string `json:"databaseName"`
	ApiKeysCollection string `json:"apiKeysCollection"`
	ReadTimeout       int    `json:"readTimeout"`
	WriteTimeout      int    `json:"writeTimeout"`
	IdleTimeout       int    `json:"idleTimeout"`
	CleanupInterval   int    `json:"cleanupInterval"`
}

type APIKey struct {
	ID            string    `bson:"_id" json:"key"`
	Name          string    `bson:"Name,omitempty" json:"name,omitempty"`
	Expiration    time.Time `bson:"Expiration" json:"expiration"`
	RPM           int       `bson:"RPM" json:"rpm"`
	ThreadsLimit  int       `bson:"ThreadsLimit" json:"threads_limit"`
	TotalRequests int64     `bson:"TotalRequests" json:"total_requests"`
	Active        bool      `bson:"Active" json:"active"`
	Created       time.Time `bson:"Created" json:"created"`
	LastUsed      time.Time `bson:"LastUsed,omitempty" json:"last_used,omitempty"`
	RequestCount  int64     `bson:"RequestCount,omitempty" json:"request_count,omitempty"`
}

type KeyState struct {
	RequestCount int64
	LastUpdated  time.Time
}

type APIKeyManager struct {
	mongoClient       *mongo.Client
	apiKeysCollection *mongo.Collection
	cache             sync.Map
	keyStates         sync.Map
	rateLimiters      sync.Map
	concurrencySlots  sync.Map
	config            *Config
	configMutex       sync.RWMutex
	sfGroup           singleflight.Group
	stopChan          chan struct{}
	wg                sync.WaitGroup
	dbConnected       atomic.Bool
}

type RateLimiter struct {
	requests   []time.Time
	mutex      sync.Mutex
	limit      int
	windowSize time.Duration
}

func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{
		limit:      limit,
		requests:   make([]time.Time, 0, limit),
		windowSize: time.Minute,
	}
}

func (rl *RateLimiter) Allow() bool {
	if rl.limit <= 0 {
		return true
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.windowSize)

	newIndex := 0
	for _, timestamp := range rl.requests {
		if timestamp.After(cutoff) {
			rl.requests[newIndex] = timestamp
			newIndex++
		}
	}
	rl.requests = rl.requests[:newIndex]

	if len(rl.requests) >= rl.limit {
		return false
	}

	rl.requests = append(rl.requests, now)
	return true
}

func loadConfig() (*Config, error) {
	defaultConfig := &Config{
		ServerPort:        "8080",
		MongoURI:          "mongodb://localhost:27017",
		DatabaseName:      "apiKeyManager",
		ApiKeysCollection: "apiKeys",
		ReadTimeout:       30,
		WriteTimeout:      30,
		IdleTimeout:       60,
		CleanupInterval:   60,
	}

	file, err := os.Open("config.json")
	if err != nil {
		log.Printf("Config file not found, using defaults: %v", err)
		return defaultConfig, nil
	}
	defer file.Close()

	config := defaultConfig
	if err := json.NewDecoder(file).Decode(config); err != nil {
		log.Printf("Error parsing config, using defaults: %v", err)
		return defaultConfig, nil
	}

	// Apply environment variable overrides if they exist
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		config.ServerPort = envPort
	}
	if envMongoURI := os.Getenv("MONGO_URI"); envMongoURI != "" {
		config.MongoURI = envMongoURI
	}
	if envDBName := os.Getenv("DB_NAME"); envDBName != "" {
		config.DatabaseName = envDBName
	}
	if envCollection := os.Getenv("API_KEYS_COLLECTION"); envCollection != "" {
		config.ApiKeysCollection = envCollection
	}

	return config, nil
}

func NewAPIKeyManager(config *Config) *APIKeyManager {
	return &APIKeyManager{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

func (m *APIKeyManager) connectMongo(ctx context.Context) error {
	clientOptions := options.Client().
		ApplyURI(m.config.MongoURI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetRetryWrites(true).
		SetRetryReads(true).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(5 * time.Second)

	var err error
	m.mongoClient, err = mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err = m.mongoClient.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	m.apiKeysCollection = m.mongoClient.Database(m.config.DatabaseName).Collection(m.config.ApiKeysCollection)
	m.dbConnected.Store(true)
	log.Println("Connected to MongoDB")
	return nil
}

func (m *APIKeyManager) ensureMongoConnection() bool {
	if m.dbConnected.Load() {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.connectMongo(ctx); err != nil {
		log.Printf("Failed to reconnect to MongoDB: %v", err)
		return false
	}

	return true
}

func (m *APIKeyManager) loadAPIKeysToCache() error {
	if !m.ensureMongoConnection() {
		return errors.New("database connection not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := options.Find().SetBatchSize(100)
	cursor, err := m.apiKeysCollection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return fmt.Errorf("failed to find API keys: %w", err)
	}
	defer cursor.Close(ctx)

	count := 0
	for cursor.Next(ctx) {
		var apiKey APIKey
		if err := cursor.Decode(&apiKey); err != nil {
			log.Printf("Error decoding API key: %v", err)
			continue
		}

		m.cache.Store(apiKey.ID, &apiKey)

		if apiKey.RPM > 0 && apiKey.Active {
			m.rateLimiters.Store(apiKey.ID, NewRateLimiter(apiKey.RPM))
		}

		if apiKey.ThreadsLimit > 0 && apiKey.Active {
			m.concurrencySlots.Store(apiKey.ID, make(chan struct{}, apiKey.ThreadsLimit))
		}

		m.keyStates.Store(apiKey.ID, &KeyState{
			RequestCount: apiKey.RequestCount,
			LastUpdated:  time.Now(),
		})

		count++
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error: %w", err)
	}

	log.Printf("Loaded %d API keys into cache", count)
	return nil
}

func (m *APIKeyManager) startCleanupWorker() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(time.Duration(m.config.CleanupInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.cleanupExpiredKeys()
				m.flushRequestCounts()
			case <-m.stopChan:
				return
			}
		}
	}()
}

func (m *APIKeyManager) cleanupExpiredKeys() {
	if !m.ensureMongoConnection() {
		log.Println("Skipping cleanup, database connection not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	filter := bson.M{"Expiration": bson.M{"$lt": now}}

	cursor, err := m.apiKeysCollection.Find(ctx, filter)
	if err != nil {
		log.Printf("Error finding expired keys: %v", err)
		return
	}
	defer cursor.Close(ctx)

	var expiredKeys []string
	for cursor.Next(ctx) {
		var apiKey APIKey
		if err := cursor.Decode(&apiKey); err != nil {
			continue
		}
		expiredKeys = append(expiredKeys, apiKey.ID)
	}

	if len(expiredKeys) == 0 {
		return
	}

	_, err = m.apiKeysCollection.DeleteMany(ctx, filter)
	if err != nil {
		log.Printf("Error deleting expired keys: %v", err)
		return
	}

	for _, key := range expiredKeys {
		m.cache.Delete(key)
		m.rateLimiters.Delete(key)
		m.concurrencySlots.Delete(key)
		m.keyStates.Delete(key)
	}

	log.Printf("Cleaned up %d expired API keys", len(expiredKeys))
}

func (m *APIKeyManager) flushRequestCounts() {
	if !m.ensureMongoConnection() {
		log.Println("Skipping request count flush, database connection not available")
		return
	}

	var operations []mongo.WriteModel
	var keysToUpdate []string

	m.keyStates.Range(func(key, value interface{}) bool {
		keyID, ok := key.(string)
		if !ok {
			return true
		}

		state, ok := value.(*KeyState)
		if !ok {
			return true
		}

		// Only flush if there's actual activity and it hasn't been updated recently
		if state.RequestCount > 0 && time.Since(state.LastUpdated) > time.Minute {
			apiKeyValue, exists := m.cache.Load(keyID)
			if !exists {
				return true
			}

			apiKey, ok := apiKeyValue.(*APIKey)
			if !ok {
				return true
			}

			operation := mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": keyID}).
				SetUpdate(bson.M{
					"$set": bson.M{"LastUsed": time.Now().UTC()},
					"$inc": bson.M{"RequestCount": state.RequestCount},
				})

			operations = append(operations, operation)
			keysToUpdate = append(keysToUpdate, keyID)

			// Update the API key in cache
			apiKey.RequestCount += state.RequestCount
			apiKey.LastUsed = time.Now().UTC()
			m.cache.Store(keyID, apiKey)

			// Reset the state
			state.RequestCount = 0
			state.LastUpdated = time.Now()
			m.keyStates.Store(keyID, state)
		}

		return true
	})

	if len(operations) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := options.BulkWrite().SetOrdered(false)
	_, err := m.apiKeysCollection.BulkWrite(ctx, operations, opts)
	if err != nil {
		log.Printf("Error flushing request counts: %v", err)
	} else {
		log.Printf("Flushed request counts for %d API keys", len(keysToUpdate))
	}
}

func generateRandomKey(length int) (string, error) {
	if length <= 0 {
		length = 32
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

func parseExpiration(expirationStr string) (time.Duration, error) {
	expirationStr = strings.TrimSpace(expirationStr)
	if expirationStr == "" {
		return 0, errors.New("empty expiration string")
	}

	var numericPart, unit string
	for i, char := range expirationStr {
		if char < '0' || char > '9' {
			numericPart = expirationStr[:i]
			unit = expirationStr[i:]
			break
		}
	}

	if numericPart == "" || unit == "" {
		return 0, errors.New("invalid expiration format, expected number followed by unit (e.g. 24h, 7d)")
	}

	value, err := strconv.Atoi(numericPart)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in expiration: %w", err)
	}

	if value <= 0 {
		return 0, errors.New("expiration value must be positive")
	}

	switch unit {
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "w":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	case "mo":
		return time.Duration(value) * 30 * 24 * time.Hour, nil
	case "y":
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid expiration unit: %s, expected m, h, d, w, mo, or y", unit)
	}
}

func (m *APIKeyManager) generateAPIKey(c *gin.Context) {
	if !m.ensureMongoConnection() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database connection not available"})
		return
	}

	expiration := c.Query("expiration")
	if expiration == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expiration parameter is required"})
		return
	}

	rpm := parseQueryParam(c, "rpm", 0)
	threadsLimit := parseQueryParam(c, "tl", 0)
	totalRequests := parseQueryParamInt64(c, "limit", 0)
	customAPIKey := c.Query("apikey")
	name := c.Query("name")

	expirationDuration, err := parseExpiration(expiration)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid expiration format: %v", err)})
		return
	}

	if customAPIKey != "" {
		if _, exists := m.cache.Load(customAPIKey); exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Custom API key already exists"})
			return
		}
	} else {
		for i := 0; i < 5; i++ {
			customAPIKey, err = generateRandomKey(32)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate API key"})
				return
			}
			if _, exists := m.cache.Load(customAPIKey); !exists {
				break
			}
			customAPIKey = ""
		}
		if customAPIKey == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate a unique API key after multiple attempts"})
			return
		}
	}

	now := time.Now().UTC()
	apiKey := &APIKey{
		ID:            customAPIKey,
		Name:          name,
		Expiration:    now.Add(expirationDuration),
		RPM:           rpm,
		ThreadsLimit:  threadsLimit,
		TotalRequests: totalRequests,
		Active:        true,
		Created:       now,
		RequestCount:  0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = m.apiKeysCollection.ReplaceOne(ctx, bson.M{"_id": apiKey.ID}, apiKey, options.Replace().SetUpsert(true))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save API key: %v", err)})
		return
	}

	m.cache.Store(apiKey.ID, apiKey)

	if rpm > 0 {
		m.rateLimiters.Store(apiKey.ID, NewRateLimiter(rpm))
	}

	if threadsLimit > 0 {
		m.concurrencySlots.Store(apiKey.ID, make(chan struct{}, threadsLimit))
	}

	m.keyStates.Store(apiKey.ID, &KeyState{
		RequestCount: 0,
		LastUpdated:  now,
	})

	response := gin.H{
		"message":    "API Key Generated Successfully",
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

func (m *APIKeyManager) updateAPIKey(c *gin.Context) {
	if !m.ensureMongoConnection() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database connection not available"})
		return
	}

	apiKeyStr := c.Query("apikey")
	if apiKeyStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key parameter is required"})
		return
	}

	value, exists := m.cache.Load(apiKeyStr)
	if !exists {
		// Try to load from database if not in cache
		result, err, _ := m.sfGroup.Do(apiKeyStr, func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var apiKeyDB APIKey
			err := m.apiKeysCollection.FindOne(ctx, bson.M{"_id": apiKeyStr}).Decode(&apiKeyDB)
			if err != nil {
				if err == mongo.ErrNoDocuments {
					return nil, nil
				}
				return nil, err
			}

			m.cache.Store(apiKeyStr, &apiKeyDB)
			return &apiKeyDB, nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database error: %v", err)})
			return
		}

		if result == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}

		value = result
	}

	apiKey, ok := value.(*APIKey)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid cache entry type"})
		return
	}

	// Make a copy to track changes
	originalAPIKey := *apiKey

	// Process updates
	if newExpiration := c.Query("expiration"); newExpiration != "" {
		expirationDuration, err := parseExpiration(newExpiration)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid expiration format: %v", err)})
			return
		}

		now := time.Now().UTC()
		if apiKey.Expiration.Before(now) {
			apiKey.Expiration = now.Add(expirationDuration)
		} else {
			apiKey.Expiration = apiKey.Expiration.Add(expirationDuration)
		}
	}

	if c.Query("rpm") != "" {
		apiKey.RPM = parseQueryParam(c, "rpm", 0)
		m.rateLimiters.Delete(apiKey.ID)
		if apiKey.RPM > 0 && apiKey.Active {
			m.rateLimiters.Store(apiKey.ID, NewRateLimiter(apiKey.RPM))
		}
	}

	if c.Query("tl") != "" {
		apiKey.ThreadsLimit = parseQueryParam(c, "tl", 0)
		m.concurrencySlots.Delete(apiKey.ID)
		if apiKey.ThreadsLimit > 0 && apiKey.Active {
			m.concurrencySlots.Store(apiKey.ID, make(chan struct{}, apiKey.ThreadsLimit))
		}
	}

	if c.Query("limit") != "" {
		apiKey.TotalRequests = parseQueryParamInt64(c, "limit", 0)
	}

	if newName := c.Query("name"); newName != "" {
		apiKey.Name = newName
	}

	if activeStr := c.Query("active"); activeStr != "" {
		apiKey.Active = activeStr == "true" || activeStr == "1"

		// If deactivated, remove from rate limiters and concurrency slots
		if !apiKey.Active {
			m.rateLimiters.Delete(apiKey.ID)
			m.concurrencySlots.Delete(apiKey.ID)
		} else if apiKey.Active && !originalAPIKey.Active {
			// If reactivated, add back to rate limiters and concurrency slots
			if apiKey.RPM > 0 {
				m.rateLimiters.Store(apiKey.ID, NewRateLimiter(apiKey.RPM))
			}
			if apiKey.ThreadsLimit > 0 {
				m.concurrencySlots.Store(apiKey.ID, make(chan struct{}, apiKey.ThreadsLimit))
			}
		}
	}

	// Update cache first
	m.cache.Store(apiKey.ID, apiKey)

	// Then update database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.apiKeysCollection.ReplaceOne(ctx, bson.M{"_id": apiKey.ID}, apiKey)
	if err != nil {
		// Revert cache if database update fails
		m.cache.Store(apiKey.ID, &originalAPIKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update API key: %v", err)})
		return
	}

	// Build changes summary
	var changes []string
	if originalAPIKey.Name != apiKey.Name {
		changes = append(changes, fmt.Sprintf("Name: %s → %s", originalAPIKey.Name, apiKey.Name))
	}
	if !originalAPIKey.Expiration.Equal(apiKey.Expiration) {
		changes = append(changes, fmt.Sprintf("Expiration: %s → %s",
			originalAPIKey.Expiration.Format(time.RFC3339),
			apiKey.Expiration.Format(time.RFC3339)))
	}
	if originalAPIKey.RPM != apiKey.RPM {
		changes = append(changes, fmt.Sprintf("RPM: %d → %d", originalAPIKey.RPM, apiKey.RPM))
	}
	if originalAPIKey.ThreadsLimit != apiKey.ThreadsLimit {
		changes = append(changes, fmt.Sprintf("ThreadsLimit: %d → %d", originalAPIKey.ThreadsLimit, apiKey.ThreadsLimit))
	}
	if originalAPIKey.TotalRequests != apiKey.TotalRequests {
		changes = append(changes, fmt.Sprintf("TotalRequests: %d → %d", originalAPIKey.TotalRequests, apiKey.TotalRequests))
	}
	if originalAPIKey.Active != apiKey.Active {
		changes = append(changes, fmt.Sprintf("Active: %t → %t", originalAPIKey.Active, apiKey.Active))
	}

	response := gin.H{
		"message":    "API Key Updated Successfully",
		"key":        apiKey.ID,
		"expiration": apiKey.Expiration.Format(time.RFC3339),
		"rpm":        apiKey.RPM,
		"tl":         apiKey.ThreadsLimit,
		"limit":      apiKey.TotalRequests,
		"active":     apiKey.Active,
	}

	if apiKey.Name != "" {
		response["name"] = apiKey.Name
	}

	if len(changes) > 0 {
		response["changes"] = changes
	}

	c.JSON(http.StatusOK, response)
}

func (m *APIKeyManager) getAPIKeyInfo(c *gin.Context) {
	if !m.ensureMongoConnection() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database connection not available"})
		return
	}

	apiKeyStr := c.Query("apikey")
	if apiKeyStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key parameter is required"})
		return
	}

	value, exists := m.cache.Load(apiKeyStr)
	if !exists {
		// Try to load from database if not in cache
		result, err, _ := m.sfGroup.Do(apiKeyStr, func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var apiKeyDB APIKey
			err := m.apiKeysCollection.FindOne(ctx, bson.M{"_id": apiKeyStr}).Decode(&apiKeyDB)
			if err != nil {
				if err == mongo.ErrNoDocuments {
					return nil, nil
				}
				return nil, err
			}

			m.cache.Store(apiKeyStr, &apiKeyDB)
			return &apiKeyDB, nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database error: %v", err)})
			return
		}

		if result == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}

		value = result
	}

	apiKey, ok := value.(*APIKey)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid cache entry type"})
		return
	}

	// Get current request count from state and add to API key
	keyStateValue, exists := m.keyStates.Load(apiKeyStr)
	if exists {
		if keyState, ok := keyStateValue.(*KeyState); ok {
			apiKey.RequestCount += keyState.RequestCount
		}
	}

	// Check if the key is valid
	now := time.Now().UTC()
	isExpired := now.After(apiKey.Expiration)
	isValid := apiKey.Active && !isExpired

	response := gin.H{
		"key":           apiKey.ID,
		"expiration":    apiKey.Expiration.Format(time.RFC3339),
		"rpm":           apiKey.RPM,
		"tl":            apiKey.ThreadsLimit,
		"limit":         apiKey.TotalRequests,
		"active":        apiKey.Active,
		"created":       apiKey.Created.Format(time.RFC3339),
		"request_count": apiKey.RequestCount,
		"valid":         isValid,
		"expired":       isExpired,
	}

	if apiKey.Name != "" {
		response["name"] = apiKey.Name
	}

	if !apiKey.LastUsed.IsZero() {
		response["last_used"] = apiKey.LastUsed.Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, response)
}

func (m *APIKeyManager) cleanExpiredAPIKeys(c *gin.Context) {
	if !m.ensureMongoConnection() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database connection not available"})
		return
	}

	apiKeyStr := c.Query("apikey")
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// If a specific API key is provided
	if apiKeyStr != "" {
		// Check if it exists first
		var exists bool
		if _, exists = m.cache.Load(apiKeyStr); !exists {
			// Try to find it in the database
			err := m.apiKeysCollection.FindOne(ctx, bson.M{"_id": apiKeyStr}).Err()
			if err != nil {
				if err == mongo.ErrNoDocuments {
					c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
				} else {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database error: %v", err)})
				}
				return
			}
			exists = true
		}

		// Delete if expired or force delete
		filter := bson.M{"_id": apiKeyStr}
		if forceStr := c.Query("force"); forceStr != "true" && forceStr != "1" {
			filter["Expiration"] = bson.M{"$lt": now}
		}

		res, err := m.apiKeysCollection.DeleteOne(ctx, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to delete API key: %v", err)})
			return
		}

		if res.DeletedCount > 0 {
			m.cache.Delete(apiKeyStr)
			m.rateLimiters.Delete(apiKeyStr)
			m.concurrencySlots.Delete(apiKeyStr)
			m.keyStates.Delete(apiKeyStr)
			c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Deleted API key: %s", apiKeyStr)})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "API key not expired or already deleted"})
		}
		return
	}

	// Otherwise, clean all expired keys
	filter := bson.M{"Expiration": bson.M{"$lt": now}}
	cursor, err := m.apiKeysCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to find expired API keys: %v", err)})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to clean expired API keys: %v", err)})
		return
	}

	for _, key := range expiredKeys {
		m.cache.Delete(key)
		m.rateLimiters.Delete(key)
		m.concurrencySlots.Delete(key)
		m.keyStates.Delete(key)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      fmt.Sprintf("Cleaned %d expired API key(s)", res.DeletedCount),
		"deleted_keys": expiredKeys,
	})
}

func (m *APIKeyManager) validateAPIKey(c *gin.Context) {
	apiKey := c.GetHeader("x-lh-key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "invalid", "message": "Missing API key header (x-lh-key)"})
		return
	}

	cacheValue, exists := m.cache.Load(apiKey)
	if !exists {
		// Try to load from database if not in cache
		if !m.ensureMongoConnection() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Database connection not available"})
			return
		}

		result, err, _ := m.sfGroup.Do(apiKey, func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var apiKeyDB APIKey
			err := m.apiKeysCollection.FindOne(ctx, bson.M{"_id": apiKey}).Decode(&apiKeyDB)
			if err != nil {
				if err == mongo.ErrNoDocuments {
					return nil, nil
				}
				return nil, err
			}

			m.cache.Store(apiKey, &apiKeyDB)

			if apiKeyDB.RPM > 0 && apiKeyDB.Active {
				m.rateLimiters.Store(apiKey, NewRateLimiter(apiKeyDB.RPM))
			}

			if apiKeyDB.ThreadsLimit > 0 && apiKeyDB.Active {
				m.concurrencySlots.Store(apiKey, make(chan struct{}, apiKeyDB.ThreadsLimit))
			}

			m.keyStates.Store(apiKey, &KeyState{
				RequestCount: 0,
				LastUpdated:  time.Now(),
			})

			return &apiKeyDB, nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Database error"})
			return
		}

		if result == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "invalid", "message": "API key not found"})
			return
		}

		cacheValue = result
	}

	apiKeyInfo, ok := cacheValue.(*APIKey)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Invalid cache entry type"})
		return
	}

	// Check if key is active
	if !apiKeyInfo.Active {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "invalid", "message": "API key is inactive"})
		return
	}

	// Check if key is expired
	if time.Now().UTC().After(apiKeyInfo.Expiration) {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "expired", "message": "API key has expired"})
		return
	}

	// Check total request limit
	if apiKeyInfo.TotalRequests > 0 {
		stateValue, _ := m.keyStates.LoadOrStore(apiKey, &KeyState{
			RequestCount: 0,
			LastUpdated:  time.Now(),
		})

		state, ok := stateValue.(*KeyState)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Invalid key state type"})
			return
		}

		totalCount := apiKeyInfo.RequestCount + state.RequestCount + 1
		if totalCount > apiKeyInfo.TotalRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":  "limited",
				"message": "Total request limit exceeded",
				"limit":   apiKeyInfo.TotalRequests,
				"used":    totalCount - 1,
			})
			return
		}

		// Increment request count in state
		state.RequestCount++
		state.LastUpdated = time.Now()
		m.keyStates.Store(apiKey, state)
	}

	// Check rate limit
	rateLimiterValue, exists := m.rateLimiters.Load(apiKey)
	if exists {
		rateLimiter, ok := rateLimiterValue.(*RateLimiter)
		if ok && !rateLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":    "limited",
				"message":   "Rate limit exceeded",
				"limit_rpm": apiKeyInfo.RPM,
			})
			return
		}
	}

	// Check concurrency limit
	concurrencySemValue, exists := m.concurrencySlots.Load(apiKey)
	if exists {
		concurrencySem, ok := concurrencySemValue.(chan struct{})
		if ok {
			select {
			case concurrencySem <- struct{}{}:
				defer func() { <-concurrencySem }()
			default:
				c.JSON(http.StatusTooManyRequests, gin.H{
					"status":        "limited",
					"message":       "Concurrency limit exceeded",
					"limit_threads": apiKeyInfo.ThreadsLimit,
				})
				return
			}
		}
	}

	// All checks passed, key is valid
	c.JSON(http.StatusOK, gin.H{
		"status": "valid",
		"key_info": gin.H{
			"name":          apiKeyInfo.Name,
			"expires_at":    apiKeyInfo.Expiration.Format(time.RFC3339),
			"rpm":           apiKeyInfo.RPM,
			"threads_limit": apiKeyInfo.ThreadsLimit,
		},
	})
}

func parseQueryParam(c *gin.Context, param string, defaultValue int) int {
	valueStr := c.Query(param)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

func parseQueryParamInt64(c *gin.Context, param string, defaultValue int64) int64 {
	valueStr := c.Query(param)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

func (m *APIKeyManager) shutdown(server *http.Server) {
	log.Println("Shutting down API Key Manager...")

	// Signal background workers to stop
	close(m.stopChan)

	// Wait for all goroutines to finish
	log.Println("Waiting for background tasks to complete...")
	m.wg.Wait()

	// Flush any pending request counts to database
	log.Println("Flushing request counts...")
	m.flushRequestCounts()

	// Disconnect from MongoDB
	if m.mongoClient != nil {
		log.Println("Disconnecting from MongoDB...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		m.mongoClient.Disconnect(ctx)
	}

	// Shutdown HTTP server
	log.Println("Shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx)

	log.Println("Shutdown complete")
}

func main() {
	// Set max CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Set logging format
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Starting API Key Manager...")

	// Release mode for Gin
	gin.SetMode(gin.ReleaseMode)

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Printf("Warning: %v", err)
	}

	// Create manager
	manager := NewAPIKeyManager(config)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := manager.connectMongo(ctx); err != nil {
		log.Printf("Warning: Failed to connect to MongoDB: %v", err)
		log.Println("Will retry database connection when needed")
	}
	cancel()

	// Load API keys to cache
	if manager.dbConnected.Load() {
		if err := manager.loadAPIKeysToCache(); err != nil {
			log.Printf("Warning: Failed to load API keys to cache: %v", err)
		}
	}

	// Start background workers
	manager.startCleanupWorker()

	// Create router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors.Default())

	// Setup routes - no authentication required for management endpoints
	router.GET("/generate-api-key", manager.generateAPIKey)
	router.GET("/update-api-key", manager.updateAPIKey)
	router.GET("/info-api-key", manager.getAPIKeyInfo)
	router.GET("/clean-api-key", manager.cleanExpiredAPIKeys)

	// Root endpoint validates API keys using x-lh-key header
	router.Any("/", manager.validateAPIKey)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + manager.config.ServerPort,
		Handler:      router,
		ReadTimeout:  time.Duration(manager.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(manager.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(manager.config.IdleTimeout) * time.Second,
	}

	// Start server
	go func() {
		log.Printf("Server is running on port %s", manager.config.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// Graceful shutdown
	manager.shutdown(server)
}
