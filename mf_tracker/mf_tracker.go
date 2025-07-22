package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/andybalholm/brotli"
	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	TelegramBotToken string
	TelegramChatID   string
	FundIDsFile      string
	DatabaseFile     string
	CheckInterval    time.Duration
	RetryAttempts    int
	RetryDelay       time.Duration
	RequestTimeout   time.Duration
	MappingCacheTTL  time.Duration
	RateLimitDelay   time.Duration
}

type FundInfo struct {
	ID   string
	Name string
}

type NAVResponse struct {
	Status string      `json:"status"`
	Data   [][]float64 `json:"data"`
}

type NAVData struct {
	Timestamp  time.Time
	Value      float64
	EntryCount int
}

type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type NAVTracker struct {
	config       Config
	httpClient   *http.Client
	db           *sql.DB
	dbMutex      sync.RWMutex
	funds        []FundInfo
	fundsMutex   sync.RWMutex
	fundMapping  map[string]string
	mappingTime  time.Time
	mappingMutex sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	shutdown     chan struct{}
	rateLimiter  chan struct{}
	stats        struct {
		successCount int64
		errorCount   int64
	}
}

func NewNAVTracker(config Config) *NAVTracker {
	ctx, cancel := context.WithCancel(context.Background())

	return &NAVTracker{
		config: config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:      10,
				IdleConnTimeout:   30 * time.Second,
				DisableKeepAlives: false,
			},
		},
		fundMapping: make(map[string]string),
		ctx:         ctx,
		cancel:      cancel,
		shutdown:    make(chan struct{}),
		rateLimiter: make(chan struct{}, 1), // Only allow 1 concurrent request
	}
}

func (nt *NAVTracker) initDatabase() error {
	var err error

	nt.db, err = sql.Open("sqlite3", nt.config.DatabaseFile+"?_journal_mode=WAL&_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	nt.db.SetMaxOpenConns(5)
	nt.db.SetMaxIdleConns(2)

	if err := nt.db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS fund_cache (
		fund_id TEXT PRIMARY KEY,
		fund_name TEXT NOT NULL,
		last_nav_value REAL NOT NULL,
		last_nav_timestamp INTEGER NOT NULL,
		last_entry_count INTEGER NOT NULL DEFAULT 0,
		last_check_timestamp INTEGER NOT NULL,
		check_count INTEGER DEFAULT 0,
		error_count INTEGER DEFAULT 0,
		last_error TEXT,
		updated_at INTEGER DEFAULT (strftime('%s','now'))
	);

	CREATE TABLE IF NOT EXISTS fund_mapping_cache (
		fund_id TEXT PRIMARY KEY,
		fund_name TEXT NOT NULL,
		cached_at INTEGER DEFAULT (strftime('%s','now'))
	);

	CREATE INDEX IF NOT EXISTS idx_fund_cache_updated ON fund_cache(updated_at);
	CREATE INDEX IF NOT EXISTS idx_mapping_cached ON fund_mapping_cache(cached_at);
	`

	if _, err := nt.db.Exec(schema); err != nil {
		return fmt.Errorf("schema creation failed: %w", err)
	}

	return nil
}

func (nt *NAVTracker) getFundCache(fundID string) (*NAVData, error) {
	nt.dbMutex.RLock()
	defer nt.dbMutex.RUnlock()

	var navValue, navTimestamp, entryCount sql.NullInt64

	query := `SELECT last_nav_value, last_nav_timestamp, last_entry_count 
	          FROM fund_cache WHERE fund_id = ?`

	err := nt.db.QueryRow(query, fundID).Scan(&navValue, &navTimestamp, &entryCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if !navValue.Valid || !navTimestamp.Valid {
		return nil, nil
	}

	return &NAVData{
		Timestamp:  time.Unix(navTimestamp.Int64, 0),
		Value:      float64(navValue.Int64) / 1000, // Store as millis for precision
		EntryCount: int(entryCount.Int64),
	}, nil
}

func (nt *NAVTracker) updateFundCache(fundID, fundName string, navData *NAVData, errorMsg string) error {
	nt.dbMutex.Lock()
	defer nt.dbMutex.Unlock()

	now := time.Now().Unix()

	if navData != nil {
		query := `
		INSERT INTO fund_cache (fund_id, fund_name, last_nav_value, last_nav_timestamp, 
		                       last_entry_count, last_check_timestamp, check_count)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(fund_id) DO UPDATE SET
			fund_name = excluded.fund_name,
			last_nav_value = excluded.last_nav_value,
			last_nav_timestamp = excluded.last_nav_timestamp,
			last_entry_count = excluded.last_entry_count,
			last_check_timestamp = excluded.last_check_timestamp,
			check_count = check_count + 1,
			last_error = NULL
		`
		_, err := nt.db.Exec(query, fundID, fundName, int64(navData.Value*1000),
			navData.Timestamp.Unix(), navData.EntryCount, now)
		return err
	} else {
		query := `
		INSERT INTO fund_cache (fund_id, fund_name, last_check_timestamp, check_count, error_count, last_error)
		VALUES (?, ?, ?, 1, 1, ?)
		ON CONFLICT(fund_id) DO UPDATE SET
			last_check_timestamp = excluded.last_check_timestamp,
			check_count = check_count + 1,
			error_count = error_count + 1,
			last_error = excluded.last_error
		`
		_, err := nt.db.Exec(query, fundID, fundName, now, errorMsg)
		return err
	}
}

func (nt *NAVTracker) loadFundIDs() ([]string, error) {
	file, err := os.Open(nt.config.FundIDsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var fundIDs []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fundID := strings.ToUpper(line)
		if regexp.MustCompile(`^[A-Z0-9]{12}$`).MatchString(fundID) {
			fundIDs = append(fundIDs, fundID)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return fundIDs, nil
}

func (nt *NAVTracker) fetchHTTP(url string) (string, error) {
	req, err := http.NewRequestWithContext(nt.ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Lucif3rHun1; Intel Mac OS X 10.15; rv:129.0) Gecko/20100101 Firefox/129.0")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-GB,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("X-CSRFToken", "undefined")
	req.Header.Set("Origin", "https://coin.zerodha.com")
	req.Header.Set("DNT", "1")
	req.Header.Set("Sec-GPC", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://coin.zerodha.com/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("TE", "trailers")

	resp, err := nt.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body

	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()
		reader = gzReader
	case "br":
		reader = brotli.NewReader(resp.Body)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (nt *NAVTracker) fetchHTTPWithRetry(url string) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= nt.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			delay := nt.config.RetryDelay * time.Duration(attempt)
			select {
			case <-time.After(delay):
			case <-nt.ctx.Done():
				return "", nt.ctx.Err()
			}
		}

		content, err := nt.fetchHTTP(url)
		if err == nil {
			atomic.AddInt64(&nt.stats.successCount, 1)
			return content, nil
		}

		lastErr = err
	}

	atomic.AddInt64(&nt.stats.errorCount, 1)
	return "", lastErr
}

func (nt *NAVTracker) fetchFundMapping() error {
	content, err := os.ReadFile("mapping.js")
	if err != nil {
		return fmt.Errorf("mapping.js not found: %w", err)
	}

	mapping, err := nt.parseFundMapping(string(content))
	if err != nil {
		return err
	}

	nt.mappingMutex.Lock()
	nt.fundMapping = mapping
	nt.mappingTime = time.Now()
	nt.mappingMutex.Unlock()

	nt.saveMappingToCache(mapping)
	log.Printf("Loaded %d fund mappings", len(mapping))
	return nil
}

func (nt *NAVTracker) parseFundMapping(content string) (map[string]string, error) {
	mapping := make(map[string]string)

	// Try JSON format first
	if strings.Contains(content, `"data"`) && strings.Contains(content, `"funds"`) {
		var apiResponse struct {
			Data struct {
				Funds []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"funds"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(content), &apiResponse); err == nil {
			for _, fund := range apiResponse.Data.Funds {
				if len(fund.ID) == 12 && len(fund.Name) > 3 {
					mapping[fund.ID] = fund.Name
				}
			}
			if len(mapping) > 0 {
				return mapping, nil
			}
		}
	}

	// Regex patterns for JS arrays
	patterns := []string{
		`\["([A-Z0-9]{12})","[^"]*","([^"]{3,})"`,
		`\[\"([A-Z0-9]{12})\",\"[^\"]*\",\"([^\"]{3,})\"`,
		`\{[^}]*"id"\s*:\s*"([A-Z0-9]{12})"[^}]*"name"\s*:\s*"([^"]{3,})"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(content, -1)

		for _, match := range matches {
			if len(match) >= 3 {
				fundID, fundName := match[1], match[2]
				if len(fundID) == 12 && len(fundName) > 3 {
					fundName = strings.ReplaceAll(fundName, "\\\"", "\"")
					fundName = regexp.MustCompile(`\s+`).ReplaceAllString(fundName, " ")
					fundName = strings.TrimSpace(fundName)

					if !strings.Contains(fundName, "http") && !strings.Contains(fundName, "function") {
						mapping[fundID] = fundName
					}
				}
			}
		}

		if len(mapping) > 100 {
			break
		}
	}

	if len(mapping) == 0 {
		return nil, fmt.Errorf("no mappings found")
	}

	return mapping, nil
}

func (nt *NAVTracker) saveMappingToCache(mapping map[string]string) error {
	nt.dbMutex.Lock()
	defer nt.dbMutex.Unlock()

	tx, err := nt.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for fundID, fundName := range mapping {
		_, err := tx.Exec(`INSERT OR REPLACE INTO fund_mapping_cache (fund_id, fund_name) VALUES (?, ?)`,
			fundID, fundName)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (nt *NAVTracker) loadMappingFromCache() (map[string]string, error) {
	nt.dbMutex.RLock()
	defer nt.dbMutex.RUnlock()

	mapping := make(map[string]string)
	rows, err := nt.db.Query("SELECT fund_id, fund_name FROM fund_mapping_cache")
	if err != nil {
		return mapping, err
	}
	defer rows.Close()

	for rows.Next() {
		var fundID, fundName string
		if err := rows.Scan(&fundID, &fundName); err != nil {
			continue
		}
		mapping[fundID] = fundName
	}

	return mapping, rows.Err()
}

func (nt *NAVTracker) getFundName(fundID string) string {
	nt.mappingMutex.RLock()
	name, exists := nt.fundMapping[fundID]
	mappingAge := time.Since(nt.mappingTime)
	nt.mappingMutex.RUnlock()

	if exists && mappingAge <= nt.config.MappingCacheTTL {
		return name
	}

	// Try cache
	if cachedMapping, err := nt.loadMappingFromCache(); err == nil && len(cachedMapping) > 0 {
		if cachedName, found := cachedMapping[fundID]; found {
			return cachedName
		}
	}

	return fmt.Sprintf("Fund_%s", fundID)
}

func (nt *NAVTracker) loadFunds() error {
	fundIDs, err := nt.loadFundIDs()
	if err != nil {
		return err
	}

	// Load mapping if not available
	nt.mappingMutex.RLock()
	mappingEmpty := len(nt.fundMapping) == 0
	nt.mappingMutex.RUnlock()

	if mappingEmpty {
		if cachedMapping, err := nt.loadMappingFromCache(); err == nil && len(cachedMapping) > 0 {
			nt.mappingMutex.Lock()
			nt.fundMapping = cachedMapping
			nt.mappingTime = time.Now()
			nt.mappingMutex.Unlock()
		}

		if len(nt.fundMapping) == 0 {
			if err := nt.fetchFundMapping(); err != nil {
				log.Printf("Warning: Could not fetch fund mapping: %v", err)
			}
		}
	}

	var funds []FundInfo
	for _, fundID := range fundIDs {
		funds = append(funds, FundInfo{
			ID:   fundID,
			Name: nt.getFundName(fundID),
		})
	}

	nt.fundsMutex.Lock()
	nt.funds = funds
	nt.fundsMutex.Unlock()

	log.Printf("Loaded %d funds for tracking", len(funds))
	return nil
}

func (nt *NAVTracker) fetchNAVData(fundID string) (*NAVData, error) {
	url := fmt.Sprintf("https://staticassets.zerodha.com/coin/historical-nav/%s.json", fundID)

	content, err := nt.fetchHTTPWithRetry(url)
	if err != nil {
		return nil, err
	}

	if !json.Valid([]byte(content)) {
		return nil, fmt.Errorf("invalid JSON")
	}

	var navResponse NAVResponse
	if err := json.Unmarshal([]byte(content), &navResponse); err != nil {
		return nil, err
	}

	if navResponse.Status != "success" {
		return nil, fmt.Errorf("API error: %s", navResponse.Status)
	}

	entryCount := len(navResponse.Data)
	if entryCount == 0 {
		return nil, fmt.Errorf("no data")
	}

	latest := navResponse.Data[entryCount-1]
	if len(latest) < 2 || latest[1] <= 0 {
		return nil, fmt.Errorf("invalid data")
	}

	return &NAVData{
		Timestamp:  time.Unix(int64(latest[0]), 0),
		Value:      latest[1],
		EntryCount: entryCount,
	}, nil
}

func (nt *NAVTracker) sendTelegramMessage(message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", nt.config.TelegramBotToken)

	telegramMsg := TelegramMessage{
		ChatID:    nt.config.TelegramChatID,
		Text:      message,
		ParseMode: "HTML",
	}

	jsonData, err := json.Marshal(telegramMsg)
	if err != nil {
		return err
	}

	for attempt := 0; attempt <= 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequestWithContext(nt.ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		resp, err := nt.httpClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}

		resp.Body.Close()
		if attempt == 2 {
			return fmt.Errorf("telegram failed: %d", resp.StatusCode)
		}
	}

	return fmt.Errorf("telegram retries exhausted")
}

func (nt *NAVTracker) generateHashtag(fundName string) string {
	words := strings.Fields(fundName)
	var cleanWords []string

	skipWords := map[string]bool{
		"fund": true, "direct": true, "growth": true, "regular": true,
		"plan": true, "option": true, "mutual": true, "the": true,
		"and": true, "of": true, "in": true, "for": true, "-": true,
	}

	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, "()[]{}"))
		if !skipWords[cleanWord] && len(word) > 1 {
			cleanWords = append(cleanWords, word)
		}
	}

	if len(cleanWords) == 0 {
		cleanWords = []string{"Fund"}
	}

	hashtag := "#" + strings.Join(cleanWords, "")
	return regexp.MustCompile(`[^a-zA-Z0-9_#]`).ReplaceAllString(hashtag, "")
}

func (nt *NAVTracker) formatFirstCheckMessage(fund FundInfo, currentNAV *NAVData) string {
	return fmt.Sprintf(
		`📊 <b>NAV Tracking Started</b>

<b>%s</b>
<b>Date:</b> %s
<b>NAV:</b> ₹%.3f

Now monitoring for changes.

%s #NAVTracker
<i>%s</i>`,
		fund.Name,
		currentNAV.Timestamp.Format("02-Jan-2006"),
		currentNAV.Value,
		nt.generateHashtag(fund.Name),
		time.Now().Format("15:04"),
	)
}

func (nt *NAVTracker) formatChangeMessage(fund FundInfo, currentNAV, previousNAV *NAVData) string {
	changeAmount := currentNAV.Value - previousNAV.Value
	changePercent := (changeAmount / previousNAV.Value) * 100

	var status, emoji string
	if changeAmount > 0 {
		status = "Increased"
		emoji = "📈"
	} else if changeAmount < 0 {
		status = "Decreased"
		emoji = "📉"
	} else {
		status = "Unchanged"
		emoji = "➡️"
	}

	return fmt.Sprintf(
		`%s <b>NAV Update</b>

<b>%s</b>
<b>Date:</b> %s

<b>Current:</b> ₹%.3f
<b>Previous:</b> ₹%.3f
<b>Change:</b> ₹%.3f (%.2f%%)

<b>Status:</b> %s

%s #NAVUpdate
<i>%s</i>`,
		emoji,
		fund.Name,
		currentNAV.Timestamp.Format("02-Jan-2006"),
		currentNAV.Value,
		previousNAV.Value,
		changeAmount,
		changePercent,
		status,
		nt.generateHashtag(fund.Name),
		time.Now().Format("15:04"),
	)
}

func (nt *NAVTracker) checkFund(fund FundInfo) error {
	// Rate limiting - only one request at a time
	select {
	case nt.rateLimiter <- struct{}{}:
		defer func() { <-nt.rateLimiter }()
	case <-nt.ctx.Done():
		return nt.ctx.Err()
	}

	time.Sleep(nt.config.RateLimitDelay)

	currentNAV, err := nt.fetchNAVData(fund.ID)
	if err != nil {
		nt.updateFundCache(fund.ID, fund.Name, nil, err.Error())
		return fmt.Errorf("fetch NAV failed for %s: %w", fund.ID, err)
	}

	cachedNAV, err := nt.getFundCache(fund.ID)
	if err != nil {
		nt.updateFundCache(fund.ID, fund.Name, currentNAV, "")
		return fmt.Errorf("cache error for %s: %w", fund.ID, err)
	}

	shouldNotify := false
	isFirstCheck := cachedNAV == nil

	if cachedNAV != nil {
		if currentNAV.EntryCount > cachedNAV.EntryCount {
			shouldNotify = true
			log.Printf("🆕 New NAV entry for %s: %d→%d entries, ₹%.3f→₹%.3f",
				fund.ID, cachedNAV.EntryCount, currentNAV.EntryCount, cachedNAV.Value, currentNAV.Value)
		} else {
			log.Printf("✅ No new entries for %s: %d entries, ₹%.3f",
				fund.ID, currentNAV.EntryCount, currentNAV.Value)
		}
	} else {
		shouldNotify = true
		log.Printf("📊 First check for %s: %d entries, ₹%.3f",
			fund.ID, currentNAV.EntryCount, currentNAV.Value)
	}

	if shouldNotify {
		var message string
		if isFirstCheck {
			message = nt.formatFirstCheckMessage(fund, currentNAV)
		} else {
			message = nt.formatChangeMessage(fund, currentNAV, cachedNAV)
		}

		if err := nt.sendTelegramMessage(message); err != nil {
			nt.updateFundCache(fund.ID, fund.Name, currentNAV, fmt.Sprintf("Telegram: %v", err))
			return fmt.Errorf("telegram failed for %s: %w", fund.ID, err)
		} else {
			log.Printf("📤 Notification sent for %s", fund.Name)
		}
	}

	return nt.updateFundCache(fund.ID, fund.Name, currentNAV, "")
}

func (nt *NAVTracker) checkAllFunds() {
	nt.fundsMutex.RLock()
	funds := make([]FundInfo, len(nt.funds))
	copy(funds, nt.funds)
	nt.fundsMutex.RUnlock()

	if len(funds) == 0 {
		log.Println("No funds to check")
		return
	}

	log.Printf("Checking %d funds...", len(funds))

	// Sequential processing with rate limiting
	for _, fund := range funds {
		if err := nt.checkFund(fund); err != nil {
			log.Printf("Error checking %s: %v", fund.ID, err)
		}

		// Check for shutdown
		select {
		case <-nt.ctx.Done():
			log.Println("Check cancelled")
			return
		default:
		}
	}

	success := atomic.LoadInt64(&nt.stats.successCount)
	errors := atomic.LoadInt64(&nt.stats.errorCount)
	log.Printf("Check completed - Success: %d, Errors: %d", success, errors)
}

func (nt *NAVTracker) setupGracefulShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received...")
		nt.cancel()

		// Wait for graceful shutdown
		done := make(chan struct{})
		go func() {
			nt.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("Graceful shutdown completed")
		case <-time.After(10 * time.Second):
			log.Println("Shutdown timeout, forcing exit")
		}

		close(nt.shutdown)
	}()
}

func (nt *NAVTracker) Start() {
	log.Println("Starting NAV Tracker...")

	if err := nt.initDatabase(); err != nil {
		log.Fatalf("Database init failed: %v", err)
	}
	defer nt.db.Close()

	if err := nt.loadFunds(); err != nil {
		log.Fatalf("Load funds failed: %v", err)
	}

	nt.setupGracefulShutdown()

	log.Println("Running initial check...")
	nt.checkAllFunds()

	ticker := time.NewTicker(nt.config.CheckInterval)
	defer ticker.Stop()

	log.Printf("Monitoring every %v", nt.config.CheckInterval)

	for {
		select {
		case <-ticker.C:
			nt.checkAllFunds()
		case <-nt.shutdown:
			log.Println("Shutting down...")
			return
		case <-nt.ctx.Done():
			log.Println("Context cancelled")
			return
		}
	}
}

func main() {
	config := Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		FundIDsFile:      "fund_ids.txt",
		DatabaseFile:     "nav_tracker.db",
		CheckInterval:    15 * time.Minute,
		RetryAttempts:    3,
		RetryDelay:       2 * time.Second,
		RequestTimeout:   30 * time.Second,
		MappingCacheTTL:  6 * time.Hour,
		RateLimitDelay:   2 * time.Second,
	}

	if config.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable required")
	}
	if config.TelegramChatID == "" {
		log.Fatal("TELEGRAM_CHAT_ID environment variable required")
	}

	if _, err := os.Stat(config.FundIDsFile); os.IsNotExist(err) {
		sampleContent := `INF846K01EW2
INF090I01039
INF204K01CI0
INF277K01AQ8`
		if err := os.WriteFile(config.FundIDsFile, []byte(sampleContent), 0644); err != nil {
			log.Fatalf("Error creating fund IDs file: %v", err)
		}
		log.Fatalf("Created %s. Please add your fund IDs and restart", config.FundIDsFile)
	}

	if _, err := os.Stat("mapping.js"); os.IsNotExist(err) {
		log.Printf("mapping.js not found. Fund names will show as Fund_ID format.")
	}

	tracker := NewNAVTracker(config)
	tracker.Start()
}
