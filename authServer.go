// authServer.go

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sync/singleflight"
)

type Configuration struct {
	ServerPort   string
	MongoURI     string
	CacheTTL     time.Duration
	CacheCleanup time.Duration
}

func LoadConfig() *Configuration {
	return &Configuration{
		ServerPort:   "3002",
		MongoURI:     "mongodb+srv://authserver:authserver@apiKeysManager.mmm8e.mongodb.net/?retryWrites=true&w=majority",
		CacheTTL:     5 * time.Minute,
		CacheCleanup: 10 * time.Minute,
	}
}

type APIKeyInfo struct {
	Key           string    `bson:"_id"`
	Name          string    `bson:"Name,omitempty"`
	Expiration    time.Time `bson:"Expiration"`
	RPM           int       `bson:"RPM"`
	ThreadsLimit  int       `bson:"tl"`
	TotalRequests int64     `bson:"TotalRequests"`
}

type FixedWindowRateLimiter struct {
	windowStart time.Time
	count       int
	limit       int
	mutex       sync.Mutex
}

func NewFixedWindowRateLimiter(limit int) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		windowStart: time.Now(),
		count:       0,
		limit:       limit,
	}
}

func (f *FixedWindowRateLimiter) Allow() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	now := time.Now()
	if now.Sub(f.windowStart) >= time.Minute {
		f.windowStart = now
		f.count = 1
		return true
	}

	if f.count < f.limit || f.limit == 0 {
		f.count++
		return true
	}

	return false
}

type APIKeyState struct {
	rateLimiter    *FixedWindowRateLimiter
	concurrencySem chan struct{}
}

type AuthManager struct {
	cache              *cache.Cache
	sfGroup            singleflight.Group
	mongoClient        *mongo.Client
	apiKeysCollection  *mongo.Collection
	config             *Configuration
	keyStates          sync.Map
	changeStreamCtx    context.Context
	changeStreamCancel context.CancelFunc
}

type CacheEntry struct {
	APIKeyInfo       *APIKeyInfo
	ValidationResult string
	Timestamp        time.Time
}

func NewAuthManager(config *Configuration) *AuthManager {
	return &AuthManager{
		cache:  cache.New(config.CacheTTL, config.CacheCleanup),
		config: config,
	}
}

func (m *AuthManager) ConnectMongo() error {
	clientOptions := options.Client().
		ApplyURI(m.config.MongoURI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetRetryReads(true).
		SetRetryWrites(true).
		SetConnectTimeout(10 * time.Second).
		SetSocketTimeout(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	m.mongoClient, err = mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()

	if err := m.mongoClient.Ping(ctxPing, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	m.apiKeysCollection = m.mongoClient.Database("apiKeyManager").Collection("apiKeys")
	return nil
}

func (m *AuthManager) StartChangeStreamWatcher() {
	ctx, cancel := context.WithCancel(context.Background())
	m.changeStreamCtx = ctx
	m.changeStreamCancel = cancel

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "operationType", Value: bson.D{{Key: "$in", Value: bson.A{"insert", "update", "replace", "delete"}}}},
		}}},
	}

	changeStreamOptions := options.ChangeStream().SetFullDocument(options.UpdateLookup)

	changeStream, err := m.apiKeysCollection.Watch(ctx, pipeline, changeStreamOptions)
	if err != nil {
		log.Fatalf("Failed to start change stream: %v", err)
	}

	go func() {
		defer changeStream.Close(ctx)
		for {
			if changeStream.Next(ctx) {
				var event bson.M
				if err := changeStream.Decode(&event); err != nil {
					continue
				}
				m.processChangeEvent(event)
			} else if err := changeStream.Err(); err != nil {
				time.Sleep(time.Second)
			} else {
				if ctx.Err() != nil {
					return
				}
			}
		}
	}()
}

func (m *AuthManager) processChangeEvent(event bson.M) {
	operationType, ok := event["operationType"].(string)
	if !ok {
		return
	}

	switch operationType {
	case "insert", "update", "replace":
		fullDocument, ok := event["fullDocument"].(bson.M)
		if !ok {
			return
		}

		apiKey, ok := fullDocument["_id"].(string)
		if !ok {
			return
		}

		expiration, err := parseTime(fullDocument["Expiration"])
		if err != nil {
			return
		}

		rpm, _ := toInt(fullDocument["RPM"])
		threadsLimit, _ := toInt(fullDocument["tl"])
		totalRequests, _ := toInt64(fullDocument["TotalRequests"])
		name, _ := fullDocument["Name"].(string)

		apiKeyInfo := &APIKeyInfo{
			Key:           apiKey,
			Name:          name,
			Expiration:    expiration,
			RPM:           rpm,
			ThreadsLimit:  threadsLimit,
			TotalRequests: totalRequests,
		}

		m.cache.Set(apiKey, &CacheEntry{
			APIKeyInfo:       apiKeyInfo,
			ValidationResult: "valid",
			Timestamp:        time.Now(),
		}, m.config.CacheTTL)

		m.keyStates.Delete(apiKey)

	case "delete":
		documentKey, ok := event["documentKey"].(bson.M)
		if !ok {
			return
		}

		apiKey, ok := documentKey["_id"].(string)
		if !ok {
			return
		}

		m.cache.Delete(apiKey)
		m.keyStates.Delete(apiKey)
	}
}

func parseTime(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case primitive.DateTime:
		return v.Time(), nil
	case bson.M:
		if dateStr, ok := v["$date"].(string); ok {
			return time.Parse(time.RFC3339, dateStr)
		}
		if dateInt, ok := v["$date"].(int64); ok {
			return time.Unix(0, dateInt*int64(time.Millisecond)), nil
		}
	}
	return time.Time{}, errors.New("invalid time format")
}

func toInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, errors.New("cannot convert to int")
	}
}

func toInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	default:
		return 0, errors.New("cannot convert to int64")
	}
}

func (m *AuthManager) getAPIKeyState(apiKey string, apiKeyInfo *APIKeyInfo) *APIKeyState {
	stateInterface, exists := m.keyStates.Load(apiKey)
	if !exists {
		var rateLimiter *FixedWindowRateLimiter
		if apiKeyInfo.RPM > 0 {
			rateLimiter = NewFixedWindowRateLimiter(apiKeyInfo.RPM)
		} else {
			rateLimiter = NewFixedWindowRateLimiter(0)
		}

		threadsLimit := apiKeyInfo.ThreadsLimit
		if threadsLimit <= 0 {
			threadsLimit = 1
		}

		state := &APIKeyState{
			rateLimiter:    rateLimiter,
			concurrencySem: make(chan struct{}, threadsLimit),
		}
		m.keyStates.Store(apiKey, state)
		return state
	}
	return stateInterface.(*APIKeyState)
}

func (m *AuthManager) ValidateAPIKey(apiKey string) (*APIKeyInfo, string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, "invalid", errors.New("empty API key")
	}

	if entry, found := m.cache.Get(apiKey); found {
		if cacheEntry, ok := entry.(*CacheEntry); ok {
			return cacheEntry.APIKeyInfo, cacheEntry.ValidationResult, nil
		}
		m.cache.Delete(apiKey)
	}

	result, err, _ := m.sfGroup.Do(apiKey, func() (interface{}, error) {
		if entry, found := m.cache.Get(apiKey); found {
			if cacheEntry, ok := entry.(*CacheEntry); ok {
				return cacheEntry, nil
			}
			m.cache.Delete(apiKey)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var apiKeyInfoDB APIKeyInfo
		filter := bson.M{"_id": apiKey}
		err := m.apiKeysCollection.FindOne(ctx, filter).Decode(&apiKeyInfoDB)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				cacheEntry := &CacheEntry{
					APIKeyInfo:       nil,
					ValidationResult: "invalid",
					Timestamp:        time.Now(),
				}
				m.cache.Set(apiKey, cacheEntry, m.config.CacheTTL)
				return cacheEntry, nil
			}
			return nil, fmt.Errorf("error fetching API key from MongoDB: %w", err)
		}

		cacheEntry := &CacheEntry{
			APIKeyInfo:       &apiKeyInfoDB,
			ValidationResult: "valid",
			Timestamp:        time.Now(),
		}
		m.cache.Set(apiKey, cacheEntry, m.config.CacheTTL)

		return cacheEntry, nil
	})

	if err != nil {
		return nil, "error", err
	}

	if cacheEntry, ok := result.(*CacheEntry); ok {
		return cacheEntry.APIKeyInfo, cacheEntry.ValidationResult, nil
	}

	return nil, "invalid", errors.New("invalid cache entry type")
}

func (m *AuthManager) validateAndAcquire(c *gin.Context, apiKey string) (*APIKeyState, bool) {
	apiKeyInfo, validationResult, err := m.ValidateAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error"})
		return nil, false
	}

	if validationResult != "valid" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "invalid"})
		return nil, false
	}

	if time.Now().After(apiKeyInfo.Expiration) {
		m.cache.Set(apiKey, &CacheEntry{
			APIKeyInfo:       nil,
			ValidationResult: "expired",
			Timestamp:        time.Now(),
		}, m.config.CacheTTL)
		c.JSON(http.StatusUnauthorized, gin.H{"status": "expired"})
		return nil, false
	}

	if apiKeyInfo.TotalRequests > 0 {
		remaining := atomic.AddInt64(&apiKeyInfo.TotalRequests, -1)
		if remaining < 0 {
			m.cache.Set(apiKey, &CacheEntry{
				APIKeyInfo:       nil,
				ValidationResult: "limited",
				Timestamp:        time.Now(),
			}, m.config.CacheTTL)
			c.JSON(http.StatusTooManyRequests, gin.H{"status": "limited"})
			return nil, false
		}
	}

	state := m.getAPIKeyState(apiKey, apiKeyInfo)

	if state.rateLimiter.limit > 0 && !state.rateLimiter.Allow() {
		c.JSON(http.StatusTooManyRequests, gin.H{"status": "limited"})
		return nil, false
	}

	return state, true
}

func (m *AuthManager) handleRequest(c *gin.Context) {
	apiKey := c.GetHeader("x-lh-key")
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "invalid"})
		return
	}

	state, ok := m.validateAndAcquire(c, apiKey)
	if !ok {
		return
	}

	// Try to acquire a concurrency slot
	select {
	case state.concurrencySem <- struct{}{}:
		defer func() {
			<-state.concurrencySem
		}()
	default:
		// Could not acquire concurrency slot
		c.JSON(http.StatusTooManyRequests, gin.H{"status": "limited"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "valid"})
}

func (m *AuthManager) Shutdown(ctx context.Context) error {
	if m.changeStreamCancel != nil {
		m.changeStreamCancel()
	}
	if m.mongoClient != nil {
		if err := m.mongoClient.Disconnect(ctx); err != nil {
			return fmt.Errorf("error disconnecting MongoDB: %w", err)
		}
	}
	return nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	config := LoadConfig()

	manager := NewAuthManager(config)

	if err := manager.ConnectMongo(); err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}

	manager.StartChangeStreamWatcher()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(gin.Recovery())

	router.Any("/auth", manager.handleRequest)

	server := &http.Server{
		Addr:         ":" + config.ServerPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server started on port %s", config.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Server exited gracefully.")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	if err := manager.Shutdown(shutdownCtx); err != nil {
		// Handle error if needed
	}
}
