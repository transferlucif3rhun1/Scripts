package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const (
	forwardURL = "http://45.92.1.127:8080/api/forward"
	apiKey     = "my-auth-key-1"
	port       = "3000"
)

type Payload struct {
	FollowRedirects             bool                `json:"followRedirects"`
	WithRandomTLSExtensionOrder bool                `json:"withRandomTLSExtensionOrder"`
	SessionID                   *string             `json:"sessionId,omitempty"`
	ProxyURL                    *string             `json:"proxyUrl,omitempty"`
	Headers                     map[string]string   `json:"headers,omitempty"`
	RequestURL                  string              `json:"requestUrl"`
	RequestMethod               string              `json:"requestMethod"`
	RequestBody                 string              `json:"requestBody"`
	RequestCookies              []map[string]string `json:"requestCookies,omitempty"`
	ForceHttp1                  bool                `json:"forceHttp1,omitempty"`
	HeaderOrder                 []string            `json:"headerOrder,omitempty"`
	TLSClientIdentifier         string              `json:"tlsClientIdentifier,omitempty"`
}

type ForwardResponse struct {
	ID           string              `json:"id"`
	Body         string              `json:"body"`
	Cookies      map[string]string   `json:"cookies"`
	Headers      map[string][]string `json:"headers"`
	Status       int                 `json:"status"`
	Target       string              `json:"target"`
	UsedProtocol string              `json:"usedProtocol"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	http.HandleFunc("/", proxyHandler)
	fmt.Printf("Proxy server running on http://localhost:%s\n", port)
	fmt.Printf("Forward requests to this server, and they will be proxied to %s\n", forwardURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	payload, err := formatPayload(r, bodyBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error formatting payload: %v", err), http.StatusBadRequest)
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshaling JSON: %v", err), http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest("POST", forwardURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating request: %v", err), http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error forwarding request: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading response: %v", err), http.StatusInternalServerError)
		return
	}

	var forwardResponse ForwardResponse
	if err := json.Unmarshal(respBody, &forwardResponse); err != nil {
		log.Printf("Error parsing response format: %v. Returning original response.", err)

		for key, values := range resp.Header {
			if !containsIgnoreCase([]string{"transfer-encoding", "connection", "content-length"}, key) {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
		}

		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	if contentTypes, ok := forwardResponse.Headers["Content-Type"]; ok && len(contentTypes) > 0 {
		w.Header().Set("Content-Type", contentTypes[0])
	} else if contentTypes, ok := forwardResponse.Headers["content-type"]; ok && len(contentTypes) > 0 {
		w.Header().Set("Content-Type", contentTypes[0])
	}

	for key, values := range forwardResponse.Headers {
		headerKey := key
		if !containsIgnoreCase([]string{"transfer-encoding", "connection", "content-length"}, headerKey) {
			for _, value := range values {
				w.Header().Add(headerKey, value)
			}
		}
	}

	for name, value := range forwardResponse.Cookies {
		http.SetCookie(w, &http.Cookie{
			Name:  name,
			Value: value,
			Path:  "/",
		})
	}

	var responseBody []byte
	var bodyObj interface{}
	if err := json.Unmarshal([]byte(forwardResponse.Body), &bodyObj); err == nil {
		responseBody = []byte(forwardResponse.Body)
	} else {
		body := forwardResponse.Body
		if len(body) > 2 && body[0] == '"' && body[len(body)-1] == '"' {
			body = body[1 : len(body)-1]
			body = strings.ReplaceAll(body, "\\\"", "\"")
			body = strings.ReplaceAll(body, "\\\\", "\\")
		}
		responseBody = []byte(body)
	}

	w.WriteHeader(forwardResponse.Status)
	w.Write(responseBody)
}

func formatPayload(r *http.Request, bodyBytes []byte) (*Payload, error) {
	payload := &Payload{
		FollowRedirects:             true,
		WithRandomTLSExtensionOrder: true,
		RequestMethod:               r.Method,
		RequestBody:                 "<body>",
		Headers:                     make(map[string]string),
	}

	payload.RequestURL = ""

	specialHeaders := []string{
		"x-sid", "x-proxy", "x-url", "x-api-key", "cookie",
		"x-version", "x-randh", "x-client",
	}

	var headerOrder []string
	var randomizeHeaders bool

	for key, values := range r.Header {
		if len(values) == 0 {
			continue
		}

		lowerKey := strings.ToLower(key)
		value := values[0]

		headerOrder = append(headerOrder, key)

		switch lowerKey {
		case "x-sid":
			payload.SessionID = &value
		case "x-proxy":
			if value != "" {
				proxyUrl := "http://" + value
				payload.ProxyURL = &proxyUrl
			}
		case "x-url":
			payload.RequestURL = value
		case "x-version":
			if value == "1" {
				payload.ForceHttp1 = true
			}
		case "x-randh":
			if strings.ToLower(value) == "true" {
				randomizeHeaders = true
			}
		case "x-client":
			payload.TLSClientIdentifier = value
		case "cookie":
		default:
			if !containsIgnoreCase(specialHeaders, lowerKey) {
				payload.Headers[key] = value
			}
		}
	}

	if len(headerOrder) > 0 {
		var finalHeaderOrder []string
		for _, header := range headerOrder {
			if !containsIgnoreCase(specialHeaders, header) {
				finalHeaderOrder = append(finalHeaderOrder, header)
			}
		}

		if randomizeHeaders && len(finalHeaderOrder) > 1 {
			rand.Shuffle(len(finalHeaderOrder), func(i, j int) {
				finalHeaderOrder[i], finalHeaderOrder[j] = finalHeaderOrder[j], finalHeaderOrder[i]
			})
		}

		payload.HeaderOrder = finalHeaderOrder
	}

	if payload.RequestURL == "" {
		return nil, errors.New("x-url header is required")
	}

	if len(payload.Headers) == 0 {
		payload.Headers = nil
	}

	if len(bodyBytes) > 0 {
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(strings.ToLower(contentType), "application/json") {
			var jsonObj interface{}
			if err := json.Unmarshal(bodyBytes, &jsonObj); err == nil {
				jsonStr, err := json.Marshal(jsonObj)
				if err == nil {
					payload.RequestBody = string(jsonStr)
				} else {
					payload.RequestBody = string(bodyBytes)
				}
			} else {
				payload.RequestBody = string(bodyBytes)
			}
		} else {
			payload.RequestBody = string(bodyBytes)
		}
	}

	cookies := r.Cookies()
	if len(cookies) > 0 {
		cookieList := make([]map[string]string, 0, len(cookies))
		for _, cookie := range cookies {
			cookieList = append(cookieList, map[string]string{
				"name":  cookie.Name,
				"value": cookie.Value,
			})
		}
		payload.RequestCookies = cookieList
	}

	return payload, nil
}

func containsIgnoreCase(slice []string, item string) bool {
	lowerItem := strings.ToLower(item)
	for _, s := range slice {
		if strings.ToLower(s) == lowerItem {
			return true
		}
	}
	return false
}
