package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

// CookieHandler handles the conversion of JSON cookies to the required format
func CookieHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read request body", http.StatusBadRequest)
		return
	}

	// Parse the JSON body into a map
	var cookies map[string]string
	err = json.Unmarshal(body, &cookies)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert the map to the required format
	var cookieString strings.Builder
	for name, value := range cookies {
		cookieString.WriteString(fmt.Sprintf("%s=%s; ", name, value))
	}

	// Remove the last "; " if it exists
	result := strings.TrimSuffix(cookieString.String(), "; ")

	// Return the result as plain text
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, result)
}

func main() {
	http.HandleFunc("/convertCookies", CookieHandler)

	// Start the server on localhost:3099
	fmt.Println("Server is listening on http://localhost:3099/convertCookies")
	log.Fatal(http.ListenAndServe(":3099", nil))
}
