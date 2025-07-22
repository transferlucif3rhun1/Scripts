package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Config struct {
	TelegramBotToken string
	TelegramChatID   string
	URLsFile         string
	DatabaseFile     string
	CheckInterval    time.Duration
	RetryAttempts    int
	RetryDelay       time.Duration
	RequestTimeout   time.Duration
}

type FundInfo struct {
	ID   string
	Name string
	URL  string
}

type NAVResponse struct {
	Status string      `json:"status"`
	Data   [][]float64 `json:"data"`
}

type NAVData struct {
	Timestamp time.Time
	Value     float64
}

type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

type NAVTracker struct {
	config     Config
	httpClient *http.Client
	db         *sql.DB
	funds      []FundInfo
	mutex      sync.RWMutex
}

func NewNAVTracker(config Config) *NAVTracker {
	return &NAVTracker{
		config: config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
		},
	}
}

func (nt *NAVTracker) initDatabase() error {
	var err error
	nt.db, err = sql.Open("sqlite3", nt.config.DatabaseFile)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS fund_cache (
		fund_id TEXT PRIMARY KEY,
		fund_name TEXT NOT NULL,
		last_nav_value REAL,
		last_nav_timestamp INTEGER,
		last_check_timestamp INTEGER,
		check_count INTEGER DEFAULT 0,
		error_count INTEGER DEFAULT 0,
		last_error TEXT,
		created_at INTEGER DEFAULT (strftime('%s','now')),
		updated_at INTEGER DEFAULT (strftime('%s','now'))
	);

	CREATE INDEX IF NOT EXISTS idx_fund_id ON fund_cache(fund_id);
	CREATE INDEX IF NOT EXISTS idx_last_check ON fund_cache(last_check_timestamp);
	`

	_, err = nt.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}

	return nil
}

func (nt *NAVTracker) getFundCache(fundID string) (*NAVData, error) {
	var navValue sql.NullFloat64
	var navTimestamp sql.NullInt64

	query := "SELECT last_nav_value, last_nav_timestamp FROM fund_cache WHERE fund_id = ?"
	err := nt.db.QueryRow(query, fundID).Scan(&navValue, &navTimestamp)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying cache: %w", err)
	}

	if !navValue.Valid || !navTimestamp.Valid {
		return nil, nil
	}

	return &NAVData{
		Timestamp: time.Unix(navTimestamp.Int64, 0),
		Value:     navValue.Float64,
	}, nil
}

func (nt *NAVTracker) updateFundCache(fundID, fundName string, navData *NAVData, errorMsg string) error {
	now := time.Now().Unix()

	if navData != nil {
		insertSQL := `
		INSERT INTO fund_cache (fund_id, fund_name, last_nav_value, last_nav_timestamp, last_check_timestamp, check_count, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(fund_id) DO UPDATE SET
			fund_name = excluded.fund_name,
			last_nav_value = excluded.last_nav_value,
			last_nav_timestamp = excluded.last_nav_timestamp,
			last_check_timestamp = excluded.last_check_timestamp,
			check_count = check_count + 1,
			updated_at = excluded.updated_at,
			last_error = CASE WHEN excluded.last_nav_value IS NOT NULL THEN NULL ELSE last_error END
		`
		_, err := nt.db.Exec(insertSQL, fundID, fundName, navData.Value, navData.Timestamp.Unix(), now, now)
		return err
	} else {
		updateErrorSQL := `
		INSERT INTO fund_cache (fund_id, fund_name, last_check_timestamp, check_count, error_count, last_error, updated_at)
		VALUES (?, ?, ?, 1, 1, ?, ?)
		ON CONFLICT(fund_id) DO UPDATE SET
			fund_name = excluded.fund_name,
			last_check_timestamp = excluded.last_check_timestamp,
			check_count = check_count + 1,
			error_count = error_count + 1,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
		`
		_, err := nt.db.Exec(updateErrorSQL, fundID, fundName, now, errorMsg, now)
		return err
	}
}

func (nt *NAVTracker) parseFundURL(urlStr string) (*FundInfo, error) {
	patterns := []string{
		`https://coin\.zerodha\.com/mf/fund/([A-Z0-9]+)/(.+)`,
		`coin\.zerodha\.com/mf/fund/([A-Z0-9]+)/(.+)`,
		`/mf/fund/([A-Z0-9]+)/(.+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(urlStr)
		if len(matches) == 3 {
			fundID := matches[1]
			fundSlug := matches[2]
			fundName := strings.ReplaceAll(fundSlug, "-", " ")
			fundName = strings.Title(fundName)
			return &FundInfo{
				ID:   fundID,
				Name: fundName,
				URL:  urlStr,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid URL format: %s", urlStr)
}

func (nt *NAVTracker) loadFunds() error {
	file, err := os.Open(nt.config.URLsFile)
	if err != nil {
		return fmt.Errorf("opening URLs file: %w", err)
	}
	defer file.Close()

	var funds []FundInfo
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fundInfo, err := nt.parseFundURL(line)
		if err != nil {
			log.Printf("Warning: Invalid URL on line %d: %v", lineNum, err)
			continue
		}

		funds = append(funds, *fundInfo)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading URLs file: %w", err)
	}

	nt.mutex.Lock()
	nt.funds = funds
	nt.mutex.Unlock()

	log.Printf("Loaded %d funds from %s", len(funds), nt.config.URLsFile)
	return nil
}

func (nt *NAVTracker) makeRequestWithRetry(url string, headers map[string]string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= nt.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * nt.config.RetryDelay
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			time.Sleep(delay)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("creating request: %w", err)
			continue
		}

		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := nt.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("making request (attempt %d): %w", attempt+1, err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			lastErr = fmt.Errorf("rate limited (attempt %d)", attempt+1)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d (attempt %d)", resp.StatusCode, attempt+1)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("reading response body (attempt %d): %w", attempt+1, err)
			continue
		}

		return body, nil
	}

	return nil, lastErr
}

func (nt *NAVTracker) fetchNAVData(fundID string) (*NAVData, error) {
	url := fmt.Sprintf("https://staticassets.zerodha.com/coin/historical-nav/%s.json", fundID)

	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Lucif3rHun1; Intel Mac OS X 10.15; rv:129.0) Gecko/20100101 Firefox/129.0",
		"Accept":          "application/json, text/plain, */*",
		"Accept-Language": "en-GB,en;q=0.5",
		"Accept-Encoding": "gzip, deflate, br, zstd",
		"X-CSRFToken":     "undefined",
		"Origin":          "https://coin.zerodha.com",
		"DNT":             "1",
		"Sec-GPC":         "1",
		"Connection":      "keep-alive",
		"Referer":         "https://coin.zerodha.com/",
		"Sec-Fetch-Dest":  "empty",
		"Sec-Fetch-Mode":  "cors",
		"Sec-Fetch-Site":  "same-site",
		"Pragma":          "no-cache",
		"Cache-Control":   "no-cache",
		"TE":              "trailers",
	}

	body, err := nt.makeRequestWithRetry(url, headers)
	if err != nil {
		return nil, err
	}

	var navResponse NAVResponse
	if err := json.Unmarshal(body, &navResponse); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if navResponse.Status != "success" {
		return nil, fmt.Errorf("API returned status: %s", navResponse.Status)
	}

	if len(navResponse.Data) == 0 {
		return nil, fmt.Errorf("no data in response")
	}

	latest := navResponse.Data[len(navResponse.Data)-1]
	if len(latest) < 2 {
		return nil, fmt.Errorf("invalid data format")
	}

	timestamp := time.Unix(int64(latest[0]), 0)
	value := latest[1]

	if value <= 0 {
		return nil, fmt.Errorf("invalid NAV value: %f", value)
	}

	return &NAVData{
		Timestamp: timestamp,
		Value:     value,
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
		return fmt.Errorf("marshaling telegram message: %w", err)
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	body, err := nt.makeRequestWithRetry(url, headers)
	if err != nil {
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, postErr := nt.httpClient.Do(req)
		if postErr != nil {
			return fmt.Errorf("sending telegram message: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("telegram API error: %d - %s", resp.StatusCode, string(bodyBytes))
		}
		return nil
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err == nil {
		if ok, exists := response["ok"].(bool); exists && !ok {
			return fmt.Errorf("telegram API error: %v", response)
		}
	}

	return nil
}

func (nt *NAVTracker) generateHashtag(fundName string) string {
	words := strings.Fields(fundName)
	var cleanWords []string

	skipWords := map[string]bool{
		"fund": true, "direct": true, "growth": true, "regular": true,
		"plan": true, "option": true, "mutual": true, "the": true,
		"and": true, "of": true, "in": true, "for": true,
	}

	for _, word := range words {
		cleanWord := strings.ToLower(word)
		if !skipWords[cleanWord] && len(word) > 1 {
			cleanWords = append(cleanWords, word)
		}
	}

	if len(cleanWords) == 0 {
		cleanWords = []string{"Fund"}
	}

	hashtag := "#" + strings.Join(cleanWords, "")
	re := regexp.MustCompile(`[^a-zA-Z0-9_#]`)
	return re.ReplaceAllString(hashtag, "")
}

func (nt *NAVTracker) formatMessage(fund FundInfo, currentNAV, previousNAV *NAVData) string {
	changeAmount := currentNAV.Value - previousNAV.Value
	changePercent := (changeAmount / previousNAV.Value) * 100

	var direction, emoji string
	if changeAmount > 0 {
		direction = "increased"
		emoji = "📈"
	} else if changeAmount < 0 {
		direction = "decreased"
		emoji = "📉"
	} else {
		direction = "unchanged"
		emoji = "➡️"
	}

	fundHashtag := nt.generateHashtag(fund.Name)
	dateHashtag := fmt.Sprintf("#NAV_%s", currentNAV.Timestamp.Format("02Jan2006"))

	return fmt.Sprintf(
		`%s <b>NAV Update Alert</b> %s

<b>Fund:</b> %s
<b>Fund ID:</b> <code>%s</code>
<b>Date:</b> %s

<b>Current NAV:</b> ₹%.3f
<b>Previous NAV:</b> ₹%.3f
<b>Change:</b> ₹%.3f (%.2f%%)

<b>Status:</b> NAV has %s

%s #NAVUpdate #MutualFund #ZerodhaTracker
%s #DirectGrowth

<i>Updated: %s</i>`,
		emoji, emoji,
		fund.Name,
		fund.ID,
		currentNAV.Timestamp.Format("02-Jan-2006"),
		currentNAV.Value,
		previousNAV.Value,
		changeAmount,
		changePercent,
		direction,
		fundHashtag,
		dateHashtag,
		time.Now().Format("15:04:05 MST"),
	)
}

func (nt *NAVTracker) checkFund(fund FundInfo) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in checkFund for %s: %v", fund.ID, r)
			nt.updateFundCache(fund.ID, fund.Name, nil, fmt.Sprintf("Panic: %v", r))
		}
	}()

	log.Printf("Checking %s (%s)", fund.Name, fund.ID)

	currentNAV, err := nt.fetchNAVData(fund.ID)
	if err != nil {
		log.Printf("Error fetching NAV for %s: %v", fund.ID, err)
		nt.updateFundCache(fund.ID, fund.Name, nil, err.Error())
		return
	}

	cachedNAV, err := nt.getFundCache(fund.ID)
	if err != nil {
		log.Printf("Error getting cache for %s: %v", fund.ID, err)
		nt.updateFundCache(fund.ID, fund.Name, currentNAV, "")
		return
	}

	log.Printf("Current NAV for %s: ₹%.3f (Date: %s)",
		fund.Name, currentNAV.Value, currentNAV.Timestamp.Format("02-Jan-2006"))

	shouldNotify := false
	if cachedNAV != nil {
		if currentNAV.Value != cachedNAV.Value || !currentNAV.Timestamp.Equal(cachedNAV.Timestamp) {
			shouldNotify = true
			log.Printf("NAV changed for %s: ₹%.3f → ₹%.3f",
				fund.Name, cachedNAV.Value, currentNAV.Value)
		} else {
			log.Printf("NAV unchanged for %s", fund.Name)
		}
	} else {
		log.Printf("First check for %s", fund.Name)
	}

	if shouldNotify && cachedNAV != nil {
		message := nt.formatMessage(fund, currentNAV, cachedNAV)
		if err := nt.sendTelegramMessage(message); err != nil {
			log.Printf("Error sending telegram message for %s: %v", fund.ID, err)
			nt.updateFundCache(fund.ID, fund.Name, currentNAV, fmt.Sprintf("Telegram error: %v", err))
		} else {
			log.Printf("Notification sent for %s", fund.Name)
			nt.updateFundCache(fund.ID, fund.Name, currentNAV, "")
		}
	} else {
		nt.updateFundCache(fund.ID, fund.Name, currentNAV, "")
	}
}

func (nt *NAVTracker) checkAllFunds() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in checkAllFunds: %v", r)
		}
	}()

	nt.mutex.RLock()
	funds := make([]FundInfo, len(nt.funds))
	copy(funds, nt.funds)
	nt.mutex.RUnlock()

	if len(funds) == 0 {
		log.Println("No funds to check")
		return
	}

	log.Printf("Starting check for %d funds", len(funds))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)

	for _, fund := range funds {
		wg.Add(1)
		go func(f FundInfo) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Panic in fund goroutine for %s: %v", f.ID, r)
				}
			}()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			nt.checkFund(f)
		}(fund)
	}

	wg.Wait()
	log.Println("Completed checking all funds")
}

func (nt *NAVTracker) reloadFunds() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in reloadFunds: %v", r)
		}
	}()

	if err := nt.loadFunds(); err != nil {
		log.Printf("Error reloading funds: %v", err)
	}
}

func (nt *NAVTracker) Start() {
	log.Println("Starting Multi-Fund NAV Tracker...")

	if err := nt.initDatabase(); err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer nt.db.Close()

	if err := nt.loadFunds(); err != nil {
		log.Fatalf("Loading funds failed: %v", err)
	}

	log.Printf("Monitoring %d funds with %v intervals", len(nt.funds), nt.config.CheckInterval)

	nt.checkAllFunds()

	checkTicker := time.NewTicker(nt.config.CheckInterval)
	reloadTicker := time.NewTicker(5 * time.Minute)
	defer checkTicker.Stop()
	defer reloadTicker.Stop()

	for {
		select {
		case <-checkTicker.C:
			nt.checkAllFunds()
		case <-reloadTicker.C:
			nt.reloadFunds()
		}
	}
}

func main() {
	config := Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		URLsFile:         "fund_urls.txt",
		DatabaseFile:     "nav_tracker.db",
		CheckInterval:    15 * time.Minute,
		RetryAttempts:    3,
		RetryDelay:       2 * time.Second,
		RequestTimeout:   30 * time.Second,
	}

	if config.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}
	if config.TelegramChatID == "" {
		log.Fatal("TELEGRAM_CHAT_ID environment variable is required")
	}

	if _, err := os.Stat(config.URLsFile); os.IsNotExist(err) {
		sampleContent := `https://coin.zerodha.com/mf/fund/INF846K01EW2/axis-elss-tax-saver-fund-direct-growth
https://coin.zerodha.com/mf/fund/INF090I01039/icici-prudential-bluechip-fund-direct-growth`
		if err := os.WriteFile(config.URLsFile, []byte(sampleContent), 0644); err != nil {
			log.Fatalf("Error creating URLs file: %v", err)
		}
		log.Fatalf("Created %s. Please add your fund URLs and restart", config.URLsFile)
	}

	tracker := NewNAVTracker(config)
	tracker.Start()
}
