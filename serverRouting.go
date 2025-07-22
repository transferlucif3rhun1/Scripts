// serverRouting.go
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iancoleman/orderedmap"
)

const (
	ServerAddress            = ":3050"
	HeadersHashURL           = "http://45.92.1.127:3054/hash?filename=routes.json"
	HeadersDownloadURL       = "http://45.92.1.127:3054/download?filename=routes.json"
	RepeatedStringMultiplier = 1024
	RequestTimeout           = 10 * time.Second
	ShutdownTimeout          = 5 * time.Second
	EncryptionKey            = "12345678901234567890123456789012"
	LocalRoutesFileEncrypted = "routes.json.enc"
	LocalHashFileEncrypted   = "routes.hash.enc"
	MonitorInterval          = 1 * time.Minute
)

type RequestPayload struct {
	Proxy    string `json:"proxy,omitempty"`
	Times    int    `json:"times,omitempty"`
	Random   bool   `json:"random"`
	Override *bool  `json:"override,omitempty"`
}

type HeadersResponsePayload struct {
	Headers        string `json:"headers"`
	Proxy          string `json:"proxy,omitempty"`
	RepeatedString string `json:"repeatedString,omitempty"`
}

type CheckResponse struct {
	Status string `json:"status"`
}

type Manager struct {
	headersValues       map[string]*orderedmap.OrderedMap
	defaultHeaders      *orderedmap.OrderedMap
	cachedMergedHeaders atomic.Value // map[string]map[string]*orderedmap.OrderedMap
	cachedHeadersJSON   atomic.Value // map[string]map[string]string
	hash                string
	mutex               sync.RWMutex
	jobQueue            chan func()
	wg                  sync.WaitGroup
	blockedWords        []string
}

func NewManager(queueSize int) *Manager {
	m := &Manager{
		headersValues:  make(map[string]*orderedmap.OrderedMap),
		defaultHeaders: orderedmap.New(),
		jobQueue:       make(chan func(), queueSize),
		blockedWords:   []string{},
	}
	m.cachedMergedHeaders.Store(make(map[string]map[string]*orderedmap.OrderedMap))
	m.cachedHeadersJSON.Store(make(map[string]map[string]string))
	return m
}

func Encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(EncryptionKey))
	if err != nil {
		return nil, fmt.Errorf("cipher initialization failed: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher mode setup failed: %w", err)
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(cryptoRand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation failed: %w", err)
	}
	ciphertext := aesGCM.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher([]byte(EncryptionKey))
	if err != nil {
		return nil, fmt.Errorf("cipher initialization failed: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher mode setup failed: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("data length insufficient")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("data decryption failed: %w", err)
	}
	return plaintext, nil
}

func ComputeSHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func LoadAndDecrypt(filename string) ([]byte, error) {
	encryptedData, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %s: %w", filename, err)
	}
	decryptedData, err := Decrypt(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("unable to process file %s: %w", filename, err)
	}
	return decryptedData, nil
}

func EncryptAndSave(data []byte, filename string) error {
	encryptedData, err := Encrypt(data)
	if err != nil {
		return fmt.Errorf("data processing failed: %w", err)
	}
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, encryptedData, 0644); err != nil {
		return fmt.Errorf("file write operation failed: %w", err)
	}
	if err := os.Rename(tempFile, filename); err != nil {
		return fmt.Errorf("file renaming failed: %w", err)
	}
	return nil
}

func getOrderedMap(data interface{}) (*orderedmap.OrderedMap, error) {
	switch v := data.(type) {
	case *orderedmap.OrderedMap:
		return v, nil
	case orderedmap.OrderedMap:
		return &v, nil
	case map[string]interface{}:
		om := orderedmap.New()
		for key, value := range v {
			om.Set(key, value)
		}
		return om, nil
	default:
		return nil, fmt.Errorf("unexpected data type: %T", data)
	}
}

func (m *Manager) FetchAndStoreRoutes(remoteHash, localRoutesFile, localHashFile string, isUpdate bool) error {
	resp, err := http.Get(HeadersDownloadURL)
	if err != nil {
		return fmt.Errorf("unable to retrieve routes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response during routes retrieval: %s", resp.Status)
	}
	routesBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("data reading failed: %w", err)
	}
	var allRoutes orderedmap.OrderedMap
	if err := json.Unmarshal(routesBytes, &allRoutes); err != nil {
		return fmt.Errorf("data parsing failed: %w", err)
	}
	if err := m.extractHeaders(&allRoutes); err != nil {
		return fmt.Errorf("header extraction failed: %w", err)
	}
	m.mutex.Lock()
	m.hash = remoteHash
	m.mutex.Unlock()
	if err := EncryptAndSave(routesBytes, localRoutesFile); err != nil {
		return fmt.Errorf("data saving failed: %w", err)
	}
	hashBytesToSave := []byte(m.hash)
	if err := EncryptAndSave(hashBytesToSave, localHashFile); err != nil {
		return fmt.Errorf("hash saving failed: %w", err)
	}
	if isUpdate {
		log.Println("Routes have been successfully updated.")
	} else {
		log.Println("Routes have been successfully fetched.")
	}
	return nil
}

func (m *Manager) extractHeaders(allRoutes *orderedmap.OrderedMap) error {
	defaultHeadersRaw, exists := allRoutes.Get("defaultheaders")
	if !exists {
		return fmt.Errorf("default headers missing")
	}
	defaultHeadersMap, err := getOrderedMap(defaultHeadersRaw)
	if err != nil {
		return fmt.Errorf("invalid default headers format: %w", err)
	}
	defaultHeaders := orderedmap.New()
	for _, key := range defaultHeadersMap.Keys() {
		value, _ := defaultHeadersMap.Get(key)
		defaultHeaders.Set(key, value)
	}
	allRoutes.Delete("defaultheaders")

	blockedWordsRaw, exists := allRoutes.Get("blockedwords")
	if exists {
		switch bw := blockedWordsRaw.(type) {
		case []interface{}:
			tempBlockedWords := make([]string, 0, len(bw))
			for _, word := range bw {
				if str, ok := word.(string); ok {
					tempBlockedWords = append(tempBlockedWords, str)
				}
			}
			m.mutex.Lock()
			m.blockedWords = tempBlockedWords
			m.mutex.Unlock()
		case []string:
			m.mutex.Lock()
			m.blockedWords = bw
			m.mutex.Unlock()
		}
	}
	allRoutes.Delete("blockedwords")

	normalizedRoutes := make(map[string]*orderedmap.OrderedMap)
	for _, key := range allRoutes.Keys() {
		lowerRouteType := strings.ToLower(key)
		headersRaw, exists := allRoutes.Get(key)
		if !exists {
			continue
		}
		headersMap, err := getOrderedMap(headersRaw)
		if err != nil {
			return fmt.Errorf("invalid headers format for route type: %w", err)
		}
		headersOrderedMap := orderedmap.New()
		for _, headerKey := range headersMap.Keys() {
			value, _ := headersMap.Get(headerKey)
			headersOrderedMap.Set(headerKey, value)
		}
		normalizedRoutes[lowerRouteType] = headersOrderedMap
	}

	newCachedMergedHeaders := make(map[string]map[string]*orderedmap.OrderedMap)
	newCachedHeadersJSON := make(map[string]map[string]string)

	for routeType, specificHeaders := range normalizedRoutes {
		newCachedMergedHeaders[routeType] = make(map[string]*orderedmap.OrderedMap)
		newCachedHeadersJSON[routeType] = make(map[string]string)
		overriddenHeaders := mergeHeaders(specificHeaders, defaultHeaders, true)
		newCachedMergedHeaders[routeType]["true"] = overriddenHeaders

		normalHeaders := orderedmap.New()
		for _, key := range specificHeaders.Keys() {
			value, _ := specificHeaders.Get(key)
			normalHeaders.Set(key, value)
		}
		newCachedMergedHeaders[routeType]["false"] = normalHeaders

		for overrideKey, headersMap := range map[string]*orderedmap.OrderedMap{"true": overriddenHeaders, "false": normalHeaders} {
			headersJSONBytes, err := json.Marshal(headersMap)
			if err != nil {
				return fmt.Errorf("serialization failed for route type and override: %w", err)
			}
			headersJSONString := string(headersJSONBytes)
			headersJSONString = strings.ReplaceAll(headersJSONString, "\\u003c", "<")
			headersJSONString = strings.ReplaceAll(headersJSONString, "\\u003e", ">")
			headersJSONString = strings.TrimSpace(headersJSONString)
			newCachedHeadersJSON[routeType][overrideKey] = headersJSONString
		}
	}

	m.mutex.Lock()
	m.headersValues = normalizedRoutes
	m.defaultHeaders = defaultHeaders
	m.mutex.Unlock()
	m.cachedMergedHeaders.Store(newCachedMergedHeaders)
	m.cachedHeadersJSON.Store(newCachedHeadersJSON)
	return nil
}

func mergeHeaders(specific, defaults *orderedmap.OrderedMap, override bool) *orderedmap.OrderedMap {
	merged := orderedmap.New()
	defaultKeys := make(map[string]bool)
	for _, key := range defaults.Keys() {
		defaultKeys[key] = true
	}
	for _, key := range specific.Keys() {
		value, _ := specific.Get(key)
		if override {
			if defaultKeys[key] {
				value, _ = defaults.Get(key)
			}
		}
		merged.Set(key, value)
	}
	return merged
}

func (m *Manager) LoadHeaders() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, HeadersHashURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Server request failed, attempting to load local files...")
		return m.loadLocalHeaders()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return m.FetchAndStoreRoutes("unknown", LocalRoutesFileEncrypted, LocalHashFileEncrypted, false)
	}
	remoteHashBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return m.FetchAndStoreRoutes("unknown", LocalRoutesFileEncrypted, LocalHashFileEncrypted, false)
	}
	remoteHash := strings.TrimSpace(string(remoteHashBytes))
	remoteHash = strings.ToLower(remoteHash)

	decryptedLocalHashBytes, err := LoadAndDecrypt(LocalHashFileEncrypted)
	if err != nil {
		return m.FetchAndStoreRoutes(remoteHash, LocalRoutesFileEncrypted, LocalHashFileEncrypted, false)
	}
	localHash := strings.TrimSpace(string(decryptedLocalHashBytes))
	localHash = strings.ToLower(localHash)

	if localHash == remoteHash {
		// Load from local if hashes match
		return m.loadLocalHeaders()
	} else {
		// Fetch and store if hashes do not match
		if err := m.FetchAndStoreRoutes(remoteHash, LocalRoutesFileEncrypted, LocalHashFileEncrypted, true); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) loadLocalHeaders() error {
	decryptedRoutes, err := LoadAndDecrypt(LocalRoutesFileEncrypted)
	if err != nil {
		return fmt.Errorf("failed to load local routes: %w", err)
	}
	var allRoutes orderedmap.OrderedMap
	if err := json.Unmarshal(decryptedRoutes, &allRoutes); err != nil {
		return fmt.Errorf("failed to parse local routes: %w", err)
	}
	if err := m.extractHeaders(&allRoutes); err != nil {
		return fmt.Errorf("header extraction failed: %w", err)
	}
	decryptedLocalHashBytes, err := LoadAndDecrypt(LocalHashFileEncrypted)
	if err != nil {
		return fmt.Errorf("failed to load local hash: %w", err)
	}
	localHash := strings.TrimSpace(string(decryptedLocalHashBytes))
	m.mutex.Lock()
	m.hash = localHash
	m.mutex.Unlock()
	log.Println("Routes loaded from local files.")
	return nil
}

func (m *Manager) StartWorkerPool() {
	workerCount := runtime.NumCPU()
	for i := 0; i < workerCount; i++ {
		go m.worker()
	}
}

func (m *Manager) worker() {
	for job := range m.jobQueue {
		job()
		m.wg.Done()
	}
}

func (m *Manager) SubmitJob(job func()) {
	m.wg.Add(1)
	m.jobQueue <- job
}

func (m *Manager) ShutdownWorkerPool() {
	close(m.jobQueue)
	m.wg.Wait()
}

func HandleError(c *gin.Context, err error, statusCode int, message string) {
	c.JSON(statusCode, gin.H{"error": message})
	c.Abort()
}

func HandleCheckError(c *gin.Context, err error, statusCode int, message string) {
	c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
	c.Abort()
}

func RandomChoice(choices []string) string {
	return choices[rand.Intn(len(choices))]
}

func GenerateRandomToken() string {
	b := make([]byte, 16)
	if _, err := cryptoRand.Read(b); err != nil {
		return "randomtoken"
	}
	return fmt.Sprintf("%x", b)
}

func GenerateRandomDate() string {
	t := time.Now().Add(-time.Duration(rand.Intn(100*24)) * time.Hour)
	return t.UTC().Format(http.TimeFormat)
}

func GenerateRandomHTTPMethods(count int) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH", "HEAD"}
	chosenMethods := make([]string, count)
	for i := 0; i < count; i++ {
		chosenMethods[i] = RandomChoice(methods)
	}
	return strings.Join(chosenMethods, ", ")
}

func GenerateHeadersWithRandomValuesOrdered(baseHeaders *orderedmap.OrderedMap) *orderedmap.OrderedMap {
	headers := orderedmap.New()
	for _, key := range baseHeaders.Keys() {
		value, _ := baseHeaders.Get(key)
		headers.Set(key, value)
	}
	return headers
}

func ParseProxy(proxy string) (string, error) {
	// Try to parse the proxy URL
	u, err := url.Parse(proxy)
	if err != nil {
		return "", fmt.Errorf("invalid proxy format")
	}
	return u.String(), nil
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func ConvertCookiesFromMap(cookies map[string]string) []Cookie {
	result := make([]Cookie, 0, len(cookies))
	for name, value := range cookies {
		if name != "" && value != "" {
			result = append(result, Cookie{Name: name, Value: value})
		}
	}
	return result
}

func ConvertBracketedCookies(payload string) string {
	if len(payload) < 2 {
		return ""
	}
	content := payload[1 : len(payload)-1]
	pairs := strings.Split(content, "), (")
	var result []string
	for _, pair := range pairs {
		parts := strings.Split(pair, ", ")
		if len(parts) != 2 {
			continue
		}
		name := strings.Trim(parts[0], "\"() ")
		value := strings.Trim(parts[1], "\"() ")
		result = append(result, fmt.Sprintf("%s=%s", name, value))
	}
	return strings.Join(result, "; ")
}

func generateRandomHeaders() map[string]string {
	return map[string]string{
		"x-request-id":                  GenerateRandomToken(),
		"x-correlation-id":              GenerateRandomToken(),
		"save-data":                     "Yes",
		"upgrade-insecure-requests":     "1",
		"content-security-policy":       "default-src 'self'",
		"te":                            "Trailers",
		"trailer":                       "Expires",
		"prefer":                        "return=representation",
		"pragma":                        "no-cache",
		"if-unmodified-since":           GenerateRandomDate(),
		"if-range":                      GenerateRandomDate(),
		"if-none-match":                 GenerateRandomToken(),
		"if-match":                      GenerateRandomToken(),
		"expect":                        "100-continue",
		"access-control-request-method": GenerateRandomHTTPMethods(rand.Intn(3) + 1),
		"x-uidh":                        GenerateRandomToken(),
		"last-modified":                 GenerateRandomDate(),
		"vary":                          "Accept-Encoding",
		"x-powered-by":                  RandomChoice([]string{"PHP/7.4.3", "Apache/2.4.41", "Nginx/1.18.0", "Express/4.17.1"}),
		"x-redirect-by":                 RandomChoice([]string{"WordPress", "Nginx", "Apache", "URL redirection"}),
		"expires":                       GenerateRandomDate(),
		"x-csrf-token":                  GenerateRandomToken(),
		"x-xsrf-token":                  GenerateRandomToken(),
		"sec-gpc":                       "1",
		"allow":                         GenerateRandomHTTPMethods(rand.Intn(3) + 1),
		"dnt":                           "1",
		"via":                           "1.1 varnish",
		"warning":                       "110 - Response is Stale",
	}
}

func (m *Manager) GenerateHeadersHandler(c *gin.Context) {
	headerType := strings.ToLower(c.Param("type"))
	var payload RequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil && err.Error() != "EOF" {
		HandleError(c, err, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	override := true
	if payload.Override != nil {
		override = *payload.Override
	}
	overrideKey := "true"
	if !override {
		overrideKey = "false"
	}
	if !payload.Random {
		currentJSONCache, ok := m.cachedHeadersJSON.Load().(map[string]map[string]string)
		if !ok {
			HandleError(c, fmt.Errorf("invalid cache format"), http.StatusInternalServerError, "Internal server error")
			return
		}
		headerTypeCache, exists := currentJSONCache[headerType]
		if !exists {
			HandleError(c, fmt.Errorf("header values not found for type %s", headerType), http.StatusBadRequest, "Header values not found")
			return
		}
		headersJSONString, exists := headerTypeCache[overrideKey]
		if !exists {
			HandleError(c, fmt.Errorf("headers not found for override=%v", override), http.StatusBadRequest, "Headers configuration not found")
			return
		}
		responseMap := map[string]interface{}{
			"headers": headersJSONString,
		}
		if payload.Proxy != "" {
			proxy, err := ParseProxy(payload.Proxy)
			if err != nil {
				HandleError(c, err, http.StatusBadRequest, err.Error())
				return
			}
			responseMap["proxy"] = proxy
		}
		if payload.Times > 0 {
			repeatedStr := strings.Repeat("-", payload.Times*RepeatedStringMultiplier)
			responseMap["repeatedString"] = repeatedStr
		}
		responseJSONBytes, err := json.Marshal(responseMap)
		if err != nil {
			HandleError(c, err, http.StatusInternalServerError, "Failed to serialize response")
			return
		}
		responseJSONString := string(responseJSONBytes)
		responseJSONString = strings.ReplaceAll(responseJSONString, "\\u003c", "<")
		responseJSONString = strings.ReplaceAll(responseJSONString, "\\u003e", ">")
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, responseJSONString)
		return
	}
	currentCache, ok := m.cachedMergedHeaders.Load().(map[string]map[string]*orderedmap.OrderedMap)
	if !ok {
		HandleError(c, fmt.Errorf("invalid cache format"), http.StatusInternalServerError, "Internal server error")
		return
	}
	routeHeadersMap, exists := currentCache[headerType]
	if !exists {
		HandleError(c, fmt.Errorf("header values not found for type %s", headerType), http.StatusBadRequest, "Header values not found")
		return
	}
	mergedHeaders, exists := routeHeadersMap[overrideKey]
	if !exists {
		HandleError(c, fmt.Errorf("headers not found for override=%v", override), http.StatusBadRequest, "Headers configuration not found")
		return
	}
	mergedHeaders = GenerateHeadersWithRandomValuesOrdered(mergedHeaders)
	for name, value := range generateRandomHeaders() {
		mergedHeaders.Set(name, value)
	}
	headersJSONBytes, err := json.Marshal(mergedHeaders)
	if err != nil {
		HandleError(c, err, http.StatusInternalServerError, "Failed to serialize headers")
		return
	}
	headersJSONString := string(headersJSONBytes)
	headersJSONString = strings.ReplaceAll(headersJSONString, "\\u003c", "<")
	headersJSONString = strings.ReplaceAll(headersJSONString, "\\u003e", ">")
	headersJSONString = strings.TrimSpace(headersJSONString)
	responseMap := map[string]interface{}{
		"headers": headersJSONString,
	}
	if payload.Proxy != "" {
		proxy, err := ParseProxy(payload.Proxy)
		if err != nil {
			HandleError(c, err, http.StatusBadRequest, err.Error())
			return
		}
		responseMap["proxy"] = proxy
	}
	if payload.Times > 0 {
		repeatedStr := strings.Repeat("-", payload.Times*RepeatedStringMultiplier)
		responseMap["repeatedString"] = repeatedStr
	}
	responseJSONBytesFinal, err := json.Marshal(responseMap)
	if err != nil {
		HandleError(c, err, http.StatusInternalServerError, "Failed to serialize response")
		return
	}
	responseJSONStringFinal := string(responseJSONBytesFinal)
	responseJSONStringFinal = strings.ReplaceAll(responseJSONStringFinal, "\\u003c", "<")
	responseJSONStringFinal = strings.ReplaceAll(responseJSONStringFinal, "\\u003e", ">")
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, responseJSONStringFinal)
}

func (m *Manager) ConvertCookiesHandler(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		HandleError(c, fmt.Errorf("invalid request method"), http.StatusMethodNotAllowed, "Invalid request method")
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		HandleError(c, err, http.StatusBadRequest, "Failed to read request body")
		return
	}
	payloadString := strings.TrimSpace(string(body))
	if payloadString == "" {
		HandleError(c, fmt.Errorf("empty payload"), http.StatusBadRequest, "Payload cannot be empty")
		return
	}
	var result interface{}
	if strings.HasPrefix(payloadString, "{") && strings.Contains(payloadString, "(") {
		result = ConvertBracketedCookies(payloadString)
	} else if strings.HasPrefix(payloadString, "{") {
		var cookies map[string]string
		if err := json.Unmarshal([]byte(payloadString), &cookies); err == nil {
			result = ConvertCookiesFromMap(cookies)
		} else {
			result = payloadString
		}
	} else {
		result = payloadString
	}
	if strings.TrimSpace(fmt.Sprintf("%v", result)) == "" {
		HandleError(c, fmt.Errorf("empty payload after processing"), http.StatusBadRequest, "Payload cannot be empty")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (m *Manager) CheckHandler(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		HandleCheckError(c, fmt.Errorf("invalid request method"), http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if c.Request.ContentLength == 0 {
		c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
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
	var textToCheck string
	ls := `"body":`
	rs := `,"`
	lsIndex := strings.Index(requestBody, ls)
	if lsIndex != -1 {
		start := lsIndex + len(ls)
		rsIndex := strings.Index(requestBody[start:], rs)
		if rsIndex != -1 {
			textToCheck = requestBody[start : start+rsIndex]
		} else {
			textToCheck = requestBody[start:]
		}
	} else {
		textToCheck = requestBody
	}
	textToCheck = strings.TrimSpace(textToCheck)
	m.mutex.RLock()
	blockedWords := make([]string, len(m.blockedWords))
	copy(blockedWords, m.blockedWords)
	m.mutex.RUnlock()
	if len(blockedWords) == 0 {
		c.String(http.StatusOK, requestBody)
		return
	}
	for _, word := range blockedWords {
		if word == strings.ToLower(word) {
			if strings.Contains(strings.ToLower(textToCheck), word) {
				c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
				return
			}
		} else {
			if strings.Contains(textToCheck, word) {
				c.JSON(http.StatusOK, CheckResponse{Status: "retry"})
				return
			}
		}
	}
	c.String(http.StatusOK, requestBody)
}

func HealthCheck(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

func (m *Manager) SetupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/generateHeaders/:type", m.GenerateHeadersHandler)
	router.POST("/convertCookies", m.ConvertCookiesHandler)
	router.POST("/check/", m.CheckHandler)
	router.GET("/health", HealthCheck)
	return router
}

func (m *Manager) MonitorHashes() {
	ticker := time.NewTicker(MonitorInterval)
	defer ticker.Stop()
	for range ticker.C {
		resp, err := http.Get(HeadersHashURL)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		remoteHashBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		remoteHash := strings.TrimSpace(string(remoteHashBytes))
		remoteHash = strings.ToLower(remoteHash)
		m.mutex.RLock()
		currentHash := strings.ToLower(m.hash)
		m.mutex.RUnlock()
		if remoteHash != currentHash {
			m.FetchAndStoreRoutes(remoteHash, LocalRoutesFileEncrypted, LocalHashFileEncrypted, true)
		}
	}
}

func main() {
	log.SetFlags(0)
	rand.Seed(time.Now().UnixNano())
	manager := NewManager(100)
	if err := manager.LoadHeaders(); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
	manager.StartWorkerPool()
	defer manager.ShutdownWorkerPool()
	go manager.MonitorHashes()
	log.Println("Server routing has started.")
	log.Println("Version: 1.0.0")
	router := manager.SetupRouter()
	srv := &http.Server{
		Addr:         ServerAddress,
		Handler:      router,
		ReadTimeout:  RequestTimeout,
		WriteTimeout: RequestTimeout,
		IdleTimeout:  30 * time.Second,
	}
	serverShutdown := make(chan struct{})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server encountered an issue: %v", err)
		}
		close(serverShutdown)
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("Initiating server shutdown...")
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown encountered an issue: %v", err)
	}
	<-serverShutdown
	log.Println("Server has shut down gracefully.")
}
