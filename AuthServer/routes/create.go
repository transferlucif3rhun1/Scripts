package routes

import (
	database "AuthServer/Databases"

	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	DB = database.ConnectDB()
	PostCollection = DB.Database("apikeys").Collection("apikeys")
)


type Output struct {
	Key      string        `json:"key"`
	Limit    int64         `json:"limit"`
	Expiration time.Duration `json:"expiry"`
	Usage    int64         `json:"usage"`
	Reset    int64     `json:"reset"`
}

type input struct{
    ExpirationQuery string `json:"expiration"`
	KeyLimit int64 `json:"keylimit"`
}


var DurationMap = map[string]time.Duration{
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
	"mo": 30 * 24 * time.Hour, 
	"y":  365 * 24 * time.Hour,
}

func parseDuration(input string) (time.Duration, error) {
	var number int
	var unit string

	// Attempt to parse the input
	_, err := fmt.Sscanf(input, "%d%s", &number, &unit)
	if err != nil {
		return 0, err
	}

	durationPerUnit, ok := DurationMap[unit]
	if !ok {
		return 0, fmt.Errorf("invalid time unit")
	}

	return time.Duration(number) * durationPerUnit, nil
}


func CreateKey(w http.ResponseWriter, r *http.Request) {
	var Limited int64
	errorMessage := "Failed To Generate ApiKey"
	errorresponse := map[string]interface{}{
		"message": errorMessage,
		"Data":    "",
		"error":   "true",
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse)
		return
	}
	i := &input{}
	err = json.Unmarshal(b, &i)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	apiKey := hex.EncodeToString(randomBytes)

	expirationDuration := time.Hour * 24 * 30 // Default: 1 month
	if i.ExpirationQuery != "" {
		duration, errz := parseDuration(i.ExpirationQuery)
		if errz == nil {
			expirationDuration = duration
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorresponse)
			return
		}
	}
	
	
	if i.KeyLimit != int64(0) {
		Limited = i.KeyLimit
			
	}else{
		Limited = 0
	}

	oures := Output{
		Key:        apiKey,
		Limit: 	 	Limited,
		Expiration: expirationDuration,
		Usage: 0,	
	}
		
	_, err = PostCollection.InsertOne(ctx, oures)
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse)
		return
	}
	jsonData, err := json.Marshal(oures)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse)
		return
	}
	response := map[string]interface{}{
		"message": "ApiKey Generated",
		"Data":    string(jsonData),
		"error":   "false",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
	
}
