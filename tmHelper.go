package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/publicsuffix"
)

// Config holds application configuration
var config = struct {
	LogEnabled bool
	Logger     *log.Logger
}{
	LogEnabled: false,
	Logger:     log.New(os.Stdout, "[CAPTCHA] ", log.LstdFlags),
}

// logf logs a message if logging is enabled

// RecaptchaResponse represents the response from the reCAPTCHA API
type RecaptchaResponse struct {
	RResp string `json:"rresp,omitempty"`
	Error string `json:"error,omitempty"`
}

// CheckResponse represents the response from the check handler
type CheckResponse struct {
	Status string `json:"status"`
}

// CookiePair represents a name-value cookie pair
type CookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var Constants = struct {
	ReCAPTCHA struct {
		BaseURL          string
		AnchorURL        string
		PostDataTemplate string
		TokenPattern     *regexp.Regexp
		ResponsePattern  *regexp.Regexp
		URLPattern       *regexp.Regexp
	}
	Ticketmaster struct {
		SiteKey     string
		MaxAttempts int
	}
	HTTP struct {
		HeadersTicketmaster map[string]string
		HeadersReCaptcha    map[string]string
	}
}{
	ReCAPTCHA: struct {
		BaseURL          string
		AnchorURL        string
		PostDataTemplate string
		TokenPattern     *regexp.Regexp
		ResponsePattern  *regexp.Regexp
		URLPattern       *regexp.Regexp
	}{
		BaseURL:          "https://www.google.com/recaptcha/",
		AnchorURL:        "https://www.google.com/recaptcha/enterprise/anchor?ar=1&k=6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1&co=aHR0cHM6Ly9hdXRoLnRpY2tldG1hc3Rlci5jb206NDQz&hl=fr&v=lqsTZ5beIbCkK4uGEGv9JmUR&size=invisible&cb=c8csckoko34z",
		PostDataTemplate: "v=%s&reason=q&c=%s&k=%s&co=%s",
		TokenPattern:     regexp.MustCompile(`"recaptcha-token" value="(.*?)"`),
		ResponsePattern:  regexp.MustCompile(`"rresp","(.*?)"`),
		URLPattern:       regexp.MustCompile(`(api2|enterprise)/anchor\?(.*)`),
	},
	Ticketmaster: struct {
		SiteKey     string
		MaxAttempts int
	}{
		SiteKey:     "6LdWxZEkAAAAAIHtgtxW_lIfRHlcLWzZMMiwx9E1",
		MaxAttempts: 3,
	},
	HTTP: struct {
		HeadersTicketmaster map[string]string
		HeadersReCaptcha    map[string]string
	}{
		HeadersTicketmaster: map[string]string{
			"sec-ch-ua-platform": "Windows",
			"user-agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"sec-ch-ua":          "\"Brave\";v=\"131\", \"Chromium\";v=\"131\", \"Not_A Brand\";v=\"24\"",
			"dnt":                "1",
			"sec-ch-ua-mobile":   "?0",
			"accept":             "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
			"sec-gpc":            "1",
			"accept-language":    "en-GB,en;q=0.7",
			"sec-fetch-site":     "same-origin",
			"sec-fetch-mode":     "no-cors",
			"sec-fetch-dest":     "image",
			"referer":            "https://auth.ticketmaster.com/",
			"accept-encoding":    "gzip, deflate, br, zstd",
		},
		HeadersReCaptcha: map[string]string{
			"Accept":             "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
			"Accept-Language":    "en-US,en;q=0.9",
			"Referer":            "https://www.google.com/recaptcha/",
			"Sec-CH-UA":          `"Chromium";v="130", "Brave";v="130", "Not?A_Brand";v="99"`,
			"Sec-CH-UA-Mobile":   "?0",
			"Sec-CH-UA-Platform": `"Windows"`,
			"Sec-Fetch-Dest":     "image",
			"Sec-Fetch-Mode":     "no-cors",
			"Sec-Fetch-Site":     "same-origin",
			"Sec-GPC":            "1",
			"User-Agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
			"Content-Type":       "application/x-www-form-urlencoded",
		},
	},
}

var blockedWords = []string{
	"\"block",
	"\"captcha",
	"failed to do request",
	"failed to parse the response",
	"get your identity verified",
	"response pending",
	"correlationid",
	"correlationId",
}

// logf logs a message if logging is enabled
func logf(format string, v ...interface{}) {
	if config.LogEnabled {
		config.Logger.Printf(format, v...)
	}
}

func HandleCheckError(c *gin.Context, err error, statusCode int, message string) {
	logf("Error in CheckHandler: %v, Status: %d, Message: %s", err, statusCode, message)
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
	c.Abort()
}

func CheckHandler(c *gin.Context) {
	logf("CheckHandler: Processing request")
	if c.Request.Method != http.MethodPost {
		HandleCheckError(c, errors.New("invalid request method"), http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		HandleCheckError(c, err, http.StatusBadRequest, "Failed to read request body")
		return
	}

	requestBody := string(bodyBytes)
	if strings.TrimSpace(requestBody) == "" {
		logf("CheckHandler: Empty request body")
		c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
		return
	}

	ls := `body":`
	rs := `,"`
	startIdx := strings.Index(requestBody, ls)
	var textToCheck string
	if startIdx != -1 {
		startIdx += len(ls)
		endIdx := strings.Index(requestBody[startIdx:], rs)
		if endIdx != -1 {
			endIdx += startIdx
			bodyValue := requestBody[startIdx:endIdx]
			bodyValue = strings.TrimSpace(bodyValue)
			if strings.HasPrefix(bodyValue, `"`) && strings.HasSuffix(bodyValue, `"`) {
				bodyValue = bodyValue[1 : len(bodyValue)-1]
			}
			if bodyValue != "" {
				textToCheck = bodyValue
			}
		}
	}

	if textToCheck == "" {
		textToCheck = requestBody
	}

	logf("CheckHandler: Checking text for blocked words")

	lowerText := strings.ToLower(textToCheck)
	for _, word := range blockedWords {
		lowerWord := strings.ToLower(word)
		if strings.Contains(lowerText, lowerWord) {
			logf("CheckHandler: Blocked word found: %s", word)
			c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
			return
		}
	}

	logf("CheckHandler: No blocked words found, proceeding")
	c.Data(http.StatusOK, "application/json", bodyBytes)
}

func ConvertCookiesHandler(c *gin.Context) {
	logf("ConvertCookiesHandler: Processing request")
	if c.Request.Method != http.MethodPost {
		logf("ConvertCookiesHandler: Invalid method: %s", c.Request.Method)
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	if c.Request.ContentLength == 0 {
		logf("ConvertCookiesHandler: Empty request")
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logf("ConvertCookiesHandler: Failed to read body: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	requestBody := string(bodyBytes)
	if strings.TrimSpace(requestBody) == "" {
		logf("ConvertCookiesHandler: Empty request body")
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	var cookieMap map[string]string
	if err := json.Unmarshal(bodyBytes, &cookieMap); err != nil {
		logf("ConvertCookiesHandler: Failed to parse JSON: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	var cookies []CookiePair
	for name, value := range cookieMap {
		if value != "" {
			cookies = append(cookies, CookiePair{
				Name:  name,
				Value: value,
			})
		}
	}

	convertedCookies, err := json.Marshal(cookies)
	if err != nil {
		logf("ConvertCookiesHandler: Failed to marshal cookies: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	logf("ConvertCookiesHandler: Successfully converted %d cookies", len(cookies))
	c.Data(http.StatusOK, "application/json", convertedCookies)
}

func parseRecaptchaURL(anchorURL string) (string, string, error) {
	logf("parseRecaptchaURL: Parsing URL: %s", anchorURL)
	matches := Constants.ReCAPTCHA.URLPattern.FindStringSubmatch(anchorURL)
	if len(matches) < 3 {
		return "", "", errors.New("invalid anchor URL format")
	}
	logf("parseRecaptchaURL: Found API version: %s", matches[1])
	return matches[1], matches[2], nil
}

func parseQueryParams(paramsStr string) (map[string]string, error) {
	logf("parseQueryParams: Parsing parameters")
	values, err := url.ParseQuery(paramsStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query params: %w", err)
	}

	params := make(map[string]string)
	for key, vals := range values {
		if len(vals) > 0 {
			params[key] = vals[0]
		}
	}

	logf("parseQueryParams: Extracted %d parameters", len(params))
	return params, nil
}

func createHTTPClient(proxyStr string, timeout time.Duration) (*http.Client, error) {
	logf("createHTTPClient: Creating client with timeout: %v", timeout)

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false, // Enable compression
		ResponseHeaderTimeout: 10 * time.Second,

		// TCP level optimizations
		DisableKeepAlives: false,
	}

	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy format: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
		logf("createHTTPClient: Using proxy: %s", proxyStr)
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Jar:       jar,
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Copy the headers from the original request
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			for key, values := range via[0].Header {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			return nil
		},
	}

	return client, nil
}

func solveRecaptcha(ctx context.Context, anchorURL string, client *http.Client) (string, error) {
	logf("solveRecaptcha: Starting for URL: %s", anchorURL)

	// Create a context with timeout if not already done
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	apiVersion, paramsStr, err := parseRecaptchaURL(anchorURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse reCAPTCHA URL: %w", err)
	}

	urlBase := Constants.ReCAPTCHA.BaseURL + apiVersion + "/"
	initialURL := urlBase + "anchor?" + paramsStr

	req, err := http.NewRequestWithContext(ctx, "GET", initialURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create anchor request: %w", err)
	}

	for key, value := range Constants.HTTP.HeadersReCaptcha {
		req.Header.Set(key, value)
	}

	logf("solveRecaptcha: Sending request to anchor URL")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to GET anchor URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anchor request returned non-OK status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read anchor response: %w", err)
	}

	bodyStr := string(bodyBytes)
	tokenMatch := Constants.ReCAPTCHA.TokenPattern.FindStringSubmatch(bodyStr)
	if len(tokenMatch) < 2 {
		return "", errors.New("could not find recaptcha token")
	}

	token := tokenMatch[1]
	logf("solveRecaptcha: Found token (length: %d)", len(token))

	// Check if context has been canceled
	if ctx.Err() != nil {
		return "", fmt.Errorf("operation canceled: %w", ctx.Err())
	}

	queryParams, err := parseQueryParams(paramsStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse query parameters: %w", err)
	}

	postData := fmt.Sprintf(
		Constants.ReCAPTCHA.PostDataTemplate,
		queryParams["v"],
		token,
		queryParams["k"],
		queryParams["co"],
	)

	reloadURL := fmt.Sprintf("%sreload?k=%s", urlBase, url.QueryEscape(queryParams["k"]))
	postReq, err := http.NewRequestWithContext(ctx, "POST", reloadURL, bytes.NewBufferString(postData))
	if err != nil {
		return "", fmt.Errorf("failed to create reload request: %w", err)
	}

	for key, value := range Constants.HTTP.HeadersReCaptcha {
		postReq.Header.Set(key, value)
	}

	logf("solveRecaptcha: Sending POST request to reload URL")
	postResp, err := client.Do(postReq)
	if err != nil {
		return "", fmt.Errorf("failed to POST reload: %w", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reload request returned non-OK status: %d", postResp.StatusCode)
	}

	reloadBodyBytes, err := io.ReadAll(postResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read reload response: %w", err)
	}

	reloadBodyStr := string(reloadBodyBytes)
	answerMatch := Constants.ReCAPTCHA.ResponsePattern.FindStringSubmatch(reloadBodyStr)
	if len(answerMatch) < 2 {
		return "", errors.New("could not find rresp token")
	}

	logf("solveRecaptcha: Successfully solved reCAPTCHA")
	return answerMatch[1], nil
}

// EPSData holds the data extracted from the EPS manager endpoint
type EPSData struct {
	GecToken string
	EpsSID   string
}

// extractEPSData makes a single request to extract both gecToken and eps_sid
func extractEPSData(ctx context.Context, client *http.Client) (*EPSData, error) {
	logf("extractEPSData: Extracting both gecToken and eps_sid in a single request")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://epsf.ticketmaster.com/eps-mgr", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to eps-mgr: %w", err)
	}

	for key, value := range Constants.HTTP.HeadersTicketmaster {
		req.Header.Set(key, value)
	}

	logf("extractEPSData: Sending request to eps-mgr")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to GET eps-mgr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eps-mgr request returned non-OK status: %d", resp.StatusCode)
	}

	// Extract eps_sid cookie
	var epsSID string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "eps_sid" && cookie.Value != "" {
			epsSID = cookie.Value
			break
		}
	}

	if epsSID == "" {
		return nil, errors.New("eps_sid cookie not found or empty")
	}

	// Extract gecToken from response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read eps-mgr response: %w", err)
	}

	bodyStr := string(bodyBytes)
	ls := "var epsfToken = '"
	rs := "'"
	startIdx := strings.Index(bodyStr, ls)
	if startIdx == -1 {
		return nil, errors.New("gecToken start delimiter not found")
	}

	startIdx += len(ls)
	endIdx := strings.Index(bodyStr[startIdx:], rs)
	if endIdx == -1 {
		return nil, errors.New("gecToken end delimiter not found")
	}

	endIdx += startIdx
	gecToken := bodyStr[startIdx:endIdx]
	if strings.TrimSpace(gecToken) == "" {
		return nil, errors.New("empty gecToken extracted")
	}

	result := &EPSData{
		GecToken: gecToken,
		EpsSID:   epsSID,
	}

	logf("extractEPSData: Successfully extracted gecToken (length: %d) and eps_sid", len(gecToken))
	return result, nil
}

func solveTicketmasterCaptcha(ctx context.Context, client *http.Client, epsData *EPSData) (*http.Cookie, error) {
	logf("solveTicketmasterCaptcha: Starting captcha solving")

	recaptchaToken, err := solveRecaptcha(ctx, Constants.ReCAPTCHA.AnchorURL, client)
	if err != nil {
		return nil, fmt.Errorf("recaptcha solving failed: %w", err)
	}

	// Check if context has been canceled
	if ctx.Err() != nil {
		return nil, fmt.Errorf("operation canceled: %w", ctx.Err())
	}

	authURL := fmt.Sprintf("https://auth.ticketmaster.com/epsf/gec/v2/%s/auth.ticketmaster.com/%s/LoginPage/%s",
		epsData.GecToken,
		Constants.Ticketmaster.SiteKey,
		recaptchaToken,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth request: %w", err)
	}

	for key, value := range Constants.HTTP.HeadersTicketmaster {
		req.Header.Set(key, value)
	}

	logf("solveTicketmasterCaptcha: Sending request to auth URL")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ticketmaster request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth request returned non-OK status: %d", resp.StatusCode)
	}

	var tmptCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "tmpt" && cookie.Value != "" {
			tmptCookie = cookie
			break
		}
	}

	if tmptCookie == nil {
		return nil, errors.New("tmpt cookie not found or empty")
	}

	logf("solveTicketmasterCaptcha: Successfully obtained tmpt cookie")
	return tmptCookie, nil
}

func extractTmptCookie(ctx context.Context, client *http.Client, epsData *EPSData) (string, error) {
	logf("extractTmptCookie: Starting extraction with %d max attempts", Constants.Ticketmaster.MaxAttempts)

	var lastError error
	for attempt := 1; attempt <= Constants.Ticketmaster.MaxAttempts; attempt++ {
		logf("extractTmptCookie: Attempt %d/%d", attempt, Constants.Ticketmaster.MaxAttempts)

		// Create a context with timeout for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		tmptCookie, err := solveTicketmasterCaptcha(attemptCtx, client, epsData)
		cancel() // Always cancel the context when done with this attempt

		if err != nil {
			logf("extractTmptCookie: Attempt %d failed: %v", attempt, err)
			lastError = err

			// Use exponential backoff with jitter for retries
			if attempt < Constants.Ticketmaster.MaxAttempts {
				// Calculate backoff time: baseTime * 2^attempt + jitter
				baseDelay := 500 * time.Millisecond
				maxDelay := 5 * time.Second

				delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))
				if delay > maxDelay {
					delay = maxDelay
				}

				// Add jitter (±20% of delay)
				jitter := time.Duration(rand.Float64()*0.4*float64(delay) - 0.2*float64(delay))
				delay += jitter

				logf("extractTmptCookie: Retrying in %v", delay)

				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}
			continue
		}

		if tmptCookie.Value != "" {
			logf("extractTmptCookie: Successfully extracted tmpt cookie on attempt %d", attempt)
			return tmptCookie.Value, nil
		}
	}

	if lastError != nil {
		return "", fmt.Errorf("max attempts reached: %w", lastError)
	}

	return "", errors.New("unable to extract tmpt cookie after maximum attempts")
}

func cookiesHandler(c *gin.Context) {
	logf("cookiesHandler: Processing request")

	proxy := c.Query("proxy")
	if proxy != "" {
		logf("cookiesHandler: Using proxy: %s", proxy)
	}

	// Create a parent context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()

	client, err := createHTTPClient(proxy, 30*time.Second)
	if err != nil {
		logf("cookiesHandler: Failed to create HTTP client: %v", err)
		c.JSON(http.StatusOK, map[string]string{"status": "retry", "error": "invalid_proxy"})
		return
	}

	// First, extract both gecToken and eps_sid in a single request
	epsData, err := extractEPSData(ctx, client)
	if err != nil {
		logf("cookiesHandler: Failed to extract EPS data: %v", err)
		c.JSON(http.StatusOK, map[string]string{"status": "retry", "error": "eps_data_extraction_failed"})
		return
	}

	// Then, extract tmpt cookie using the already extracted gecToken
	tmpt, err := extractTmptCookie(ctx, client, epsData)
	if err != nil {
		logf("cookiesHandler: Failed to extract tmpt cookie: %v", err)
		c.JSON(http.StatusOK, map[string]string{"status": "retry", "error": "tmpt_extraction_failed"})
		return
	}

	// Verify we have both cookies
	if tmpt == "" || epsData.EpsSID == "" {
		logf("cookiesHandler: Empty cookies extracted")
		c.JSON(http.StatusOK, map[string]string{"status": "retry", "error": "empty_cookies"})
		return
	}

	logf("cookiesHandler: Successfully extracted both cookies")
	c.JSON(http.StatusOK, map[string]string{"tmpt": tmpt, "eps_sid": epsData.EpsSID})
}

func solveRecaptchaHandler(c *gin.Context) {
	logf("solveRecaptchaHandler: Processing request")

	var request struct {
		AnchorURL string `json:"anchor_url" binding:"required"`
		Proxy     string `json:"proxy,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		logf("solveRecaptchaHandler: Invalid request payload: %v", err)
		c.JSON(http.StatusBadRequest, RecaptchaResponse{Error: "Invalid request payload"})
		return
	}

	// Validate anchor URL
	if !strings.Contains(request.AnchorURL, "google.com/recaptcha") {
		logf("solveRecaptchaHandler: Invalid anchor URL: %s", request.AnchorURL)
		c.JSON(http.StatusBadRequest, RecaptchaResponse{Error: "Invalid anchor URL"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	client, err := createHTTPClient(request.Proxy, 50*time.Second)
	if err != nil {
		logf("solveRecaptchaHandler: Invalid proxy format: %v", err)
		c.JSON(http.StatusBadRequest, RecaptchaResponse{Error: fmt.Sprintf("Invalid proxy format: %v", err)})
		return
	}

	logf("solveRecaptchaHandler: Solving reCAPTCHA for URL: %s", request.AnchorURL)
	rresp, err := solveRecaptcha(ctx, request.AnchorURL, client)
	if err != nil {
		logf("solveRecaptchaHandler: Failed to solve reCAPTCHA: %v", err)
		c.JSON(http.StatusInternalServerError, RecaptchaResponse{Error: fmt.Sprintf("Failed to solve reCAPTCHA: %v", err)})
		return
	}

	if rresp == "" {
		logf("solveRecaptchaHandler: Empty rresp received")
		c.JSON(http.StatusInternalServerError, RecaptchaResponse{Error: "Empty rresp received"})
		return
	}

	logf("solveRecaptchaHandler: Successfully solved reCAPTCHA")
	c.JSON(http.StatusOK, RecaptchaResponse{RResp: rresp})
}

func main() {
	// Process command line arguments for log toggle
	for _, arg := range os.Args[1:] {
		if arg == "log=true" {
			config.LogEnabled = true
			break
		}
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Register handlers
	router.GET("/cookies", cookiesHandler)
	router.POST("/check", CheckHandler)
	router.POST("/convertCookies", ConvertCookiesHandler)
	router.POST("/solveRecaptcha", solveRecaptchaHandler)

	serverAddress := ":3081"

	fmt.Println("Server started successfully")
	if config.LogEnabled {
		fmt.Println("Logging is enabled")
	}

	if err := router.Run(serverAddress); err != nil {
		fmt.Printf("Failed to run server: %v\n", err)
	}
}
