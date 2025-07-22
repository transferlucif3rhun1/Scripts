// main.go
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/publicsuffix"
)

// RecaptchaParams holds parsed reCAPTCHA parameters
type RecaptchaParams struct {
	APIVersion string
	ParamsStr  string
}

// RecaptchaResponse represents the response containing rresp
type RecaptchaResponse struct {
	RResp string `json:"rresp,omitempty"`
	Error string `json:"error,omitempty"`
}

// Constants holds all the constant values used in the server
var Constants = struct {
	ReCAPTCHA struct {
		BaseURL          string
		PostDataTemplate string
		TokenPattern     *regexp.Regexp
		ResponsePattern  *regexp.Regexp
		URLPattern       *regexp.Regexp
	}
	HTTP struct {
		Headers map[string]string
	}
}{
	ReCAPTCHA: struct {
		BaseURL          string
		PostDataTemplate string
		TokenPattern     *regexp.Regexp
		ResponsePattern  *regexp.Regexp
		URLPattern       *regexp.Regexp
	}{
		BaseURL:          "https://www.google.com/recaptcha/",
		PostDataTemplate: "v=%s&reason=q&c=%s&k=%s&co=%s",
		TokenPattern:     regexp.MustCompile(`"recaptcha-token" value="(.*?)"`),
		ResponsePattern:  regexp.MustCompile(`"rresp","(.*?)"`),
		URLPattern:       regexp.MustCompile(`(api2|enterprise)/anchor\?(.*)`),
	},
	HTTP: struct {
		Headers map[string]string
	}{
		Headers: map[string]string{
			"Accept":               "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
			"Accept-Language":      "en-US,en;q=0.9",
			"Referer":              "https://www.google.com/recaptcha/",
			"Sec-CH-UA":            `"Chromium";v="130", "Brave";v="130", "Not?A_Brand";v="99"`,
			"Sec-CH-UA-Mobile":     "?0",
			"Sec-CH-UA-Platform":   `"Windows"`,
			"Sec-Fetch-Dest":       "image",
			"Sec-Fetch-Mode":       "no-cors",
			"Sec-Fetch-Site":       "same-origin",
			"Sec-GPC":              "1",
			"User-Agent":           "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
			"Content-Type":         "application/x-www-form-urlencoded",
		},
	},
}

// Logger for the server
var logger = log.New(os.Stdout, "[reCAPTCHA Server] ", log.LstdFlags)

// parseRecaptchaURL parses the anchor URL to extract API version and parameters string
func parseRecaptchaURL(anchorURL string) (*RecaptchaParams, error) {
	matches := Constants.ReCAPTCHA.URLPattern.FindStringSubmatch(anchorURL)
	if len(matches) < 3 {
		return nil, errors.New("invalid anchor URL format")
	}

	return &RecaptchaParams{
		APIVersion: matches[1],
		ParamsStr:  matches[2],
	}, nil
}

// parseQueryParams parses the query parameters string into a map
func parseQueryParams(paramsStr string) (map[string]string, error) {
	values, err := url.ParseQuery(paramsStr)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	for key, vals := range values {
		if len(vals) > 0 {
			params[key] = vals[0]
		}
	}
	return params, nil
}

// createHTTPClient creates an HTTP client with optional proxy settings and 60s timeout
func createHTTPClient(proxyStr string) (*http.Client, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second, // Set total timeout to 60 seconds
	}

	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy format: %v", err)
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2: true,
		}
		client.Transport = transport
	}

	return client, nil
}

// solveRecaptcha performs the reCAPTCHA solving process and returns the rresp
func solveRecaptcha(anchorURL string, client *http.Client) (string, error) {
	recaptchaParams, err := parseRecaptchaURL(anchorURL)
	if err != nil {
		return "", err
	}

	urlBase := Constants.ReCAPTCHA.BaseURL + recaptchaParams.APIVersion + "/"

	initialURL := urlBase + "anchor?" + recaptchaParams.ParamsStr
	req, err := http.NewRequest("GET", initialURL, nil)
	if err != nil {
		return "", err
	}

	for key, value := range Constants.HTTP.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to GET anchor URL: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read anchor response: %v", err)
	}
	bodyStr := string(bodyBytes)

	tokenMatch := Constants.ReCAPTCHA.TokenPattern.FindStringSubmatch(bodyStr)
	if len(tokenMatch) < 2 {
		return "", errors.New("could not find recaptcha token")
	}
	token := tokenMatch[1]

	queryParams, err := parseQueryParams(recaptchaParams.ParamsStr)
	if err != nil {
		return "", err
	}

	postData := fmt.Sprintf(
		Constants.ReCAPTCHA.PostDataTemplate,
		queryParams["v"],
		token,
		queryParams["k"],
		queryParams["co"],
	)

	reloadURL := fmt.Sprintf("%sreload?k=%s", urlBase, url.QueryEscape(queryParams["k"]))
	postReq, err := http.NewRequest("POST", reloadURL, bytes.NewBufferString(postData))
	if err != nil {
		return "", err
	}

	for key, value := range Constants.HTTP.Headers {
		postReq.Header.Set(key, value)
	}

	postResp, err := client.Do(postReq)
	if err != nil {
		return "", fmt.Errorf("failed to POST reload: %v", err)
	}
	defer postResp.Body.Close()

	reloadBodyBytes, err := io.ReadAll(postResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read reload response: %v", err)
	}
	reloadBodyStr := string(reloadBodyBytes)

	answerMatch := Constants.ReCAPTCHA.ResponsePattern.FindStringSubmatch(reloadBodyStr)
	if len(answerMatch) < 2 {
		return "", errors.New("could not find rresp token")
	}

	return answerMatch[1], nil
}

// solveRecaptchaHandler handles the /solveRecaptcha endpoint
func solveRecaptchaHandler(c *gin.Context) {
	var request struct {
		AnchorURL string `json:"anchor_url" binding:"required"`
		Proxy     string `json:"proxy,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Printf("Invalid request payload: %v", err)
		c.String(http.StatusBadRequest, "Invalid request payload")
		return
	}

	client, err := createHTTPClient(request.Proxy)
	if err != nil {
		logger.Printf("Failed to create HTTP client: %v", err)
		c.String(http.StatusBadRequest, fmt.Sprintf("Invalid proxy format: %v", err))
		return
	}

	rresp, err := solveRecaptcha(request.AnchorURL, client)
	if err != nil {
		logger.Printf("reCAPTCHA solving failed: %v", err)
		c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to solve reCAPTCHA: %v", err))
		return
	}

	if rresp == "" {
		logger.Printf("Empty rresp received")
		c.String(http.StatusInternalServerError, "Empty rresp received")
		return
	}

	c.String(http.StatusOK, rresp)
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	// Create a Gin router without the default logger and recovery middleware
	router := gin.New()

	// Add Recovery Middleware
	router.Use(gin.Recovery())

	// Define the /solveRecaptcha endpoint
	router.POST("/solveRecaptcha", solveRecaptchaHandler)

	// Define server address
	serverAddress := ":3083" // You can change the port as needed

	logger.Printf("Starting reCAPTCHA server on %s\n", serverAddress)
	if err := router.Run(serverAddress); err != nil {
		logger.Fatalf("Failed to run reCAPTCHA server: %v\n", err)
	}
}
