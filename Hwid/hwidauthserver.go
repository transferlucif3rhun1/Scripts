package main

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "regexp"
    "runtime"
    "strings"
    "sync"
    "time"

    "github.com/gin-gonic/gin"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

const (
    mongoURI       = "mongodb+srv://mongodbapikeys:mongodbapikeys@apikeysmanager.mmm8e.mongodb.net/?retryWrites=true&w=majority&appName=apiKeysManager"
    dbName         = "hwidData"
    userCollection = "userData"
)

// AuthData represents the structure to hold the name, HWID, and status
type AuthData struct {
    ID     string `json:"_id,omitempty" bson:"_id,omitempty"`
    Name   string `json:"name"`
    HWID   string `json:"hwid,omitempty"`
    Status string `json:"status"` // Possible values: "auth", "flagged", "whitelist"
}

// dbUpdateTask represents a task for updating the database
type dbUpdateTask struct {
    authData AuthData
}

// Manager handles all database and cache operations centrally
type Manager struct {
    client    *mongo.Client
    cache     map[string]AuthData
    nameIndex map[string]string // Maps Name to ID
    hwidIndex map[string]string // Maps HWID to ID
    mu        sync.RWMutex

    // Worker pool fields
    dbUpdateQueue     chan dbUpdateTask
    changeStreamQueue chan bson.M

    // Dynamic worker counts
    dbWorkerCount int
    csWorkerCount int
}

// AuthServer represents the server structure with RWMutex for thread-safe access
type AuthServer struct {
    manager     *Manager
    loggedHWIDs map[string]bool
    mu          sync.RWMutex
}

// NewManager initializes the MongoDB client and loads collections
func NewManager() *Manager {
    client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
    if err != nil {
        log.Fatalf("Failed to connect to MongoDB: %v", err)
    }

    cpuCount := runtime.NumCPU()
    dbWorkerCount := cpuCount * 2      // Adjust as needed
    csWorkerCount := cpuCount          // Adjust as needed

    mgr := &Manager{
        client:            client,
        cache:             make(map[string]AuthData),
        nameIndex:         make(map[string]string),
        hwidIndex:         make(map[string]string),
        dbUpdateQueue:     make(chan dbUpdateTask, 100),
        changeStreamQueue: make(chan bson.M, 100),
        dbWorkerCount:     dbWorkerCount,
        csWorkerCount:     csWorkerCount,
    }

    // Load initial cache and start watchers
    mgr.loadInitialCache()
    mgr.ensureIndexes()
    go mgr.startWatchers(context.Background())

    // Start worker pools
    mgr.startDBUpdateWorkers()
    mgr.startChangeStreamWorkers()

    return mgr
}

// startDBUpdateWorkers starts the worker pool for database updates
func (m *Manager) startDBUpdateWorkers() {
    for i := 0; i < m.dbWorkerCount; i++ {
        go m.processDBUpdates(i)
    }
}

// processDBUpdates processes database update tasks asynchronously
func (m *Manager) processDBUpdates(workerID int) {
    for task := range m.dbUpdateQueue {
        err := m.saveToMongoDB(task.authData)
        if err != nil {
            logError("Worker %d: Failed to save data to MongoDB: %v", workerID, err)
        } else {
            logInfo("Worker %d: Successfully saved data for %s", workerID, task.authData.ID)
        }
    }
}

// startChangeStreamWorkers starts the worker pool for change stream events
func (m *Manager) startChangeStreamWorkers() {
    for i := 0; i < m.csWorkerCount; i++ {
        go m.processChangeStreamEvents(i)
    }
}

// processChangeStreamEvents processes change stream events
func (m *Manager) processChangeStreamEvents(workerID int) {
    for event := range m.changeStreamQueue {
        m.handleEvent(event)
    }
}

// loadInitialCache loads data from MongoDB into memory
func (m *Manager) loadInitialCache() {
    logInfo("Starting initial cache load...")

    data := m.loadCollection()

    m.mu.Lock()
    m.cache = data
    m.mu.Unlock()

    logInfo("Cache loaded.")
}

// loadCollection retrieves all documents from MongoDB and loads them into the cache
func (m *Manager) loadCollection() map[string]AuthData {
    collection := m.client.Database(dbName).Collection(userCollection)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    cursor, err := collection.Find(ctx, bson.D{})
    if err != nil {
        log.Fatalf("Failed to fetch collection %s: %v", userCollection, err)
    }
    defer cursor.Close(ctx)

    data := make(map[string]AuthData)
    for cursor.Next(ctx) {
        var auth AuthData
        if err := cursor.Decode(&auth); err != nil {
            logWarn("Error decoding data: %v", err)
            continue
        }

        if auth.ID == "" {
            id, err := generateHexID()
            if err != nil {
                logError("Error generating ID: %v", err)
                continue
            }
            auth.ID = id
            m.dbUpdateQueue <- dbUpdateTask{authData: auth} // Enqueue the update
        }

        if auth.Name == "" || auth.HWID == "" || auth.ID == "" {
            logWarn("Removing entry with empty fields: %v", auth)
            if err := m.removeFromCollection(auth.ID); err != nil {
                logError("Error removing from collection: %v", err)
            }
            continue
        }

        data[auth.ID] = auth

        // Lock mutex before modifying shared resources
        m.mu.Lock()
        m.nameIndex[auth.Name] = auth.ID
        m.hwidIndex[auth.HWID] = auth.ID
        m.mu.Unlock()
    }
    return data
}

// ensureIndexes creates indexes on key fields
func (m *Manager) ensureIndexes() {
    collection := m.client.Database(dbName).Collection(userCollection)
    indexModels := []mongo.IndexModel{
        {
            Keys:    bson.D{{Key: "name", Value: 1}},
            Options: options.Index().SetUnique(true),
        },
        {
            Keys: bson.D{{Key: "hwid", Value: 1}},
        },
        {
            Keys: bson.D{{Key: "status", Value: 1}},
        },
    }
    _, err := collection.Indexes().CreateMany(context.Background(), indexModels)
    if err != nil {
        logError("Failed to create indexes: %v", err)
    }
}

// startWatchers starts the change stream watchers
func (m *Manager) startWatchers(ctx context.Context) {
    go m.watchChanges(ctx)
}

// watchChanges listens for changes on the collection and updates the cache accordingly
func (m *Manager) watchChanges(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            logInfo("watchChanges context cancelled")
            return
        default:
            m.startChangeStream(ctx)
        }
    }
}

// startChangeStream starts the MongoDB change stream
func (m *Manager) startChangeStream(ctx context.Context) {
    collection := m.client.Database(dbName).Collection(userCollection)
    options := options.ChangeStream().SetFullDocument(options.UpdateLookup)
    pipeline := mongo.Pipeline{
        bson.D{{Key: "$match", Value: bson.D{{Key: "operationType", Value: bson.M{"$in": bson.A{"insert", "update", "replace", "delete"}}}}}},
    }
    stream, err := collection.Watch(ctx, pipeline, options)
    if err != nil {
        logError("Failed to watch changes: %v", err)
        time.Sleep(2 * time.Second)
        return
    }
    defer stream.Close(ctx)

    for stream.Next(ctx) {
        var event bson.M
        if err := stream.Decode(&event); err != nil {
            logError("Error decoding change event: %v", err)
            continue
        }

        // Enqueue event for processing
        m.changeStreamQueue <- event
    }

    if err := stream.Err(); err != nil {
        logError("Change stream error: %v", err)
    }

    // Sleep before retrying to prevent tight loop in case of persistent errors
    time.Sleep(2 * time.Second)
}

// handleEvent processes change events and updates the cache
func (m *Manager) handleEvent(event bson.M) {
    m.mu.Lock()
    defer m.mu.Unlock()

    operationType, _ := event["operationType"].(string)
    switch operationType {
    case "insert", "update", "replace":
        fullDocument, ok := event["fullDocument"].(bson.M)
        if !ok {
            logWarn("Missing fullDocument in event: %v", event)
            return
        }
        authData := m.parseAuthData(fullDocument)
        if authData != nil {
            m.cache[authData.ID] = *authData
            m.nameIndex[authData.Name] = authData.ID
            m.hwidIndex[authData.HWID] = authData.ID
            logInfo("Updated cache for %s", authData.ID)
        }
    case "delete":
        docKey, ok := event["documentKey"].(bson.M)
        if !ok {
            logWarn("Missing documentKey in delete event: %v", event)
            return
        }
        idField, ok := docKey["_id"]
        if !ok {
            logWarn("Missing _id in documentKey: %v", docKey)
            return
        }
        var id string
        switch idValue := idField.(type) {
        case string:
            id = idValue
        case primitive.ObjectID:
            id = idValue.Hex()
        default:
            logWarn("Unsupported _id type: %T", idField)
            return
        }
        authData, exists := m.cache[id]
        if exists {
            delete(m.cache, id)
            delete(m.nameIndex, authData.Name)
            delete(m.hwidIndex, authData.HWID)
            logInfo("Deleted entry %s from cache", id)
        }
    }
}

// parseAuthData converts a bson.M document to an AuthData object
func (m *Manager) parseAuthData(doc bson.M) *AuthData {
    var authData AuthData
    name, nameOK := doc["name"].(string)
    hwid, hwidOK := doc["hwid"].(string)
    status, statusOK := doc["status"].(string)
    idField, idOK := doc["_id"]

    var id string
    if !idOK {
        logWarn("Missing _id in document: %v", doc)
        return nil
    }

    switch idValue := idField.(type) {
    case string:
        id = idValue
    case primitive.ObjectID:
        id = idValue.Hex()
    default:
        logWarn("Unsupported _id type: %T", idField)
        return nil
    }

    if !nameOK || !hwidOK || !statusOK {
        logWarn("Missing fields in document: %v", doc)
        return nil
    }

    authData.ID = id
    authData.Name = name
    authData.HWID = hwid
    authData.Status = status
    return &authData
}

// saveToMongoDB saves an AuthData document to MongoDB
func (m *Manager) saveToMongoDB(data AuthData) error {
    collection := m.client.Database(dbName).Collection(userCollection)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    filter := bson.M{"_id": data.ID}
    update := bson.M{"$set": bson.M{
        "name":   data.Name,
        "hwid":   data.HWID,
        "status": data.Status,
    }}
    opts := options.Update().SetUpsert(true)

    _, err := collection.UpdateOne(ctx, filter, update, opts)
    if err != nil {
        logError("Failed to save data to MongoDB: %v", err)
        return err
    }
    logInfo("Saved data for %s to MongoDB", data.ID)
    return nil
}

// removeFromCollection removes a document by _id from MongoDB
func (m *Manager) removeFromCollection(id string) error {
    // Lock mutex before modifying shared resources
    m.mu.Lock()
    defer m.mu.Unlock()

    collection := m.client.Database(dbName).Collection(userCollection)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    _, err := collection.DeleteOne(ctx, bson.M{"_id": id})
    if err != nil {
        logError("Failed to remove user %s: %v", id, err)
        return err
    }

    // Remove from cache and indices
    authData, exists := m.cache[id]
    if exists {
        delete(m.cache, id)
        delete(m.nameIndex, authData.Name)
        delete(m.hwidIndex, authData.HWID)
        logInfo("Removed user %s from collection and cache", id)
    }

    return nil
}

// generateHexID generates a 24-character hex string for use as the _id
func generateHexID() (string, error) {
    bytes := make([]byte, 12)
    _, err := rand.Read(bytes)
    if err != nil {
        return "", fmt.Errorf("failed to generate hex ID: %v", err)
    }
    return hex.EncodeToString(bytes), nil
}

// NewAuthServer creates a new AuthServer instance
func NewAuthServer(manager *Manager) *AuthServer {
    return &AuthServer{
        manager:     manager,
        loggedHWIDs: make(map[string]bool),
    }
}

// authHandler handles requests to the /auth endpoint
func (s *AuthServer) authHandler(c *gin.Context) {
    nameRaw := c.Query("name")
    hwidRaw := c.Query("hwid")
    whitelist := c.Query("whitelist") == "true"

    name, err := url.QueryUnescape(nameRaw)
    hwid, err2 := url.QueryUnescape(hwidRaw)

    if err != nil || err2 != nil || name == "" || hwid == "" {
        c.JSON(http.StatusBadRequest, ErrorResponse{Code: 400, Message: "Invalid request"})
        return
    }

    sanitizedName := sanitizeInput(name)
    sanitizedHWID := sanitizeInput(hwid)

    hwidResult, status, err := s.generateHWID(sanitizedName, sanitizedHWID, whitelist)
    if err != nil {
        if strings.Contains(err.Error(), "flagged") {
            c.JSON(http.StatusForbidden, ErrorResponse{Code: 403, Message: err.Error()})
        } else {
            c.JSON(http.StatusConflict, ErrorResponse{Code: 409, Message: err.Error()})
        }
        return
    }

    c.JSON(http.StatusOK, gin.H{"status": status, "hwid": hwidResult})
}

// generateHWID handles the registration and authentication logic
func (s *AuthServer) generateHWID(name, hwid string, whitelist bool) (string, string, error) {
    m := s.manager

    s.mu.RLock()
    _, hwidLogged := s.loggedHWIDs[hwid]
    s.mu.RUnlock()

    m.mu.RLock()
    nameID, nameExists := m.nameIndex[name]
    m.mu.RUnlock()

    if whitelist {
        // Whitelist the user; no HWID limitations
        return s.handleWhitelist(name, hwid)
    }

    if nameExists {
        m.mu.RLock()
        authData := m.cache[nameID]
        m.mu.RUnlock()
        switch authData.Status {
        case "flagged":
            logWarn("Flagged user detected: %s. Access denied.", name)
            return "", "Access denied: user is flagged", errors.New("access denied: user is flagged")
        case "whitelist":
            // Whitelisted users can authenticate with any HWID
            if !hwidLogged {
                logInfo("Whitelisted user authenticated: %s.", name)
                s.mu.Lock()
                s.loggedHWIDs[hwid] = true
                s.mu.Unlock()
            }
            return hwid, "User authenticated (whitelist)", nil
        case "auth":
            if authData.HWID != hwid {
                // HWID does not match, flag the user
                m.flagMultipleHWIDs(name)
                return "", "Multiple HWID detected and flagged", errors.New("multiple HWIDs detected for same name")
            }
            // HWID matches, authenticate the user
            if !hwidLogged {
                logInfo("User authenticated: %s.", name)
                s.mu.Lock()
                s.loggedHWIDs[hwid] = true
                s.mu.Unlock()
            }
            return hwid, "User authenticated", nil
        default:
            logWarn("Unknown status for user %s: %s", name, authData.Status)
            return "", "Access denied: unknown status", errors.New("access denied: unknown status")
        }
    }

    // Name does not exist; register new user
    newID, err := generateHexID()
    if err != nil {
        logError("Error generating new ID: %v", err)
        return "", "", err
    }
    authData := AuthData{ID: newID, Name: name, HWID: hwid, Status: "auth"}

    m.mu.Lock()
    // Update cache
    m.cache[newID] = authData
    m.nameIndex[name] = newID
    m.hwidIndex[hwid] = newID
    m.mu.Unlock()

    // Enqueue the database update
    m.dbUpdateQueue <- dbUpdateTask{authData: authData}

    logInfo("New user registered: %s with HWID: %s", name, hwid)
    return hwid, "User registered", nil
}

// handleWhitelist handles the whitelisting of a user
func (s *AuthServer) handleWhitelist(name, hwid string) (string, string, error) {
    m := s.manager

    m.mu.Lock()
    defer m.mu.Unlock()

    id, exists := m.nameIndex[name]
    if !exists {
        // Name does not exist; create new whitelisted user
        newID, err := generateHexID()
        if err != nil {
            logError("Error generating new ID: %v", err)
            return "", "", err
        }
        authData := AuthData{ID: newID, Name: name, HWID: hwid, Status: "whitelist"}

        // Update cache
        m.cache[newID] = authData
        m.nameIndex[name] = newID
        m.hwidIndex[hwid] = newID

        // Enqueue the database update
        m.dbUpdateQueue <- dbUpdateTask{authData: authData}

        logInfo("New whitelisted user registered: %s with HWID: %s", name, hwid)
        return hwid, "User whitelisted and registered", nil
    }

    // Update existing user to be whitelisted
    authData := m.cache[id]
    authData.Status = "whitelist"
    authData.HWID = hwid // Update HWID to the new one

    // Update cache
    m.cache[id] = authData
    m.hwidIndex[hwid] = id

    // Enqueue the database update
    m.dbUpdateQueue <- dbUpdateTask{authData: authData}

    logInfo("User %s updated to whitelisted status", name)
    return hwid, "User updated to whitelisted status", nil
}

// updateUserStatus updates the status of a user and enqueues the database update
func (m *Manager) updateUserStatus(id string, status string, hwid string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    authData, exists := m.cache[id]
    if !exists {
        logWarn("User ID %s not found for status update", id)
        return
    }

    authData.Status = status
    if hwid != "" {
        authData.HWID = hwid
        m.hwidIndex[hwid] = id
    }

    m.cache[id] = authData
    m.dbUpdateQueue <- dbUpdateTask{authData: authData}
}

// flagMultipleHWIDs flags the user in the database and cache if multiple HWIDs are detected
func (m *Manager) flagMultipleHWIDs(name string) {
    id, exists := m.nameIndex[name]
    if !exists {
        logWarn("Attempted to flag non-existent user %s", name)
        return
    }
    m.updateUserStatus(id, "flagged", "")
    logInfo("Flagged user %s due to multiple HWID detection", name)
}

// flagHandler handles requests to the /flag endpoint
func (s *AuthServer) flagHandler(c *gin.Context) {
    nameRaw := c.Query("name")
    name, err := url.QueryUnescape(nameRaw)
    if err != nil || name == "" {
        c.JSON(http.StatusBadRequest, ErrorResponse{Code: 400, Message: "Invalid name"})
        return
    }

    m := s.manager
    m.flagMultipleHWIDs(name)
    c.JSON(http.StatusOK, gin.H{"status": "flagged"})
}

// healthHandler provides counts of flagged, authenticated, and whitelisted HWIDs
func (s *AuthServer) healthHandler(c *gin.Context) {
    m := s.manager

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    collection := m.client.Database(dbName).Collection(userCollection)

    flaggedCount, _ := collection.CountDocuments(ctx, bson.M{"status": "flagged"})
    authCount, _ := collection.CountDocuments(ctx, bson.M{"status": "auth"})
    whitelistCount, _ := collection.CountDocuments(ctx, bson.M{"status": "whitelist"})

    healthData := gin.H{
        "status":          "healthy",
        "flagged_count":   flaggedCount,
        "auth_count":      authCount,
        "whitelist_count": whitelistCount,
    }

    c.JSON(http.StatusOK, healthData)
}

// sanitizeInput sanitizes the input to replace spaces and special characters with underscores
func sanitizeInput(input string) string {
    allowedPattern := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
    return allowedPattern.ReplaceAllString(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(input, " ", "_"), "-", "_"), "(", ""), "_")
}

// ErrorResponse represents a structured error response
type ErrorResponse struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// Logging functions with levels
func logInfo(format string, v ...interface{}) {
    log.Printf("[INFO] "+format, v...)
}

func logWarn(format string, v ...interface{}) {
    log.Printf("[WARN] "+format, v...)
}

func logError(format string, v ...interface{}) {
    log.Printf("[ERROR] "+format, v...)
}

// main function to start the server
func main() {
    manager := NewManager()
    router := gin.Default()
    server := NewAuthServer(manager)

    router.GET("/auth", server.authHandler)
    router.GET("/flag", server.flagHandler)
    router.GET("/health", server.healthHandler)
    // No /register endpoint since registration is handled in /auth

    srv := &http.Server{
        Addr:    "0.0.0.0:8030",
        Handler: router,
    }

    go func() {
        logInfo("Auth server is running...")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logError("Failed to start server: %v", err)
        }
    }()

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)
    <-quit
    logInfo("Shutting down server...")

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        logError("Server forced to shutdown: %v", err)
    }

    // Close the queues to signal workers to exit
    close(manager.dbUpdateQueue)
    close(manager.changeStreamQueue)

    // Wait for a short period to allow workers to finish
    time.Sleep(2 * time.Second) // Adjust as needed

    if err := manager.client.Disconnect(ctx); err != nil {
        logError("Error disconnecting MongoDB client: %v", err)
    }

    logInfo("Server exiting")
}
