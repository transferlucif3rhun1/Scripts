package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io" // <-- For io.ReadAll in CheckHandler
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------
// INTEGRATED /check ENDPOINT VARIABLES & TYPES
// ---------------------------

// Reused: words that trigger a "retry" response.
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

// JSON response structure for /check.
type CheckResponse struct {
	Status string `json:"status"`
}

// Helper to handle errors in /check handler.
func HandleCheckError(c *gin.Context, err error, statusCode int, message string) {
	c.Header("Content-Type", "application/json")
	// Always respond with "retry" in this example.
	c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
	c.Abort()
}

// Core /check functionality.
func CheckHandler(c *gin.Context) {
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
		c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
		return
	}

	// Attempt to extract the `body` value from JSON:
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
	// If no separate body found, we'll check the entire request.
	if textToCheck == "" {
		textToCheck = requestBody
	}

	lowerText := strings.ToLower(textToCheck)
	for _, word := range blockedWords {
		lowerWord := strings.ToLower(word)
		if strings.Contains(lowerText, lowerWord) {
			c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
			return
		}
	}

	// If no blocked keywords, echo the original body as JSON.
	c.Data(http.StatusOK, "application/json", bodyBytes)
}

// ---------------------------
// CONFIGURABLE GLOBALS
// ---------------------------

var remoteBase = "http://195.201.106.250:8000"

// We allow up to N active ranges at once
const maxActiveRanges = 20
const sizeOfrid = 5

// Each range can store up to 5 sets
const maxQueuePerRange = 5

// If no requests for 5 minutes => clear memory
const inactivityThreshold = 5 * time.Minute

// Each remote fetch can take up to 1 minute
const fetchTimeout = 1 * time.Minute

// ---------------------------
// PROXY
// ---------------------------

// If user typed `ip:port` => "http://ip:port"
// If user typed `ip:port:user:pass` => "http://user:pass@ip:port"
// Otherwise, ""
var proxyURL string

// ---------------------------
// BROWSERLIST
// Define a dynamic list of browsers fetched from the remote server.
// ---------------------------

type BrowserListResponse struct {
	Browsers []string `json:"browsers"`
}

var browsers []string

func fetchBrowserList() error {
	url := remoteBase + "/browserlist"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("browserlist returned status code=%d", resp.StatusCode)
	}

	var blResp BrowserListResponse
	if err := json.NewDecoder(resp.Body).Decode(&blResp); err != nil {
		return err
	}

	if len(blResp.Browsers) == 0 {
		return errors.New("no browsers found in response")
	}

	browsers = blResp.Browsers
	return nil
}

// ---------------------------
// DATA STRUCTURES
// ---------------------------

// A CookieSet is what we store per range. We do not return the browser name to the user.
type CookieSet struct {
	ID       int               `json:"id"`
	RangeIdx int               `json:"rangeIdx"`
	Browser  string            `json:"-"` // hidden in JSON
	Cookies  map[string]string `json:"cookies"`
}

var (
	nextLocalID int
	idMux       sync.Mutex
)

func getNextLocalID() int {
	idMux.Lock()
	defer idMux.Unlock()
	nextLocalID++
	return nextLocalID
}

// rangeData represents one "active" range, with a queue of CookieSets (FIFO).
type rangeData struct {
	rangeIdx    int
	browserName string

	queue []*CookieSet // FIFO queue of CookieSets

	workerCtx  context.Context
	workerStop context.CancelFunc
	isActive   bool

	fetchErr error
	fetching bool

	mu sync.RWMutex
}

// ---------------------------
// GLOBAL MAPS / STATE
// ---------------------------

var (
	rangeMap    = make(map[int]*rangeData) // rangeIdx => *rangeData
	rangeMapMu  sync.Mutex                 // protect creation
	activeCount int

	idMap sync.Map // localID => *CookieSet

	// inactivity
	inactivityTimer    *time.Timer
	inactivityTimerMux sync.Mutex
)

// ---------------------------
// MAIN
// ---------------------------

func main() {
	gin.SetMode(gin.ReleaseMode)

	// 1) Parse proxy
	parseProxy()

	// 2) Fetch browserlist
	if err := fetchBrowserList(); err != nil {
		fmt.Printf("[FATAL] fetchBrowserList error: %v\n", err)
		os.Exit(1)
	}
	if len(browsers) == 0 {
		fmt.Println("[FATAL] no browsers loaded; cannot proceed.")
		os.Exit(1)
	}

	// 3) Setup Gin
	r := gin.New()
	r.GET("/cookies", handleGetCookies)
	r.DELETE("/cookies", handleDeleteCookies)

	// 4) Add the /check endpoint
	r.POST("/check", CheckHandler)

	// 5) Graceful shutdown listener
	go handleGracefulShutdown()

	// 6) Start server
	fmt.Println("[INFO] Server started on :3081")
	if err := r.Run(":3081"); err != nil {
		fmt.Printf("[FATAL] server crashed: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------
// PARSE PROXY
// ---------------------------

func parseProxy() {
	// Check if proxy.txt exists
	info, err := os.Stat("proxy.txt")
	if errors.Is(err, os.ErrNotExist) {
		// Create an empty file
		f, err2 := os.Create("proxy.txt")
		if err2 != nil {
			fmt.Printf("[WARN] Could not create proxy.txt: %v\n", err2)
		} else {
			f.Close()
		}
		fmt.Println("[INFO] proxy.txt not found. Created empty file. Running proxyless...")
		proxyURL = ""
		return
	}

	// If it's a directory, not a file
	if info.IsDir() {
		fmt.Println("[WARN] proxy.txt is a directory. Running proxyless...")
		proxyURL = ""
		return
	}

	// Read the file content
	content, err := os.ReadFile("proxy.txt")
	if err != nil {
		fmt.Printf("[WARN] Error reading proxy.txt: %v. Running proxyless...\n", err)
		proxyURL = ""
		return
	}

	line := strings.TrimSpace(string(content))
	if line == "" {
		fmt.Println("[INFO] proxy.txt is empty. Running proxyless...")
		proxyURL = ""
		return
	}

	// Parse the proxy string ( ip:port or ip:port:user:pass )
	parts := strings.Split(line, ":")
	switch len(parts) {
	case 2:
		// ip:port
		ip := parts[0]
		port := parts[1]
		proxyURL = "http://" + ip + ":" + port
		fmt.Printf("[INFO] Using proxy: %s\n", proxyURL)
	case 4:
		// ip:port:user:pass
		ip := parts[0]
		port := parts[1]
		user := parts[2]
		pass := parts[3]
		proxyURL = fmt.Sprintf("http://%s:%s@%s:%s", user, pass, ip, port)
		fmt.Printf("[INFO] Using proxy: %s\n", proxyURL)
	default:
		fmt.Println("[WARN] proxy.txt has an invalid format. Running proxyless...")
		proxyURL = ""
	}
}

// ---------------------------
// HANDLERS
// ---------------------------

// GET /cookies?number=? or ?id=?
func handleGetCookies(c *gin.Context) {
	resetInactivity()

	qID := c.Query("id")
	qNumber := c.Query("number")

	// If both "id" and "number" are provided, we give priority to "id".
	// If not found or invalid => fallback to number logic.
	if qID != "" && qNumber != "" {
		localID, err := strconv.Atoi(qID)
		if err == nil {
			val, ok := idMap.Load(localID)
			if ok {
				// ID found => return it immediately
				cs := val.(*CookieSet)
				c.JSON(http.StatusOK, gin.H{
					"id":       cs.ID,
					"rangeIdx": cs.RangeIdx,
					"cookies":  cs.Cookies,
				})
				return
			}
		}
		// If ID not found or invalid => fallback to number
		handleGetCookiesByNumber(c, qNumber)
		return
	}

	// If only "id" is present
	if qID != "" {
		localID, err := strconv.Atoi(qID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		val, ok := idMap.Load(localID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		cs := val.(*CookieSet)
		c.JSON(http.StatusOK, gin.H{
			"id":       cs.ID,
			"rangeIdx": cs.RangeIdx,
			"cookies":  cs.Cookies,
		})
		return
	}

	// If only "number" is present
	if qNumber != "" {
		handleGetCookiesByNumber(c, qNumber)
		return
	}

	// If neither is present
	c.JSON(http.StatusBadRequest, gin.H{"error": "must provide ?id= or ?number="})
}

// Helper for "number" logic
func handleGetCookiesByNumber(c *gin.Context, qNumber string) {
	numVal, err := strconv.Atoi(qNumber)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid number"})
		return
	}
	// If number=0 => treat as 1
	if numVal < 1 {
		numVal = 1
	}

	// e.g. 1..10 => rangeIdx=0, 11..20 => rangeIdx=1, etc.
	rangeIdx := (numVal - 1) / sizeOfrid

	// Check if rangeIdx exceeds available browsers
	if rangeIdx >= len(browsers) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "range index exceeds available browsers"})
		return
	}

	// Check concurrency
	if rangeIdx >= maxActiveRanges {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "range limit reached"})
		return
	}

	// Get or create the range
	rd := getOrCreateRange(rangeIdx)
	if rd == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no browser assigned to this range"})
		return
	}

	// Now, see if we have cookies in the queue
	rd.mu.RLock()
	queueLen := len(rd.queue)
	fErr := rd.fetchErr
	fch := rd.fetching
	active := rd.isActive
	rd.mu.RUnlock()

	// If not active => spin up worker if concurrency allows
	if !active {
		rangeMapMu.Lock()
		if activeCount < maxActiveRanges {
			activeCount++
			rd.isActive = true
			go worker(rd)
		} else {
			rangeMapMu.Unlock()
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "concurrency limit reached"})
			return
		}
		rangeMapMu.Unlock()
	}

	// If the queue is not empty => return the first cookie
	if queueLen > 0 {
		front := getFrontCookie(rd)
		if front != nil {
			c.JSON(http.StatusOK, gin.H{
				"id":       front.ID,
				"rangeIdx": front.RangeIdx,
				"cookies":  front.Cookies,
			})
			return
		}
	}

	// Otherwise, if there's a fetch error => retry
	if fErr != nil {
		c.JSON(http.StatusOK, gin.H{"status": "retry"})
		return
	}

	// If we are currently fetching => processing
	if fch {
		c.JSON(http.StatusOK, gin.H{"status": "processing"})
		return
	}

	// If no items yet and not fetching => "processing"
	c.JSON(http.StatusOK, gin.H{"status": "processing"})
}

// DELETE /cookies?id=?
func handleDeleteCookies(c *gin.Context) {
	resetInactivity()

	qID := c.Query("id")
	if qID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}
	localID, err := strconv.Atoi(qID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	val, ok := idMap.Load(localID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	cs := val.(*CookieSet)
	idMap.Delete(localID)

	rd := rangeMap[cs.RangeIdx]
	if rd == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "range data not found"})
		return
	}

	// Remove from the queue
	rd.mu.Lock()
	for i, s := range rd.queue {
		if s.ID == localID {
			rd.queue = append(rd.queue[:i], rd.queue[i+1:]...)
			break
		}
	}
	rd.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ---------------------------
// WORKER
// ---------------------------

// Retrieve or create the rangeData for the given index
func getOrCreateRange(idx int) *rangeData {
	rangeMapMu.Lock()
	defer rangeMapMu.Unlock()

	if rd, ok := rangeMap[idx]; ok {
		return rd
	}
	ctx, cancel := context.WithCancel(context.Background())

	if idx >= len(browsers) {
		fmt.Printf("[WARN] Range index %d exceeds available browsers. Max = %d\n", idx, len(browsers))
		cancel()
		return nil
	}

	assignedBrowser := browsers[idx]
	rd := &rangeData{
		rangeIdx:    idx,
		browserName: assignedBrowser,
		queue:       []*CookieSet{},
		workerCtx:   ctx,
		workerStop:  cancel,
	}
	rangeMap[idx] = rd
	return rd
}

// The worker continuously fetches cookies in the background (if there's capacity).
func worker(rd *rangeData) {
	defer func() {
		rangeMapMu.Lock()
		activeCount--
		rd.isActive = false
		rd.workerStop()
		rangeMapMu.Unlock()
	}()

	for {
		select {
		case <-rd.workerCtx.Done():
			return
		default:
			rd.mu.RLock()
			n := len(rd.queue)
			rd.mu.RUnlock()

			if n >= maxQueuePerRange {
				time.Sleep(2 * time.Second)
				continue
			}

			// Prepare to fetch
			rd.mu.Lock()
			rd.fetching = true
			rd.fetchErr = nil
			rd.mu.Unlock()

			cs, err := doFetch(rd)
			rd.mu.Lock()
			rd.fetching = false
			if err != nil {
				rd.fetchErr = err
				rd.mu.Unlock()
				time.Sleep(2 * time.Second)
				continue
			}
			// Build the new CookieSet
			cs.ID = getNextLocalID()
			cs.RangeIdx = rd.rangeIdx
			cs.Browser = rd.browserName

			// Push to the queue (FIFO)
			rd.queue = append(rd.queue, cs)
			rd.mu.Unlock()

			// Also store globally by ID => *CookieSet
			idMap.Store(cs.ID, cs)
		}
	}
}

// doFetch calls the remote server, requesting cookies for the assigned browser.
func doFetch(rd *rangeData) (*CookieSet, error) {
	ctx2, cancel := context.WithTimeout(rd.workerCtx, fetchTimeout)
	defer cancel()

	paramBrowser := rd.browserName

	url := remoteBase + "/cookies?browser=" + paramBrowser
	if proxyURL != "" {
		url += "&proxy=" + proxyURL
	}

	req, err := http.NewRequestWithContext(ctx2, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var obj struct {
		Status  string            `json:"status"`
		Cookies map[string]string `json:"cookies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}

	if obj.Status != "success" {
		return nil, fmt.Errorf("remote status=%s", obj.Status)
	}

	return &CookieSet{
		Browser: rd.browserName,
		Cookies: obj.Cookies,
	}, nil
}

// Return the front (first) CookieSet in the queue (FIFO)
func getFrontCookie(rd *rangeData) *CookieSet {
	rd.mu.RLock()
	defer rd.mu.RUnlock()
	if len(rd.queue) == 0 {
		return nil
	}
	return rd.queue[0]
}

// ---------------------------
// INACTIVITY TIMER
// ---------------------------

func resetInactivity() {
	inactivityTimerMux.Lock()
	defer inactivityTimerMux.Unlock()
	if inactivityTimer != nil {
		inactivityTimer.Stop()
	}
	inactivityTimer = time.AfterFunc(inactivityThreshold, func() {
		rangeMapMu.Lock()
		// Stop all workers
		for _, rd := range rangeMap {
			rd.workerStop()
		}
		rangeMap = make(map[int]*rangeData)
		activeCount = 0
		rangeMapMu.Unlock()

		idMap.Range(func(k, _ any) bool {
			idMap.Delete(k)
			return true
		})
	})
}

// ---------------------------
// GRACEFUL SHUTDOWN
// ---------------------------

func handleGracefulShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n[INFO] Shutting down server...")

	// Stop inactivity timer
	resetInactivity()

	// Stop all workers
	rangeMapMu.Lock()
	var wg sync.WaitGroup
	for _, rd := range rangeMap {
		wg.Add(1)
		go func(r *rangeData) {
			defer wg.Done()
			r.workerStop()
		}(rd)
	}
	rangeMapMu.Unlock()

	// Wait for all workers to finish
	wg.Wait()

	fmt.Println("[INFO] All workers stopped. Exiting.")
	os.Exit(0)
}
