package routes

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func DefaultHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the API key from the request headers.
	apiKeyHeader := r.Header.Get("x-lh-key")

	// Confirm an API key was provided.
	if apiKeyHeader == "" {
		http.Error(w, "Missing Api Key", http.StatusUnauthorized)
		return
	}

	targetURL, err := url.Parse("http://127.0.0.1:2023")
	if err != nil {
		http.Error(w, "Error parsing target URL", http.StatusUnauthorized)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: time.Second * 10,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Validate the API key.
	var apiKey Output

	defer cancel()

	err = PostCollection.FindOne(ctx, bson.M{"key": apiKeyHeader}).Decode(&apiKey)
	if err != nil {
		http.Error(w, "Invalid Api Key", http.StatusUnauthorized)
		return
	}

	// If the API key has a limit, check and update its usage and limit.
	if apiKey.Limit != -1 {
		// If the reset time has passed, reset the usage counter and update the reset time.
		if time.Now().After(time.Unix(0, apiKey.Reset)) {
			_, err := PostCollection.UpdateOne(ctx, bson.M{"key": apiKey.Key}, bson.M{"$set": bson.M{"usage": 0, "reset": time.Now().Add(time.Minute).UnixNano()}})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// If the usage exceeds the limit, return 429 error.
		if apiKey.Usage >= apiKey.Limit {
			http.Error(w, "Rate Limit Exceeded", http.StatusTooManyRequests)
			return
		}

		// Increment the usage counter.
		_, err := PostCollection.UpdateOne(ctx, bson.M{"key": apiKey.Key}, bson.M{"$inc": bson.M{"usage": 1}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	proxy.ServeHTTP(w, r)
}
