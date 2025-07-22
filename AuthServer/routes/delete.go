package routes
 
import (

	"context"
	"time"
	"net/http"
	"go.mongodb.org/mongo-driver/bson"
	"encoding/json"
	"io"

)
 
type inputdel struct {
	ApiKeyQuery string `json:"apikey"`
}


func DeleteKey(w http.ResponseWriter, r *http.Request) {
	errorMessage2 := "Failed To Delete ApiKey"
	errorresponse2 := map[string]interface{}{
		"message": errorMessage2,
		"Data":    "",
		"error":   "true",
	}
	b2, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse2)
		return
	}
	i2 := &inputdel{}
	err = json.Unmarshal(b2, &i2)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse2)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()
	result, err := PostCollection.DeleteOne(ctx, bson.M{"key": i2.ApiKeyQuery}) 

	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorresponse2)
		return
	}
	
	if result.DeletedCount < 1 {
		invalidmess1 := "ApiKey doesn't exist"
		invalresponse1 := map[string]interface{}{
			"message": invalidmess1,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(invalresponse1)
		return
	}

	response2 := map[string]interface{}{
		"message": "ApiKey deleted successfully",
		"error":   "false",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response2)
}

// func DeletePost(c *gin.Context) {
// 	b2, err := io.ReadAll(c.Request.Body)
// 	if err != nil {
// 		c.AbortWithStatusJSON(500, gin.H{"message": "Failed To Delete ApiKey", "Data": "", "error": "true"})
// 		return
// 	}
// 	i2 := &inputdel{}
// 	err = json.Unmarshal(b2, &i2)
// 	if err != nil {
// 		c.AbortWithStatusJSON(500, gin.H{"message": "Failed To Delete ApiKey", "Data": "", "error": "true"})
// 		return
// 	}
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

// 	defer cancel()
// 	result, err := PostCollection.DeleteOne(ctx, bson.M{"key": i2.ApiKeyQuery}) 

	
// 	if err != nil {
// 		c.AbortWithStatusJSON(500, gin.H{"message": "Failed To Delete ApiKey", "Data": "", "error": "true"})
// 		return
// 	}
	
// 	if result.DeletedCount < 1 {
// 		c.JSON(401, gin.H{"message": "ApiKey doesn't exist"})
// 		return
// 	}
	
// 	c.JSON(200, gin.H{"message": "ApiKey deleted successfully", "error": "false"})
// }