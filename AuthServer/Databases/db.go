package database


import (
	"context"
	"fmt"

  	"go.mongodb.org/mongo-driver/mongo"
  	"go.mongodb.org/mongo-driver/mongo/options"
)


func ConnectDB() *mongo.Client {
	DBURL := "mongodb+srv://mongodbapikeys:bdwo3iJXfuEQ2TKp@apikeys.uz0am8e.mongodb.net/?retryWrites=true&w=majority&appName=apiKeys" //Put Your Database Url Here 
	clientOptions := options.Client().ApplyURI(DBURL)// Connect to //MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		panic(err)
	}
	// Check the connection
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("🛢 Connected to MongoDB! 🛢")
	return client
}