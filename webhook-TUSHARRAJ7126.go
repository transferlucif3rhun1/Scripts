package main

import (
	"errors"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Message struct to represent a message document in MongoDB
type message struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	To        string             `bson:"to" json:"to"`
	From      string             `bson:"from" json:"from"`
	Msg       string             `bson:"message" json:"message"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// MongoDB initialization
func initMongoDB(uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}

	// Ping the database to verify the connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	log.Println("Connected to MongoDB")
	return client, nil
}

// Create necessary indexes for efficient querying
func createIndexes(collection *mongo.Collection) error {
	indexModel := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "to", Value: 1},
				{Key: "created_at", Value: -1}, // Efficient sorting by latest messages
			},
		},
		{
			Keys: bson.D{
				{Key: "to", Value: 1},
				{Key: "from", Value: 1},
				{Key: "message", Value: 1},
			},
			Options: options.Index().SetUnique(true), // Prevent duplicate messages
		},
	}

	_, err := collection.Indexes().CreateMany(context.Background(), indexModel)
	if err != nil {
		return err
	}

	return nil
}

// Validate message input
func validateMessageInput(to, from, message string) error {
	if len(to) == 0 || len(from) == 0 || len(message) == 0 {
		return errors.New("all parameters (to, from, message) must be provided")
	}

	if strings.TrimSpace(to) == "" || strings.TrimSpace(from) == "" || strings.TrimSpace(message) == "" {
		return errors.New("parameters 'to', 'from', and 'message' cannot be empty")
	}

	// Additional validation for lengths
	if len(to) > 20 || len(from) > 20 {
		return errors.New("'to' and 'from' should not exceed 20 characters")
	}
	if len(message) > 500 {
		return errors.New("message length should not exceed 500 characters")
	}

	return nil
}

// Insert a message into the MongoDB collection
func insertMessage(ctx context.Context, collection *mongo.Collection, msg message) error {
	// Add the created_at timestamp
	msg.CreatedAt = time.Now()

	// Insert the message into the collection
	_, err := collection.InsertOne(ctx, msg)
	return err
}

// Fetch and delete the latest message for a given 'to'
func fetchAndDeleteLatestMessage(ctx context.Context, collection *mongo.Collection, to string) (*message, error) {
	// Find the latest message for the recipient (sorted by created_at in descending order)
	filter := bson.M{"to": to}
	findOptions := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})

	var latestMsg message
	err := collection.FindOne(ctx, filter, findOptions).Decode(&latestMsg)
	if err == mongo.ErrNoDocuments {
		return nil, nil // No message found
	} else if err != nil {
		return nil, err // Some other error occurred
	}

	// Delete the latest message after fetching it
	_, err = collection.DeleteOne(ctx, bson.M{"_id": latestMsg.ID})
	if err != nil {
		return nil, err
	}

	return &latestMsg, nil
}

// Automatically purge messages older than 10 minutes
func purgeOldMessages(collection *mongo.Collection) {
	ctx := context.Background()

	// Calculate the timestamp for 10 minutes ago
	tenMinutesAgo := time.Now().Add(-10 * time.Minute)

	// Find and delete messages older than 10 minutes
	filter := bson.M{"created_at": bson.M{"$lt": tenMinutesAgo}}

	// Delete all old messages
	_, _ = collection.DeleteMany(ctx, filter)
}

// Start the purge scheduler every 10 minutes
func startPurgeScheduler(collection *mongo.Collection) {
	ticker := time.NewTicker(10 * time.Minute)

	go func() {
		for range ticker.C {
			purgeOldMessages(collection)
		}
	}()
}

func main() {
	// Set Gin to Release Mode
	gin.SetMode(gin.ReleaseMode)

	// MongoDB URI
	mongoURI := "mongodb://localhost:27017"
	client, err := initMongoDB(mongoURI)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB")
	}
	defer func() {
		if err = client.Disconnect(context.Background()); err != nil {
			log.Fatalf("Failed to disconnect from MongoDB")
		}
	}()

	// MongoDB collection
	collection := client.Database("webhookDB").Collection("messages")

	// Create necessary indexes
	if err := createIndexes(collection); err != nil {
		log.Fatalf("Failed to set up indexes")
	}

	// Start the automatic purge process every 10 minutes
	startPurgeScheduler(collection)

	// Initialize Gin router
	router := gin.New()
	router.Use(gin.Recovery())

	// Webhook route to accept messages
	router.GET("/webhook", func(c *gin.Context) {
		to := c.Query("to")
		from := c.Query("from")
		messageText := c.Query("message")

		// Validate inputs
		if err := validateMessageInput(to, from, messageText); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Create message object
		msg := message{
			To:    to,
			From:  from,
			Msg:   messageText,
		}

		// Insert the message into MongoDB
		_ = insertMessage(context.Background(), collection, msg)

		// Return success
		c.JSON(http.StatusOK, gin.H{"status": "message received"})
	})

	// Route to fetch and delete the latest message
	router.GET("/messages", func(c *gin.Context) {
		to := c.Query("to")
		if len(to) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'to' parameter"})
			return
		}

		// Fetch and delete the latest message for the given 'to'
		latestMsg, err := fetchAndDeleteLatestMessage(context.Background(), collection, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve message"})
			return
		}

		if latestMsg == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "No messages found"})
			return
		}

		// Return the latest message
		c.JSON(http.StatusOK, gin.H{
			"message": latestMsg,
		})
	})

	// Start the server
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		log.Printf("Server started on port %s", port)
		if err := router.Run(":" + port); err != nil {
			log.Fatalf("Failed to run server")
		}
	}()

	// Handle SIGINT and SIGTERM for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server")
}
