package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Config holds the configuration settings from server.json
type Config struct {
	ServerPort        string `json:"serverPort"`
	MongoURI          string `json:"mongoURI"`
	DatabaseName      string `json:"databaseName"`
	ApiKeysCollection string `json:"apiKeysCollection"`
	ReadTimeout       int    `json:"readTimeout"`
	WriteTimeout      int    `json:"writeTimeout"`
	IdleTimeout       int    `json:"idleTimeout"`
	LogLevel          string `json:"logLevel"`
}

// APIKey represents the structure of an API key document
type APIKey struct {
	ID                string    `bson:"_id,omitempty" json:"key"`
	Key               string    `bson:"key,omitempty" json:"key,omitempty"`
	Name              string    `bson:"Name,omitempty" json:"name,omitempty"`
	Expiration        time.Time `bson:"Expiration" json:"expiration"`
	RequestsPerMinute int       `bson:"RequestsPerMinute" json:"limit"`
}

// AppError represents a custom application error
type AppError struct {
	Message string
	Code    int
}

// Error implements the error interface
func (e *AppError) Error() string {
	return e.Message
}

// InitializeLogger sets up the logger with desired settings
func InitializeLogger(logLevel string) *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	level, err := logrus.ParseLevel(strings.ToLower(logLevel))
	if err != nil {
		logger.Warnf("Invalid log level '%s', defaulting to 'info'", logLevel)
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	return logger
}

// loadConfig reads and parses the configuration file
func loadConfig(filePath string, logger *logrus.Logger) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file '%s': %v", filePath, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := &Config{}
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config file '%s': %v", filePath, err)
	}

	// Validate essential fields
	if config.MongoURI == "" || config.DatabaseName == "" || config.ApiKeysCollection == "" {
		return nil, errors.New("config file must include 'mongoURI', 'databaseName', and 'apiKeysCollection'")
	}

	return config, nil
}

// backupCollection creates a backup of the specified collection
func backupCollection(client *mongo.Client, config *Config, backupFile string, logger *logrus.Logger) error {
	if backupFile == "" {
		logger.Info("No backup file specified. Skipping backup.")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := client.Database(config.DatabaseName).Collection(config.ApiKeysCollection)
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to find documents for backup: %v", err)
	}
	defer cursor.Close(ctx)

	file, err := os.Create(backupFile)
	if err != nil {
		return fmt.Errorf("failed to create backup file '%s': %v", backupFile, err)
	}
	defer file.Close()

	count := 0
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			logger.Errorf("Failed to decode document during backup: %v", err)
			continue
		}
		// Marshal the document to BSON
		docBytes, err := bson.Marshal(doc)
		if err != nil {
			logger.Errorf("Failed to marshal document during backup: %v", err)
			continue
		}
		// Write to file
		_, err = file.Write(docBytes)
		if err != nil {
			logger.Errorf("Failed to write document to backup file: %v", err)
			continue
		}
		count++
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error during backup: %v", err)
	}

	logger.Infof("Backup completed successfully. %d documents backed up to '%s'", count, backupFile)
	return nil
}

func main() {
	// Define the path to server.json
	configFilePath := "server.json"

	// Initialize a temporary logger for initial configuration loading
	tempLogger := logrus.New()
	tempLogger.SetOutput(os.Stdout)
	tempLogger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	tempLogger.SetLevel(logrus.InfoLevel) // Default level

	// Load configuration
	config, err := loadConfig(configFilePath, tempLogger)
	if err != nil {
		tempLogger.Fatalf("Configuration loading failed: %v", err)
	}

	// Initialize the main logger with the configured log level
	logger := InitializeLogger(config.LogLevel)
	logger.Info("Configuration loaded successfully.")

	// Connect to MongoDB with retries
	clientOptions := options.Client().
		ApplyURI(config.MongoURI).
		SetMaxPoolSize(100).
		SetMinPoolSize(10).
		SetRetryWrites(true)

	var client *mongo.Client
	retries := 5
	for i := 1; i <= retries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		client, err = mongo.Connect(ctx, clientOptions)
		cancel()

		if err != nil {
			logger.Errorf("MongoDB connection attempt %d/%d failed: %v", i, retries, err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Ping to verify connection
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		err = client.Ping(ctx, readpref.Primary())
		cancel()

		if err != nil {
			logger.Errorf("MongoDB ping attempt %d/%d failed: %v", i, retries, err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Successful connection
		logger.Info("Connected to MongoDB successfully.")
		break
	}

	if err != nil {
		logger.Fatalf("Failed to connect to MongoDB after %d attempts: %v", retries, err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Errorf("Error disconnecting from MongoDB: %v", err)
		} else {
			logger.Info("Disconnected from MongoDB.")
		}
	}()

	// Optional: Create a backup before migration
	backupFile := "backup_api_keys.bson" // You can change this path or make it configurable
	if err := backupCollection(client, config, backupFile, logger); err != nil {
		logger.Fatalf("Backup failed: %v", err)
	}

	collection := client.Database(config.DatabaseName).Collection(config.ApiKeysCollection)

	// Find all documents in the collection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		logger.Fatalf("Failed to retrieve documents: %v", err)
	}
	defer cursor.Close(ctx)

	totalProcessed := 0
	totalMigrated := 0
	totalSkipped := 0
	totalErrors := 0

	for cursor.Next(ctx) {
		var apiKey APIKey
		if err := cursor.Decode(&apiKey); err != nil {
			logger.Errorf("Failed to decode document: %v", err)
			totalErrors++
			continue
		}

		originalID := apiKey.ID
		apiKeyKey := apiKey.Key

		if apiKeyKey == "" {
			logger.Warnf("Document with _id '%s' has empty 'key' field. Skipping.", originalID)
			totalSkipped++
			continue
		}

		// Check if the document is already migrated
		if originalID == apiKeyKey {
			logger.Infof("Document with _id '%s' is already migrated (key is _id). Skipping.", originalID)
			totalSkipped++
			continue
		}

		// Attempt to insert the new document with _id set to key
		newDoc := bson.M{
			"_id":               apiKeyKey,
			"Name":              apiKey.Name,
			"Expiration":        apiKey.Expiration,
			"RequestsPerMinute": apiKey.RequestsPerMinute,
			// Add other fields if present
		}

		insertRetries := 3
		var insertErr error
		for j := 1; j <= insertRetries; j++ {
			_, insertErr = collection.InsertOne(context.Background(), newDoc)
			if insertErr != nil {
				if mongo.IsDuplicateKeyError(insertErr) {
					logger.Warnf("Duplicate _id '%s' encountered during insertion. Skipping.", apiKeyKey)
					break
				}
				logger.Errorf("Failed to insert new document with _id '%s' (Attempt %d/%d): %v", apiKeyKey, j, insertRetries, insertErr)
				time.Sleep(1 * time.Second)
				continue
			}
			// Successful insertion
			break
		}
		if insertErr != nil && !mongo.IsDuplicateKeyError(insertErr) {
			logger.Errorf("Exceeded retries for inserting document with _id '%s'. Skipping.", apiKeyKey)
			totalErrors++
			continue
		}

		// If insertion failed due to duplicate key, skip deletion
		if mongo.IsDuplicateKeyError(insertErr) {
			totalSkipped++
			continue
		}

		// Delete the old document with the original _id
		deleteRetries := 3
		var deleteErr error
		for j := 1; j <= deleteRetries; j++ {
			_, deleteErr = collection.DeleteOne(context.Background(), bson.M{"_id": originalID})
			if deleteErr != nil {
				logger.Errorf("Failed to delete old document with _id '%s' (Attempt %d/%d): %v", originalID, j, deleteRetries, deleteErr)
				time.Sleep(1 * time.Second)
				continue
			}
			// Successful deletion
			break
		}
		if deleteErr != nil {
			logger.Errorf("Exceeded retries for deleting old document with _id '%s'. Rolling back insertion.", originalID)
			// Attempt to delete the newly inserted document to maintain consistency
			_, rollbackErr := collection.DeleteOne(context.Background(), bson.M{"_id": apiKeyKey})
			if rollbackErr != nil {
				logger.Errorf("Failed to rollback inserted document with _id '%s': %v", apiKeyKey, rollbackErr)
			}
			totalErrors++
			continue
		}

		logger.Infof("Migrated API key '%s' from _id '%s'", apiKeyKey, originalID)
		totalMigrated++
		totalProcessed++

		// Log progress every 100 documents
		if totalProcessed%100 == 0 {
			logger.Infof("Progress: %d documents processed, %d migrated, %d skipped, %d errors", totalProcessed, totalMigrated, totalSkipped, totalErrors)
		}
	}

	if err := cursor.Err(); err != nil {
		logger.Fatalf("Cursor encountered error: %v", err)
	}

	// Final summary
	logger.Infof("Migration completed. Total processed: %d, Migrated: %d, Skipped: %d, Errors: %d", totalProcessed, totalMigrated, totalSkipped, totalErrors)
}
