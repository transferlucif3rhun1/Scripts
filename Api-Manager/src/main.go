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

type LogLevel string

const (
	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
)

type Config struct {
	ServerPort         string `json:"serverPort"`
	MongoURI           string `json:"mongoURI"`
	DatabaseName       string `json:"databaseName"`
	ApiKeysCollection  string `json:"apiKeysCollection"`
	LogsCollection     string `json:"logsCollection"`
	SettingsCollection string `json:"settingsCollection"`
	LogRetention       int    `json:"logRetention"`
	LogBufferSize      int    `json:"logBufferSize"`
	ReadTimeout        int    `json:"readTimeout"`
	WriteTimeout       int    `json:"writeTimeout"`
	IdleTimeout        int    `json:"idleTimeout"`
	CleanupInterval    int    `json:"cleanupInterval"`
}

type Tag struct {
	Name  string `bson:"name" json:"name"`
	Color string `bson:"color,omitempty" json:"color,omitempty"`
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
	Tags          []Tag     `bson:"Tags,omitempty" json:"tags,omitempty"`
}

type KeyTemplate struct {
	ID           string    `bson:"_id,omitempty" json:"id,omitempty"`
	Name         string    `bson:"name" json:"name"`
	Description  string    `bson:"description,omitempty" json:"description,omitempty"`
	RPM          int       `bson:"rpm" json:"rpm"`
	ThreadsLimit int       `bson:"threadsLimit" json:"threads_limit"`
	Duration     string    `bson:"duration" json:"duration"`
	Tags         []Tag     `bson:"tags,omitempty" json:"tags,omitempty"`
	IsDefault    bool      `bson:"isDefault" json:"is_default"`
	Created      time.Time `bson:"created" json:"created"`
	LastModified time.Time `bson:"lastModified" json:"last_modified"`
}

type SystemLog struct {
	ID        string    `bson:"_id,omitempty" json:"id,omitempty"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Level     LogLevel  `bson:"level" json:"level"`
	Component string    `bson:"component" json:"component"`
	Message   string    `bson:"message" json:"message"`
	Details   string    `bson:"details,omitempty" json:"details,omitempty"`
}

type UserSettings struct {
	ID              string      `bson:"_id,omitempty" json:"id,omitempty"`
	Data            interface{} `bson:"data" json:"data"`
	SettingsVersion int         `bson:"settingsVersion" json:"settingsVersion"`
	LastUpdated     time.Time   `bson:"lastUpdated" json:"lastUpdated"`
}

type ApiKeyLog struct {
	KeyID     string    `bson:"keyId" json:"key_id"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Action    string    `bson:"action" json:"action"`
	Path      string    `bson:"path" json:"path"`
	Method    string    `bson:"method" json:"method"`
	IP        string    `bson:"ip" json:"ip"`
	UserAgent string    `bson:"userAgent" json:"user_agent"`
	Status    int       `bson:"status" json:"status"`
	Duration  int64     `bson:"duration" json:"duration"`
}

type KeyState struct {
	RequestCount int64
	LastUpdated  time.Time
}

type MonitoringStats struct {
	TotalKeys         int       `json:"total_keys"`
	ActiveKeys        int       `json:"active_keys"`
	ExpiringKeys      int       `json:"expiring_keys"`
	TotalRequests24h  int64     `json:"total_requests_24h"`
	ErrorRate24h      float64   `json:"error_rate_24h"`
	AvgResponseTime   int64     `json:"avg_response_time"`
	LastUpdated       time.Time `json:"last_updated"`
	SystemStatus      string    `json:"system_status"`
	DatabaseConnected bool      `json:"database_connected"`
}

type PagedResponse struct {
	Data       interface{} `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalItems int64       `json:"total_items"`
	TotalPages int         `json:"total_pages"`
}

type ApiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

type BulkOperationRequest struct {
	Keys []string `json:"keys"`
}

type BulkTagRequest struct {
	Keys    []string `json:"keys"`
	AddTags []Tag    `json:"add_tags,omitempty"`
	DelTags []string `json:"del_tags,omitempty"`
}

type BulkExtendRequest struct {
	Keys      []string `json:"keys"`
	Extension string   `json:"extension"`
}

type BulkOperationResponse struct {
	SuccessCount int      `json:"success_count"`
	ErrorCount   int      `json:"error_count"`
	Errors       []string `json:"errors,omitempty"`
}

type AppService struct {
	config           *Config
	mongoClient      *mongo.Client
	collections      map[string]*mongo.Collection
	dbConnected      atomic.Bool
	sfGroup          singleflight.Group
	cache            sync.Map
	keyStates        sync.Map
	rateLimiters     sync.Map
	concurrencySlots sync.Map
	stopChan         chan struct{}
	wg               sync.WaitGroup
	router           *gin.Engine
}

type RateLimiter struct {
	requests   []time.Time
	mutex      sync.Mutex
	limit      int
	windowSize time.Duration
}

func respond(c *gin.Context, status int, success bool, data interface{}, err string, meta ...interface{}) {
	resp := ApiResponse{Success: success, Data: data, Error: err}
	if len(meta) > 0 {
		resp.Meta = meta[0]
	}
	c.JSON(status, resp)
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

func (s *AppService) fetchKey(id string) (*APIKey, int, error) {
	if id == "" {
		return nil, http.StatusBadRequest, errors.New("API key ID required")
	}

	if v, ok := s.cache.Load(id); ok {
		if key, ok := v.(*APIKey); ok {
			return key, 0, nil
		}
	}

	if !s.dbConnected.Load() {
		return nil, http.StatusServiceUnavailable, errors.New("database connection not available")
	}

	result, err, _ := s.sfGroup.Do(id, func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var key APIKey
		if err := s.collections["apiKeys"].FindOne(ctx, bson.M{"_id": id}).Decode(&key); err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, nil
			}
			return nil, err
		}

		s.cache.Store(id, &key)
		s.initKeyServices(&key)
		return &key, nil
	})

	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if result == nil {
		return nil, http.StatusNotFound, errors.New("API key not found")
	}

	return result.(*APIKey), 0, nil
}

func (s *AppService) initKeyServices(key *APIKey) {
	if key.RPM > 0 && key.Active {
		s.rateLimiters.Store(key.ID, NewRateLimiter(key.RPM))
	}
	if key.ThreadsLimit > 0 && key.Active {
		s.concurrencySlots.Store(key.ID, make(chan struct{}, key.ThreadsLimit))
	}
	s.keyStates.Store(key.ID, &KeyState{
		RequestCount: 0,
		LastUpdated:  time.Now(),
	})
}

func (s *AppService) removeKeyResources(id string) {
	s.cache.Delete(id)
	s.rateLimiters.Delete(id)
	s.concurrencySlots.Delete(id)
	s.keyStates.Delete(id)
}

func (s *AppService) checkRateLimit(key *APIKey) (bool, int) {
	if key.RPM <= 0 {
		return true, 0
	}

	v, _ := s.rateLimiters.LoadOrStore(key.ID, NewRateLimiter(key.RPM))
	if limiter, ok := v.(*RateLimiter); ok && !limiter.Allow() {
		return false, http.StatusTooManyRequests
	}

	return true, 0
}

func NewAppService(config *Config) *AppService {
	return &AppService{
		config:      config,
		collections: make(map[string]*mongo.Collection),
		stopChan:    make(chan struct{}),
		router:      gin.New(),
	}
}

func (s *AppService) connectMongo(ctx context.Context) error {
	clientOptions := options.Client().
		ApplyURI(s.config.MongoURI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetRetryWrites(true).
		SetRetryReads(true).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(5 * time.Second)

	var err error
	s.mongoClient, err = mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err = s.mongoClient.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	db := s.mongoClient.Database(s.config.DatabaseName)
	s.collections["apiKeys"] = db.Collection(s.config.ApiKeysCollection)
	s.collections["logs"] = db.Collection(s.config.LogsCollection)
	s.collections["settings"] = db.Collection(s.config.SettingsCollection)
	s.collections["templates"] = db.Collection("keyTemplates")

	s.createIndexes(ctx)

	s.dbConnected.Store(true)
	s.log(LogLevelInfo, "system", "Connected to MongoDB", "")

	if err := s.loadAPIKeysToCache(); err != nil {
		s.log(LogLevelWarning, "system", "Failed to load API keys to cache", err.Error())
	}

	return nil
}

func (s *AppService) createIndexes(ctx context.Context) {
	s.collections["apiKeys"].Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "Expiration", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "Name", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "Created", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "Tags.name", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "Active", Value: 1}, {Key: "Expiration", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
	})

	s.collections["logs"].Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "timestamp", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "level", Value: 1}, {Key: "timestamp", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "component", Value: 1}, {Key: "timestamp", Value: -1}},
			Options: options.Index().SetBackground(true),
		},
	})

	s.collections["templates"].Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "name", Value: 1}},
			Options: options.Index().SetUnique(true).SetBackground(true),
		},
		{
			Keys:    bson.D{{Key: "isDefault", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
	})
}

func (s *AppService) ensureMongoConnection() bool {
	if s.dbConnected.Load() {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.connectMongo(ctx); err != nil {
		s.log(LogLevelError, "database", "Failed to reconnect to MongoDB", err.Error())
		return false
	}

	s.log(LogLevelInfo, "database", "Successfully reconnected to MongoDB", "")
	return true
}

func (s *AppService) log(level LogLevel, component, message, details string) {
	logEntry := SystemLog{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Component: component,
		Message:   message,
		Details:   details,
	}

	s.addToLogBuffer(logEntry)

	if s.dbConnected.Load() && s.collections["logs"] != nil {
		go s.persistLogToDB(logEntry)
	}
}

func (s *AppService) logBuffer() []SystemLog {
	bufferValue, _ := s.cache.LoadOrStore("logBuffer", make([]SystemLog, 0, s.config.LogBufferSize))
	buffer, _ := bufferValue.([]SystemLog)
	return buffer
}

func (s *AppService) addToLogBuffer(logEntry SystemLog) {
	buffer := s.logBuffer()

	buffer = append(buffer, logEntry)
	if len(buffer) > s.config.LogBufferSize {
		buffer = buffer[len(buffer)-s.config.LogBufferSize:]
	}

	s.cache.Store("logBuffer", buffer)
}

func (s *AppService) persistLogToDB(logEntry SystemLog) {
	if !s.dbConnected.Load() || s.collections["logs"] == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collections["logs"].InsertOne(ctx, logEntry)
	if err != nil {
		fmt.Printf("Error persisting log to database: %v\n", err)
	}
}

func (s *AppService) getLogsWithFilters(ctx context.Context, level, component string, limit int, since time.Time, search string) ([]SystemLog, error) {
	if !s.dbConnected.Load() || s.collections["logs"] == nil {
		return s.getLogsFromBuffer(level, component, limit, since, search), nil
	}

	filter := bson.M{}

	if level != "" && level != "all" {
		if level == "error" {
			filter["level"] = LogLevelError
		} else if level == "warning" {
			filter["level"] = bson.M{"$in": []LogLevel{LogLevelWarning, LogLevelError}}
		} else if level == "info" {
			filter["level"] = bson.M{"$in": []LogLevel{LogLevelInfo, LogLevelWarning, LogLevelError}}
		}
	}

	if component != "" && component != "all" {
		filter["component"] = component
	}

	if !since.IsZero() {
		filter["timestamp"] = bson.M{"$gt": since}
	}

	if search != "" {
		filter["$or"] = []bson.M{
			{"message": bson.M{"$regex": search, "$options": "i"}},
			{"details": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	findOptions := options.Find()
	findOptions.SetSort(bson.M{"timestamp": -1})
	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}

	cursor, err := s.collections["logs"].Find(ctx, filter, findOptions)
	if err != nil {
		return s.getLogsFromBuffer(level, component, limit, since, search), err
	}
	defer cursor.Close(ctx)

	var logs []SystemLog
	if err := cursor.All(ctx, &logs); err != nil {
		return s.getLogsFromBuffer(level, component, limit, since, search), err
	}

	return logs, nil
}

func (s *AppService) getLogsFromBuffer(level, component string, limit int, since time.Time, search string) []SystemLog {
	buffer := s.logBuffer()

	if limit <= 0 || limit > len(buffer) {
		limit = len(buffer)
	}

	filteredLogs := make([]SystemLog, 0, limit)
	for i := len(buffer) - 1; i >= 0; i-- {
		log := buffer[i]

		if level != "" && level != "all" {
			if level == "error" && log.Level != LogLevelError {
				continue
			} else if level == "warning" && log.Level != LogLevelWarning && log.Level != LogLevelError {
				continue
			} else if level == "info" && log.Level != LogLevelInfo && log.Level != LogLevelWarning && log.Level != LogLevelError {
				continue
			}
		}

		if component != "" && component != "all" && log.Component != component {
			continue
		}

		if !since.IsZero() && log.Timestamp.Before(since) {
			continue
		}

		if search != "" && !(containsIgnoreCase(log.Message, search) || containsIgnoreCase(log.Details, search)) {
			continue
		}

		filteredLogs = append(filteredLogs, log)
		if len(filteredLogs) >= limit {
			break
		}
	}

	return filteredLogs
}

func (s *AppService) getUserSettings(ctx context.Context, id string) (*UserSettings, error) {
	cacheKey := "settings_" + id
	if settingsVal, found := s.cache.Load(cacheKey); found {
		if settings, ok := settingsVal.(*UserSettings); ok {
			return settings, nil
		}
	}

	if !s.dbConnected.Load() || s.collections["settings"] == nil {
		return nil, errors.New("database not connected")
	}

	var settings UserSettings
	err := s.collections["settings"].FindOne(ctx, bson.M{"_id": id}).Decode(&settings)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			defaultSettings := &UserSettings{
				ID:              id,
				Data:            map[string]interface{}{},
				SettingsVersion: 1,
				LastUpdated:     time.Now().UTC(),
			}

			s.cache.Store(cacheKey, defaultSettings)
			return defaultSettings, nil
		}
		return nil, err
	}

	s.cache.Store(cacheKey, &settings)
	return &settings, nil
}

func (s *AppService) loadAPIKeysToCache() error {
	if !s.ensureMongoConnection() {
		return errors.New("database connection not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := options.Find().SetBatchSize(100)
	cursor, err := s.collections["apiKeys"].Find(ctx, bson.M{}, opts)
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

		s.cache.Store(apiKey.ID, &apiKey)
		s.initKeyServices(&apiKey)
		count++
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error: %w", err)
	}

	log.Printf("Loaded %d API keys into cache", count)
	return nil
}

func (s *AppService) startCleanupWorker() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		ticker := time.NewTicker(time.Duration(s.config.CleanupInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.cleanupExpiredKeys()
				s.flushRequestCounts()
			case <-s.stopChan:
				return
			}
		}
	}()
}

func (s *AppService) cleanupExpiredKeys() {
	if !s.ensureMongoConnection() {
		log.Println("Skipping cleanup, database connection not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	filter := bson.M{"Expiration": bson.M{"$lt": now}}

	cursor, err := s.collections["apiKeys"].Find(ctx, filter)
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

	_, err = s.collections["apiKeys"].DeleteMany(ctx, filter)
	if err != nil {
		log.Printf("Error deleting expired keys: %v", err)
		return
	}

	for _, key := range expiredKeys {
		s.removeKeyResources(key)
	}

	s.log(LogLevelInfo, "system", fmt.Sprintf("Cleaned up %d expired API keys", len(expiredKeys)), "")
}

func (s *AppService) flushRequestCounts() {
	if !s.ensureMongoConnection() {
		return
	}

	updates := make(map[string]int64)

	s.keyStates.Range(func(key, value interface{}) bool {
		keyID, ok := key.(string)
		if !ok {
			return true
		}

		state, ok := value.(*KeyState)
		if !ok {
			return true
		}

		if state.RequestCount > 0 && time.Since(state.LastUpdated) > time.Minute {
			updates[keyID] = state.RequestCount
			state.RequestCount = 0
			state.LastUpdated = time.Now()
			s.keyStates.Store(keyID, state)
		}

		return true
	})

	if len(updates) == 0 {
		return
	}

	var ops []mongo.WriteModel
	for id, count := range updates {
		ops = append(ops, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": id}).
			SetUpdate(bson.M{
				"$set": bson.M{"LastUsed": time.Now().UTC()},
				"$inc": bson.M{"RequestCount": count},
			}))

		if key, ok := s.cache.Load(id); ok {
			if apiKey, ok := key.(*APIKey); ok {
				apiKey.RequestCount += count
				apiKey.LastUsed = time.Now().UTC()
				s.cache.Store(id, apiKey)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := s.collections["apiKeys"].BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false)); err != nil {
		s.log(LogLevelError, "database", "Failed to flush request counts", err.Error())
	} else {
		s.log(LogLevelInfo, "system", fmt.Sprintf("Flushed request counts for %d API keys", len(updates)), "")
	}
}

func (s *AppService) updateKeyFields(key *APIKey, updates bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.collections["apiKeys"].UpdateOne(ctx, bson.M{"_id": key.ID}, bson.M{"$set": updates})
	if err != nil {
		return err
	}

	for k, v := range updates {
		switch k {
		case "Expiration":
			key.Expiration = v.(time.Time)
		case "RPM":
			key.RPM = v.(int)
			s.rateLimiters.Delete(key.ID)
			if key.RPM > 0 && key.Active {
				s.rateLimiters.Store(key.ID, NewRateLimiter(key.RPM))
			}
		case "ThreadsLimit":
			key.ThreadsLimit = v.(int)
			s.concurrencySlots.Delete(key.ID)
			if key.ThreadsLimit > 0 && key.Active {
				s.concurrencySlots.Store(key.ID, make(chan struct{}, key.ThreadsLimit))
			}
		case "Active":
			key.Active = v.(bool)
			if !key.Active {
				s.rateLimiters.Delete(key.ID)
				s.concurrencySlots.Delete(key.ID)
			}
		case "Name":
			key.Name = v.(string)
		case "TotalRequests":
			key.TotalRequests = v.(int64)
		case "Tags":
			key.Tags = v.([]Tag)
		}
	}

	s.cache.Store(key.ID, key)
	return nil
}

func (s *AppService) processBulk(ctx context.Context, keys []string, operation func(context.Context, string) error) (int, int, []string) {
	success, fails := 0, 0
	errors := []string{}

	for _, key := range keys {
		if err := operation(ctx, key); err != nil {
			fails++
			errors = append(errors, fmt.Sprintf("%s: %v", key, err))
		} else {
			success++
		}
	}

	return success, fails, errors
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

func containsIgnoreCase(s, substr string) bool {
	if s == "" || substr == "" {
		return false
	}

	s, substr = strings.ToLower(s), strings.ToLower(substr)
	return strings.Contains(s, substr)
}

func getStackTrace(skip int) string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	if n > 0 {
		stackLines := make([]byte, 0, n)
		count := 0
		skipLines := skip * 2
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				count++
				if count > skipLines {
					stackLines = append(stackLines, buf[i])
				}
			} else if count > skipLines {
				stackLines = append(stackLines, buf[i])
			}
		}
		return string(stackLines)
	}
	return ""
}

func (s *AppService) errorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}

				stack := getStackTrace(2)
				s.log(LogLevelError, "system", "Panic recovered", fmt.Sprintf("%v\n%s", err, stack))

				if !c.IsAborted() {
					respond(c, http.StatusInternalServerError, false, nil, "Internal server error")
				}
			}
		}()

		c.Next()
	}
}

func (s *AppService) corsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-lh-key"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func (s *AppService) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		isMonitoringEndpoint := strings.Contains(path, "/monitoring/logs") ||
			strings.Contains(path, "/monitoring/health")

		if !isMonitoringEndpoint {
			s.log(LogLevelDebug, "api-request",
				fmt.Sprintf("Request: %s %s from %s", c.Request.Method, path, c.ClientIP()), "")
		}

		c.Next()

		if !isMonitoringEndpoint {
			latency := time.Since(start)
			statusCode := c.Writer.Status()

			level := LogLevelInfo
			if statusCode >= 400 && statusCode < 500 {
				level = LogLevelWarning
			} else if statusCode >= 500 {
				level = LogLevelError
			}

			s.log(level, "api-response",
				fmt.Sprintf("Response: %d for %s %s - %v", statusCode, c.Request.Method, path, latency), "")
		}
	}
}

func (s *AppService) apiKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, status, err := s.fetchKey(c.GetHeader("x-lh-key"))
		if err != nil {
			c.AbortWithStatusJSON(status, ApiResponse{Success: false, Error: err.Error()})
			return
		}

		if !key.Active || time.Now().UTC().After(key.Expiration) {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				ApiResponse{Success: false, Error: "API key is inactive or expired"})
			return
		}

		c.Set("apiKey", key)
		c.Next()

		if state, ok := s.keyStates.Load(key.ID); ok {
			if keyState, ok := state.(*KeyState); ok {
				keyState.RequestCount++
				keyState.LastUpdated = time.Now()
				s.keyStates.Store(key.ID, keyState)
			}
		}
	}
}

func (s *AppService) templateOperation(c *gin.Context, operation func(*KeyTemplate) error) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	var template KeyTemplate
	if c.Request.Method != http.MethodGet {
		if err := c.ShouldBindJSON(&template); err != nil {
			respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid template format: %v", err))
			return
		}
	}

	if id := c.Param("id"); id != "" {
		template.ID = id
	}

	if err := operation(&template); err != nil {
		status := http.StatusInternalServerError
		if err == mongo.ErrNoDocuments {
			status = http.StatusNotFound
		}
		respond(c, status, false, nil, err.Error())
		return
	}

	respond(c, http.StatusOK, true, template, "")
}

func (s *AppService) validateAPIKey(c *gin.Context) {
	apiKey := c.GetHeader("x-lh-key")
	if apiKey == "" {
		respond(c, http.StatusUnauthorized, false, nil, "Missing API key header (x-lh-key)")
		return
	}

	keyInfo, status, err := s.fetchKey(apiKey)
	if err != nil {
		respond(c, status, false, nil, err.Error())
		return
	}

	if !keyInfo.Active {
		respond(c, http.StatusUnauthorized, false, nil, "API key is inactive")
		return
	}

	if time.Now().UTC().After(keyInfo.Expiration) {
		respond(c, http.StatusUnauthorized, false, nil, "API key has expired")
		return
	}

	if keyInfo.TotalRequests > 0 {
		stateValue, _ := s.keyStates.LoadOrStore(apiKey, &KeyState{
			RequestCount: 0,
			LastUpdated:  time.Now(),
		})

		state, ok := stateValue.(*KeyState)
		if !ok {
			respond(c, http.StatusInternalServerError, false, nil, "Invalid key state type")
			return
		}

		totalCount := keyInfo.RequestCount + state.RequestCount + 1
		if totalCount > keyInfo.TotalRequests {
			respond(c, http.StatusTooManyRequests, false, nil, "Total request limit exceeded",
				map[string]interface{}{
					"limit": keyInfo.TotalRequests,
					"used":  totalCount - 1,
				})
			return
		}

		state.RequestCount++
		state.LastUpdated = time.Now()
		s.keyStates.Store(apiKey, state)
	}

	if allowed, status := s.checkRateLimit(keyInfo); !allowed {
		respond(c, status, false, nil, "Rate limit exceeded",
			map[string]interface{}{
				"limit_rpm": keyInfo.RPM,
			})
		return
	}

	concurrencySemValue, exists := s.concurrencySlots.Load(apiKey)
	if exists {
		concurrencySem, ok := concurrencySemValue.(chan struct{})
		if ok {
			select {
			case concurrencySem <- struct{}{}:
				defer func() { <-concurrencySem }()
			default:
				respond(c, http.StatusTooManyRequests, false, nil, "Concurrency limit exceeded",
					map[string]interface{}{
						"limit_threads": keyInfo.ThreadsLimit,
					})
				return
			}
		}
	}

	now := time.Now()
	startTime := time.Now()

	go func() {
		if s.collections["logs"] != nil && s.dbConnected.Load() {
			log := ApiKeyLog{
				KeyID:     apiKey,
				Timestamp: now,
				Action:    "request",
				Path:      c.Request.URL.Path,
				Method:    c.Request.Method,
				IP:        c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
				Status:    c.Writer.Status(),
				Duration:  time.Since(startTime).Milliseconds(),
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			s.collections["logs"].InsertOne(ctx, log)
		}
	}()

	respond(c, http.StatusOK, true, map[string]interface{}{
		"status": "valid",
		"key_info": map[string]interface{}{
			"name":          keyInfo.Name,
			"expires_at":    keyInfo.Expiration.Format(time.RFC3339),
			"rpm":           keyInfo.RPM,
			"threads_limit": keyInfo.ThreadsLimit,
			"tags":          keyInfo.Tags,
		},
	}, "")
}

func (s *AppService) getLogs(c *gin.Context) {
	level := c.DefaultQuery("level", "")
	component := c.DefaultQuery("component", "")
	limitStr := c.DefaultQuery("limit", "100")
	sinceStr := c.DefaultQuery("since", "")
	search := c.DefaultQuery("search", "")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	var since time.Time
	if sinceStr != "" {
		since, _ = time.Parse(time.RFC3339, sinceStr)
	}

	logs, err := s.getLogsWithFilters(c, level, component, limit, since, search)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to retrieve logs: %v", err))
		return
	}

	respond(c, http.StatusOK, true, logs, "", map[string]interface{}{
		"count": len(logs),
		"limit": limit,
	})
}

func (s *AppService) clearLogs(c *gin.Context) {
	s.cache.Store("logBuffer", make([]SystemLog, 0, s.config.LogBufferSize))

	if s.dbConnected.Load() && s.collections["logs"] != nil {
		dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := s.collections["logs"].DeleteMany(dbCtx, bson.M{}); err != nil {
			respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to clear logs: %v", err))
			return
		}
	}

	s.log(LogLevelInfo, "system", "Logs cleared by user request", "")
	respond(c, http.StatusOK, true, map[string]string{"message": "All logs have been cleared"}, "")
}

func (s *AppService) getSettings(c *gin.Context) {
	id := c.DefaultQuery("id", "global")
	settings, err := s.getUserSettings(c, id)

	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to retrieve settings: %v", err))
		return
	}

	respond(c, http.StatusOK, true, map[string]interface{}{
		"id":              settings.ID,
		"data":            settings.Data,
		"settingsVersion": settings.SettingsVersion,
		"lastUpdated":     settings.LastUpdated,
	}, "")
}

func (s *AppService) saveSettings(c *gin.Context) {
	var request struct {
		ID              string      `json:"id"`
		Data            interface{} `json:"data"`
		SettingsVersion int         `json:"settingsVersion"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid request format: %v", err))
		return
	}

	if request.ID == "" {
		request.ID = "global"
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existingSettings, err := s.getUserSettings(dbCtx, request.ID)
	if err == nil && existingSettings != nil {
		if request.SettingsVersion > 0 && existingSettings.SettingsVersion > request.SettingsVersion {
			respond(c, http.StatusConflict, false, nil, fmt.Sprintf("Version conflict: server version %d is newer than client version %d",
				existingSettings.SettingsVersion, request.SettingsVersion))
			return
		}
	}

	now := time.Now().UTC()
	settings := &UserSettings{
		ID:              request.ID,
		Data:            request.Data,
		SettingsVersion: request.SettingsVersion + 1,
		LastUpdated:     now,
	}

	if s.dbConnected.Load() && s.collections["settings"] != nil {
		_, err := s.collections["settings"].ReplaceOne(
			dbCtx,
			bson.M{"_id": request.ID},
			settings,
			options.Replace().SetUpsert(true),
		)
		if err != nil {
			s.log(LogLevelError, "settings-manager", "Failed to save settings", err.Error())
			respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to save settings: %v", err))
			return
		}
	} else {
		s.log(LogLevelWarning, "settings-manager", "Database not connected, settings saved to cache only", "")
	}

	s.cache.Store("settings_"+request.ID, settings)
	s.log(LogLevelInfo, "settings-manager", fmt.Sprintf("Settings '%s' updated to version %d", settings.ID, settings.SettingsVersion), "")

	respond(c, http.StatusOK, true, map[string]interface{}{
		"id":              settings.ID,
		"data":            settings.Data,
		"settingsVersion": settings.SettingsVersion,
		"lastUpdated":     settings.LastUpdated,
	}, "")
}

func (s *AppService) deleteSettings(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respond(c, http.StatusBadRequest, false, nil, "Settings ID is required")
		return
	}

	s.cache.Delete("settings_" + id)

	if s.dbConnected.Load() && s.collections["settings"] != nil {
		dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := s.collections["settings"].DeleteOne(dbCtx, bson.M{"_id": id})
		if err != nil && err != mongo.ErrNoDocuments {
			respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to delete settings: %v", err))
			return
		}
	}

	s.log(LogLevelInfo, "settings-manager", fmt.Sprintf("Settings '%s' deleted", id), "")
	respond(c, http.StatusOK, true, map[string]string{"message": fmt.Sprintf("Settings '%s' have been deleted", id)}, "")
}

func (s *AppService) generateAPIKey(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	expiration := c.Query("expiration")
	if expiration == "" {
		respond(c, http.StatusBadRequest, false, nil, "Expiration parameter is required")
		return
	}

	rpm := parseQueryParam(c, "rpm", 0)
	threadsLimit := parseQueryParam(c, "tl", 0)
	totalRequests := parseQueryParamInt64(c, "limit", 0)
	customAPIKey := c.Query("apikey")
	name := c.Query("name")
	tagNames := c.QueryArray("tags")

	expirationDuration, err := parseExpiration(expiration)
	if err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid expiration format: %v", err))
		return
	}

	if customAPIKey != "" {
		if _, exists := s.cache.Load(customAPIKey); exists {
			respond(c, http.StatusConflict, false, nil, "Custom API key already exists")
			return
		}
	} else {
		for i := 0; i < 5; i++ {
			customAPIKey, err = generateRandomKey(32)
			if err != nil {
				respond(c, http.StatusInternalServerError, false, nil, "Failed to generate API key")
				return
			}
			if _, exists := s.cache.Load(customAPIKey); !exists {
				break
			}
			customAPIKey = ""
		}
		if customAPIKey == "" {
			respond(c, http.StatusInternalServerError, false, nil, "Failed to generate a unique API key after multiple attempts")
			return
		}
	}

	var tags []Tag
	if len(tagNames) > 0 {
		for _, tagName := range tagNames {
			tags = append(tags, Tag{
				Name:  tagName,
				Color: "",
			})
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
		Tags:          tags,
		RequestCount:  0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = s.collections["apiKeys"].ReplaceOne(ctx, bson.M{"_id": apiKey.ID}, apiKey, options.Replace().SetUpsert(true))
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to save API key: %v", err))
		return
	}

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Generated new API key: %s (name: %s)", apiKey.ID, name), "")
	s.cache.Store(apiKey.ID, apiKey)
	s.initKeyServices(apiKey)

	response := map[string]interface{}{
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

	if len(tags) > 0 {
		response["tags"] = tags
	}

	respond(c, http.StatusOK, true, response, "")
}

func (s *AppService) updateAPIKey(c *gin.Context) {
	apiKeyStr := c.Query("apikey")
	if apiKeyStr == "" {
		respond(c, http.StatusBadRequest, false, nil, "API key parameter is required")
		return
	}

	apiKey, status, err := s.fetchKey(apiKeyStr)
	if err != nil {
		respond(c, status, false, nil, err.Error())
		return
	}

	originalAPIKey := *apiKey
	updates := bson.M{}

	if newExpiration := c.Query("expiration"); newExpiration != "" {
		expirationDuration, err := parseExpiration(newExpiration)
		if err != nil {
			respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid expiration format: %v", err))
			return
		}

		now := time.Now().UTC()
		var newExp time.Time
		if apiKey.Expiration.Before(now) {
			newExp = now.Add(expirationDuration)
		} else {
			newExp = apiKey.Expiration.Add(expirationDuration)
		}
		updates["Expiration"] = newExp
	}

	if c.Query("rpm") != "" {
		updates["RPM"] = parseQueryParam(c, "rpm", 0)
	}

	if c.Query("tl") != "" {
		updates["ThreadsLimit"] = parseQueryParam(c, "tl", 0)
	}

	if c.Query("limit") != "" {
		updates["TotalRequests"] = parseQueryParamInt64(c, "limit", 0)
	}

	if newName := c.Query("name"); newName != "" {
		updates["Name"] = newName
	}

	if activeStr := c.Query("active"); activeStr != "" {
		updates["Active"] = activeStr == "true" || activeStr == "1"
	}

	addTags := c.QueryArray("addTags")
	removeTags := c.QueryArray("removeTags")
	if len(addTags) > 0 || len(removeTags) > 0 {
		tagMap := make(map[string]bool)
		for _, tag := range apiKey.Tags {
			tagMap[tag.Name] = true
		}

		for _, tagName := range addTags {
			if !tagMap[tagName] {
				apiKey.Tags = append(apiKey.Tags, Tag{Name: tagName})
				tagMap[tagName] = true
			}
		}

		if len(removeTags) > 0 {
			removeMap := make(map[string]bool)
			for _, tagName := range removeTags {
				removeMap[tagName] = true
			}

			newTags := []Tag{}
			for _, tag := range apiKey.Tags {
				if !removeMap[tag.Name] {
					newTags = append(newTags, tag)
				}
			}
			apiKey.Tags = newTags
		}

		updates["Tags"] = apiKey.Tags
	}

	if len(updates) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No updates specified")
		return
	}

	if err := s.updateKeyFields(apiKey, updates); err != nil {
		s.cache.Store(apiKey.ID, &originalAPIKey)
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to update API key: %v", err))
		return
	}

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

	response := map[string]interface{}{
		"message":    "API Key Updated Successfully",
		"key":        apiKey.ID,
		"expiration": apiKey.Expiration.Format(time.RFC3339),
		"rpm":        apiKey.RPM,
		"tl":         apiKey.ThreadsLimit,
		"limit":      apiKey.TotalRequests,
		"active":     apiKey.Active,
		"tags":       apiKey.Tags,
	}

	if apiKey.Name != "" {
		response["name"] = apiKey.Name
	}

	if len(changes) > 0 {
		response["changes"] = changes
	}

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Updated API key: %s with changes: %v", apiKey.ID, changes), "")
	respond(c, http.StatusOK, true, response, "")
}

func (s *AppService) getAPIKeyInfo(c *gin.Context) {
	apiKeyStr := c.Query("apikey")
	if apiKeyStr == "" {
		respond(c, http.StatusBadRequest, false, nil, "API key parameter is required")
		return
	}

	apiKey, status, err := s.fetchKey(apiKeyStr)
	if err != nil {
		respond(c, status, false, nil, err.Error())
		return
	}

	keyStateValue, exists := s.keyStates.Load(apiKeyStr)
	if exists {
		if keyState, ok := keyStateValue.(*KeyState); ok {
			apiKey.RequestCount += keyState.RequestCount
		}
	}

	now := time.Now().UTC()
	isExpired := now.After(apiKey.Expiration)
	isValid := apiKey.Active && !isExpired

	response := map[string]interface{}{
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
		"tags":          apiKey.Tags,
	}

	if apiKey.Name != "" {
		response["name"] = apiKey.Name
	}

	if !apiKey.LastUsed.IsZero() {
		response["last_used"] = apiKey.LastUsed.Format(time.RFC3339)
	}

	respond(c, http.StatusOK, true, response, "")
}

func (s *AppService) cleanExpiredAPIKeys(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	apiKeyStr := c.Query("apikey")
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if apiKeyStr != "" {
		_, status, err := s.fetchKey(apiKeyStr)
		if err != nil {
			respond(c, status, false, nil, err.Error())
			return
		}

		filter := bson.M{"_id": apiKeyStr}
		if forceStr := c.Query("force"); forceStr != "true" && forceStr != "1" {
			filter["Expiration"] = bson.M{"$lt": now}
		}

		res, err := s.collections["apiKeys"].DeleteOne(ctx, filter)
		if err != nil {
			respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to delete API key: %v", err))
			return
		}

		if res.DeletedCount > 0 {
			s.removeKeyResources(apiKeyStr)
			s.log(LogLevelInfo, "api-key", fmt.Sprintf("Deleted API key: %s", apiKeyStr), "")
			respond(c, http.StatusOK, true, map[string]string{"message": fmt.Sprintf("Deleted API key: %s", apiKeyStr)}, "")
		} else {
			respond(c, http.StatusOK, true, map[string]string{"message": "API key not expired or already deleted"}, "")
		}
		return
	}

	filter := bson.M{"Expiration": bson.M{"$lt": now}}
	cursor, err := s.collections["apiKeys"].Find(ctx, filter)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to find expired API keys: %v", err))
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
		respond(c, http.StatusOK, true, map[string]string{"message": "No expired API keys to clean"}, "")
		return
	}

	res, err := s.collections["apiKeys"].DeleteMany(ctx, filter)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to clean expired API keys: %v", err))
		return
	}

	for _, key := range expiredKeys {
		s.removeKeyResources(key)
	}

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Cleaned %d expired API key(s)", res.DeletedCount), "")
	respond(c, http.StatusOK, true, map[string]interface{}{
		"message":      fmt.Sprintf("Cleaned %d expired API key(s)", res.DeletedCount),
		"deleted_keys": expiredKeys,
	}, "")
}

func (s *AppService) listApiKeys(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	filter := bson.M{}

	if status := c.Query("status"); status != "" {
		switch status {
		case "active":
			filter["Active"] = true
			filter["Expiration"] = bson.M{"$gt": time.Now().UTC()}
		case "inactive":
			filter["Active"] = false
		case "expired":
			filter["Expiration"] = bson.M{"$lt": time.Now().UTC()}
		}
	}

	if search := c.Query("search"); search != "" {
		filter["$or"] = []bson.M{
			{"_id": bson.M{"$regex": search, "$options": "i"}},
			{"Name": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	if tag := c.Query("tag"); tag != "" {
		filter["Tags.name"] = tag
	}

	page := parseQueryParam(c, "page", 1)
	if page < 1 {
		page = 1
	}

	pageSize := parseQueryParam(c, "pageSize", 20)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	sortField := c.DefaultQuery("sortField", "Created")
	sortOrder := 1
	if c.DefaultQuery("sortOrder", "desc") == "desc" {
		sortOrder = -1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	totalKeys, err := s.collections["apiKeys"].CountDocuments(ctx, filter)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to count API keys: %v", err))
		return
	}

	totalPages := (int(totalKeys) + pageSize - 1) / pageSize

	opts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: sortOrder}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize))

	cursor, err := s.collections["apiKeys"].Find(ctx, filter, opts)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to retrieve API keys: %v", err))
		return
	}
	defer cursor.Close(ctx)

	var keys []APIKey
	if err := cursor.All(ctx, &keys); err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to decode API keys: %v", err))
		return
	}

	for _, key := range keys {
		s.cache.Store(key.ID, &key)
	}

	respond(c, http.StatusOK, true, PagedResponse{
		Data:       keys,
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalKeys,
		TotalPages: totalPages,
	}, "")
}

func (s *AppService) getApiKeyById(c *gin.Context) {
	keyID := c.Param("id")
	if keyID == "" {
		respond(c, http.StatusBadRequest, false, nil, "API key ID is required")
		return
	}

	apiKey, status, err := s.fetchKey(keyID)
	if err != nil {
		respond(c, status, false, nil, err.Error())
		return
	}

	respond(c, http.StatusOK, true, apiKey, "")
}

func (s *AppService) updateApiKeyById(c *gin.Context) {
	keyID := c.Param("id")
	if keyID == "" {
		respond(c, http.StatusBadRequest, false, nil, "API key ID is required")
		return
	}

	var updateRequest struct {
		Name          *string  `json:"name"`
		Expiration    *string  `json:"expiration"`
		RPM           *int     `json:"rpm"`
		ThreadsLimit  *int     `json:"threads_limit"`
		TotalRequests *int64   `json:"total_requests"`
		Active        *bool    `json:"active"`
		AddTags       []Tag    `json:"add_tags"`
		RemoveTags    []string `json:"remove_tags"`
	}

	if err := c.ShouldBindJSON(&updateRequest); err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	apiKey, status, err := s.fetchKey(keyID)
	if err != nil {
		respond(c, status, false, nil, err.Error())
		return
	}

	originalAPIKey := *apiKey
	updates := bson.M{}

	if updateRequest.Name != nil {
		updates["Name"] = *updateRequest.Name
	}

	if updateRequest.Expiration != nil {
		expirationDuration, err := parseExpiration(*updateRequest.Expiration)
		if err != nil {
			respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid expiration format: %v", err))
			return
		}

		now := time.Now().UTC()
		if apiKey.Expiration.Before(now) {
			updates["Expiration"] = now.Add(expirationDuration)
		} else {
			updates["Expiration"] = apiKey.Expiration.Add(expirationDuration)
		}
	}

	if updateRequest.RPM != nil {
		updates["RPM"] = *updateRequest.RPM
	}

	if updateRequest.ThreadsLimit != nil {
		updates["ThreadsLimit"] = *updateRequest.ThreadsLimit
	}

	if updateRequest.TotalRequests != nil {
		updates["TotalRequests"] = *updateRequest.TotalRequests
	}

	if updateRequest.Active != nil {
		updates["Active"] = *updateRequest.Active
	}

	if len(updateRequest.AddTags) > 0 || len(updateRequest.RemoveTags) > 0 {
		tagMap := make(map[string]bool)
		for _, tag := range apiKey.Tags {
			tagMap[tag.Name] = true
		}

		for _, tag := range updateRequest.AddTags {
			if !tagMap[tag.Name] {
				apiKey.Tags = append(apiKey.Tags, tag)
				tagMap[tag.Name] = true
			}
		}

		if len(updateRequest.RemoveTags) > 0 {
			removeMap := make(map[string]bool)
			for _, tagName := range updateRequest.RemoveTags {
				removeMap[tagName] = true
			}

			newTags := []Tag{}
			for _, tag := range apiKey.Tags {
				if !removeMap[tag.Name] {
					newTags = append(newTags, tag)
				}
			}
			apiKey.Tags = newTags
		}

		updates["Tags"] = apiKey.Tags
	}

	if len(updates) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No updates specified")
		return
	}

	if err := s.updateKeyFields(apiKey, updates); err != nil {
		s.cache.Store(apiKey.ID, &originalAPIKey)
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to update API key: %v", err))
		return
	}

	respond(c, http.StatusOK, true, apiKey, "")
}

func (s *AppService) deleteApiKeyById(c *gin.Context) {
	keyID := c.Param("id")
	if keyID == "" {
		respond(c, http.StatusBadRequest, false, nil, "API key ID is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := s.collections["apiKeys"].DeleteOne(ctx, bson.M{"_id": keyID})
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to delete API key: %v", err))
		return
	}

	if res.DeletedCount == 0 {
		respond(c, http.StatusNotFound, false, nil, "API key not found")
		return
	}

	s.removeKeyResources(keyID)
	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Deleted API key: %s", keyID), "")
	respond(c, http.StatusOK, true, map[string]string{"message": "API key deleted successfully"}, "")
}

func (s *AppService) bulkDeleteApiKeys(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	var request BulkOperationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if len(request.Keys) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No keys provided for deletion")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	success, fails, errors := s.processBulk(ctx, request.Keys, func(ctx context.Context, keyID string) error {
		_, err := s.collections["apiKeys"].DeleteOne(ctx, bson.M{"_id": keyID})
		if err != nil {
			return err
		}
		s.removeKeyResources(keyID)
		return nil
	})

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Bulk deleted %d API keys", success), "")

	respond(c, http.StatusOK, true, BulkOperationResponse{
		SuccessCount: success,
		ErrorCount:   fails,
		Errors:       errors,
	}, "")
}

func (s *AppService) bulkUpdateApiKeyTags(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	var request BulkTagRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if len(request.Keys) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No keys provided for tag update")
		return
	}

	if len(request.AddTags) == 0 && len(request.DelTags) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No tag operations specified")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	updateFields := bson.M{}

	if len(request.AddTags) > 0 {
		updateFields["$addToSet"] = bson.M{"Tags": bson.M{"$each": request.AddTags}}
	}

	if len(request.DelTags) > 0 {
		updateFields["$pull"] = bson.M{"Tags": bson.M{"name": bson.M{"$in": request.DelTags}}}
	}

	result, err := s.collections["apiKeys"].UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": request.Keys}},
		updateFields,
	)

	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to update API key tags: %v", err))
		return
	}

	for _, keyID := range request.Keys {
		s.cache.Delete(keyID)
	}

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Bulk updated tags for %d API keys", result.ModifiedCount), "")

	respond(c, http.StatusOK, true, BulkOperationResponse{
		SuccessCount: int(result.ModifiedCount),
		ErrorCount:   len(request.Keys) - int(result.ModifiedCount),
	}, "")
}

func (s *AppService) bulkExtendApiKeys(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	var request BulkExtendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	if len(request.Keys) == 0 {
		respond(c, http.StatusBadRequest, false, nil, "No keys provided for extension")
		return
	}

	if request.Extension == "" {
		respond(c, http.StatusBadRequest, false, nil, "Extension duration is required")
		return
	}

	extension, err := parseExpiration(request.Extension)
	if err != nil {
		respond(c, http.StatusBadRequest, false, nil, fmt.Sprintf("Invalid extension format: %v", err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	success, fails, errors := s.processBulk(ctx, request.Keys, func(ctx context.Context, keyID string) error {
		var apiKey APIKey
		if err := s.collections["apiKeys"].FindOne(ctx, bson.M{"_id": keyID}).Decode(&apiKey); err != nil {
			return err
		}

		newExpiration := apiKey.Expiration
		if apiKey.Expiration.Before(now) {
			newExpiration = now.Add(extension)
		} else {
			newExpiration = apiKey.Expiration.Add(extension)
		}

		_, err := s.collections["apiKeys"].UpdateOne(
			ctx,
			bson.M{"_id": keyID},
			bson.M{"$set": bson.M{"Expiration": newExpiration}},
		)

		if err != nil {
			return err
		}

		if keyValue, found := s.cache.Load(keyID); found {
			if key, ok := keyValue.(*APIKey); ok {
				key.Expiration = newExpiration
				s.cache.Store(keyID, key)
			}
		} else {
			s.cache.Delete(keyID)
		}

		return nil
	})

	s.log(LogLevelInfo, "api-key", fmt.Sprintf("Bulk extended %d API keys", success), "")

	respond(c, http.StatusOK, true, BulkOperationResponse{
		SuccessCount: success,
		ErrorCount:   fails,
		Errors:       errors,
	}, "")
}

func (s *AppService) createTemplate(c *gin.Context) {
	s.templateOperation(c, func(template *KeyTemplate) error {
		if template.Name == "" {
			return errors.New("template name is required")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if template.IsDefault {
			_, err := s.collections["templates"].UpdateMany(
				ctx,
				bson.M{"isDefault": true},
				bson.M{"$set": bson.M{"isDefault": false}},
			)
			if err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		template.Created = now
		template.LastModified = now

		result, err := s.collections["templates"].InsertOne(ctx, template)
		if err != nil {
			return err
		}

		template.ID = fmt.Sprintf("%v", result.InsertedID)
		return nil
	})
}

func (s *AppService) getTemplates(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := s.collections["templates"].Find(ctx, bson.M{}, opts)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to retrieve templates: %v", err))
		return
	}
	defer cursor.Close(ctx)

	var templates []KeyTemplate
	if err := cursor.All(ctx, &templates); err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to decode templates: %v", err))
		return
	}

	respond(c, http.StatusOK, true, templates, "")
}

func (s *AppService) getTemplateById(c *gin.Context) {
	s.templateOperation(c, func(template *KeyTemplate) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var result KeyTemplate
		err := s.collections["templates"].FindOne(ctx, bson.M{"_id": template.ID}).Decode(&result)
		if err != nil {
			return err
		}

		*template = result
		return nil
	})
}

func (s *AppService) updateTemplate(c *gin.Context) {
	s.templateOperation(c, func(template *KeyTemplate) error {
		template.LastModified = time.Now().UTC()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if template.IsDefault {
			_, err := s.collections["templates"].UpdateMany(
				ctx,
				bson.M{"_id": bson.M{"$ne": template.ID}, "isDefault": true},
				bson.M{"$set": bson.M{"isDefault": false}},
			)
			if err != nil {
				return err
			}
		}

		result, err := s.collections["templates"].ReplaceOne(ctx, bson.M{"_id": template.ID}, template)
		if err != nil {
			return err
		}

		if result.MatchedCount == 0 {
			return mongo.ErrNoDocuments
		}

		return nil
	})
}

func (s *AppService) deleteTemplate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respond(c, http.StatusBadRequest, false, nil, "Template ID is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := s.collections["templates"].DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to delete template: %v", err))
		return
	}

	if result.DeletedCount == 0 {
		respond(c, http.StatusNotFound, false, nil, "Template not found")
		return
	}

	respond(c, http.StatusOK, true, map[string]string{"message": "Template deleted successfully"}, "")
}

func (s *AppService) getDefaultTemplate(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var template KeyTemplate
	err := s.collections["templates"].FindOne(ctx, bson.M{"isDefault": true}).Decode(&template)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			respond(c, http.StatusOK, true, nil, "")
		} else {
			respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Database error: %v", err))
		}
		return
	}

	respond(c, http.StatusOK, true, template, "")
}

func (s *AppService) getMonitoringStats(c *gin.Context) {
	if !s.ensureMongoConnection() {
		respond(c, http.StatusServiceUnavailable, false, nil, "Database connection not available")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	stats := MonitoringStats{
		LastUpdated:       now,
		SystemStatus:      "healthy",
		DatabaseConnected: s.dbConnected.Load(),
	}

	totalKeys, err := s.collections["apiKeys"].CountDocuments(ctx, bson.M{})
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to count API keys: %v", err))
		return
	}
	stats.TotalKeys = int(totalKeys)

	activeKeysFilter := bson.M{
		"Active":     true,
		"Expiration": bson.M{"$gt": now},
	}
	activeKeys, err := s.collections["apiKeys"].CountDocuments(ctx, activeKeysFilter)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to count active API keys: %v", err))
		return
	}
	stats.ActiveKeys = int(activeKeys)

	expiringFilter := bson.M{
		"Active": true,
		"Expiration": bson.M{
			"$gt": now,
			"$lt": now.Add(7 * 24 * time.Hour),
		},
	}
	expiringKeys, err := s.collections["apiKeys"].CountDocuments(ctx, expiringFilter)
	if err != nil {
		respond(c, http.StatusInternalServerError, false, nil, fmt.Sprintf("Failed to count expiring API keys: %v", err))
		return
	}
	stats.ExpiringKeys = int(expiringKeys)

	oneDayAgo := now.Add(-24 * time.Hour)
	logsFilter := bson.M{
		"timestamp": bson.M{"$gt": oneDayAgo},
	}

	pipeline := []bson.M{
		{"$match": logsFilter},
		{"$group": bson.M{
			"_id":         nil,
			"count":       bson.M{"$sum": 1},
			"errorCount":  bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$gte": []interface{}{"$status", 400}}, 1, 0}}},
			"avgDuration": bson.M{"$avg": "$duration"},
		}},
	}

	cursor, err := s.collections["logs"].Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("Error aggregating logs: %v", err)
	} else {
		defer cursor.Close(ctx)

		var results []struct {
			Count       int64   `bson:"count"`
			ErrorCount  int64   `bson:"errorCount"`
			AvgDuration float64 `bson:"avgDuration"`
		}

		if err := cursor.All(ctx, &results); err != nil {
			log.Printf("Error decoding logs results: %v", err)
		} else if len(results) > 0 {
			stats.TotalRequests24h = results[0].Count
			if results[0].Count > 0 {
				stats.ErrorRate24h = float64(results[0].ErrorCount) / float64(results[0].Count)
			}
			stats.AvgResponseTime = int64(results[0].AvgDuration)
		}
	}

	respond(c, http.StatusOK, true, stats, "")
}

func (s *AppService) healthCheck(c *gin.Context) {
	respond(c, http.StatusOK, true, map[string]interface{}{
		"status":      "ok",
		"version":     "1.0",
		"dbConnected": s.dbConnected.Load(),
		"timestamp":   time.Now().UTC(),
	}, "")
}

func (s *AppService) setupRoutes() {
	s.router.Use(gin.Recovery(), s.corsMiddleware(), s.loggingMiddleware(), s.errorMiddleware())

	api := s.router.Group("/api/v1")
	{
		keys := api.Group("/keys")
		{
			keys.GET("", s.listApiKeys)
			keys.GET("/:id", s.getApiKeyById)
			keys.PUT("/:id", s.updateApiKeyById)
			keys.DELETE("/:id", s.deleteApiKeyById)
			keys.POST("/bulk-delete", s.bulkDeleteApiKeys)
			keys.POST("/bulk-tags", s.bulkUpdateApiKeyTags)
			keys.POST("/bulk-extend", s.bulkExtendApiKeys)
		}

		templates := api.Group("/templates")
		{
			templates.GET("", s.getTemplates)
			templates.GET("/default", s.getDefaultTemplate)
			templates.GET("/:id", s.getTemplateById)
			templates.POST("", s.createTemplate)
			templates.PUT("/:id", s.updateTemplate)
			templates.DELETE("/:id", s.deleteTemplate)
		}

		settings := api.Group("/settings")
		{
			settings.GET("", s.getSettings)
			settings.POST("", s.saveSettings)
			settings.DELETE("/:id", s.deleteSettings)
		}

		monitoring := api.Group("/monitoring")
		{
			monitoring.GET("/health", s.healthCheck)
			monitoring.GET("/logs", s.getLogs)
			monitoring.DELETE("/logs", s.clearLogs)
			monitoring.GET("/stats", s.getMonitoringStats)
		}

		api.GET("/generate-key", s.generateAPIKey)
		api.GET("/update-key", s.updateAPIKey)
		api.GET("/key-info", s.getAPIKeyInfo)
		api.GET("/clean-keys", s.cleanExpiredAPIKeys)
	}

	s.router.Any("/validate", s.validateAPIKey)
}

func (s *AppService) shutdown(server *http.Server) {
	log.Println("Shutting down API Key Manager...")

	close(s.stopChan)
	log.Println("Waiting for background tasks to complete...")
	s.wg.Wait()

	log.Println("Flushing request counts...")
	s.flushRequestCounts()

	if s.mongoClient != nil {
		log.Println("Disconnecting from MongoDB...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.mongoClient.Disconnect(ctx)
	}

	log.Println("Shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx)

	log.Println("Shutdown complete")
}

func loadConfig() (*Config, error) {
	defaultConfig := &Config{
		ServerPort:         "8080",
		MongoURI:           "mongodb://localhost:27017",
		DatabaseName:       "apiKeyManager",
		ApiKeysCollection:  "apiKeys",
		LogsCollection:     "systemLogs",
		SettingsCollection: "userSettings",
		LogRetention:       30,
		LogBufferSize:      1000,
		ReadTimeout:        30,
		WriteTimeout:       30,
		IdleTimeout:        60,
		CleanupInterval:    60,
	}

	file, err := os.Open("server.json")
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
	if envLogsCollection := os.Getenv("LOGS_COLLECTION"); envLogsCollection != "" {
		config.LogsCollection = envLogsCollection
	}

	return config, nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Starting API Key Manager...")

	gin.SetMode(gin.ReleaseMode)

	config, _ := loadConfig()
	app := NewAppService(config)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := app.connectMongo(ctx); err != nil {
		log.Printf("Warning: %v", err)
		log.Println("Will retry database connection when needed")
	}
	cancel()

	app.startCleanupWorker()
	app.setupRoutes()

	server := &http.Server{
		Addr:         ":" + app.config.ServerPort,
		Handler:      app.router,
		ReadTimeout:  time.Duration(app.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(app.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(app.config.IdleTimeout) * time.Second,
	}

	go func() {
		log.Printf("Server is running on port %s", app.config.ServerPort)
		app.log(LogLevelInfo, "system", fmt.Sprintf("Server started on port %s", app.config.ServerPort), "")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	app.shutdown(server)
}
