package routes
 
import (
	"context"
	"time"
	"net/http"
	"go.mongodb.org/mongo-driver/bson"
	"encoding/json"
	"io"
)


type inputu struct {
	ApiQuery string `json:"apikey"`
    ExpiryQuery string `json:"expiration,omitempty"`
	LimitKeyQuery int64 `json:"keylimit,omitempty"`
}

func UpdateKey(w http.ResponseWriter, r *http.Request) {
	errorMessage1 := "Failed To Update ApiKey"
	errorresponse1 := map[string]interface{}{
		"message": errorMessage1,
		"Data":    "",
		"error":   "true",
	}
	b1, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse1)
		return
	}
	i1 := &inputu{}
	err = json.Unmarshal(b1, &i1)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse1)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Second)
	
	
	defer cancel()

	expirationDuration := time.Hour * 24 * 30 // Default: 1 month
	if i1.ExpiryQuery != "" {
		duration, errz := parseDuration(i1.ExpiryQuery)
		if errz == nil {
			expirationDuration = duration
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorresponse1)
			return
		}
	}
	
	
	edited := bson.M{"expiration": expirationDuration, "limit": i1.LimitKeyQuery}
	
	result, err := PostCollection.UpdateOne(ctx, bson.M{"key": i1.ApiQuery}, bson.M{"$set": edited})
	
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse1)
		return
	}
	
	if result.MatchedCount < 1 {
		invalidmess := "ApiKey doesn't exist"
		invalresponse := map[string]interface{}{
			"message": invalidmess,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(invalresponse)
		return
	}

	oures1 := &Output{
		Key:        i1.ApiQuery,
		Expiration: expirationDuration,
		Limit: 	 	i1.LimitKeyQuery,
	}

	jsonData1, err := json.Marshal(oures1)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse1)
		return
	}
	response1 := map[string]interface{}{
		"message": "ApiKey Updated",
		"Data":    string(jsonData1),
		"error":   "false",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response1)
}