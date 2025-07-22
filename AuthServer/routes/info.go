package routes
 
import (
	
	"context"
	"time"
	"net/http"
	"go.mongodb.org/mongo-driver/bson"
	"encoding/json"
	"io"
)

type inputread struct {
	ApiQueryString string `json:"apikey"`
}

func ReadKey(w http.ResponseWriter, r *http.Request) {
	errorMessage3 := "Failed To GET ApiKey Info"
	errorresponse3 := map[string]interface{}{
		"message": errorMessage3,
		"Data":    "",
		"error":   "true",
	}
	b3, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse3)
		return
	}
	i3 := &inputread{}
	err = json.Unmarshal(b3, &i3)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse3)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	
	var result Output
	
	defer cancel()
	
	err = PostCollection.FindOne(ctx, bson.M{"key": i3.ApiQueryString}).Decode(&result)
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse3)
		return
	}
	expirationTime := time.Now().Add(result.Expiration)
	if time.Now().After(expirationTime) {
		invalidmess8 := "ApiKey Expired!"
		invalresponse8 := map[string]interface{}{
			"message": invalidmess8,
			"error": "true",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(invalresponse8)
		return
	}
	responseww := map[string]interface{}{
		"message": "success!",
		"Data":    result,
		"error":   "false",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(responseww)

}