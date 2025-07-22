package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math"
	"math/big"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProxyConfig struct {
	Server   string `json:"server"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Type     string `json:"type,omitempty"` // http, socks4, socks5
}

type CookieRequest struct {
	URL         string      `json:"url" binding:"required"`
	Proxy       interface{} `json:"proxy,omitempty"`
	WaitTime    int         `json:"wait_time,omitempty"`
	Timeout     int         `json:"timeout,omitempty"`
	Verbose     bool        `json:"verbose,omitempty"`
	Headless    *bool       `json:"headless,omitempty"`
	RetryCount  int         `json:"retry_count,omitempty"`
	NetworkIdle int         `json:"network_idle,omitempty"`
	Browser     string      `json:"browser,omitempty"`      // "chrome", "brave"
	BrowserPath string      `json:"browser_path,omitempty"` // Custom browser executable path
}

type NavigationInfo struct {
	FinalURL          string   `json:"final_url"`
	RedirectChain     []string `json:"redirect_chain"`
	LoadTime          int64    `json:"load_time_ms"`
	StatusCode        int      `json:"status_code"`
	ContentType       string   `json:"content_type,omitempty"`
	ResourceCount     int      `json:"resource_count"`
	JavaScriptEnabled bool     `json:"javascript_enabled"`
}

type CookieResponse struct {
	Success        bool              `json:"success"`
	Cookies        map[string]string `json:"cookies"`
	Message        string            `json:"message"`
	NavigationInfo *NavigationInfo   `json:"navigation_info,omitempty"`
	DOMContent     string            `json:"dom_content,omitempty"`
	RequestID      string            `json:"request_id"`
	Timing         map[string]int64  `json:"timing,omitempty"`
	RetryAttempts  int               `json:"retry_attempts,omitempty"`
	Fingerprint    map[string]string `json:"fingerprint,omitempty"`
}

type Logger struct {
	verbose   bool
	requestID string
}

func NewLogger(verbose bool, requestID string) *Logger {
	return &Logger{verbose: verbose, requestID: requestID}
}

func (l *Logger) Info(format string, args ...interface{}) {
	if l.verbose {
		log.Printf("[INFO][%s] %s", l.requestID, fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.verbose {
		log.Printf("[DEBUG][%s] %s", l.requestID, fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR][%s] %s", l.requestID, fmt.Sprintf(format, args...))
}

type BrowserFingerprint struct {
	UserAgent         string
	Platform          string
	Vendor            string
	Language          string
	Timezone          string
	ScreenWidth       int
	ScreenHeight      int
	ColorDepth        int
	PixelRatio        float64
	HardwareCores     int
	MaxTouchPoints    int
	WebRTCIPHandling  string
	CanvasFingerprint string
	WebGLVendor       string
	WebGLRenderer     string
	AudioFingerprint  string
	FontFingerprint   string
	CPUClass          string
	DeviceMemory      int
	NavigatorPlatform string
}

type BrowserPool struct {
	contexts         []context.Context
	cancels          []context.CancelFunc
	semaphore        chan struct{}
	mu               sync.RWMutex
	maxContexts      int
	proxyConfigs     map[string]*ProxyConfig
	fingerprintCache sync.Map
}

func NewBrowserPool(maxContexts int) *BrowserPool {
	return &BrowserPool{
		contexts:     make([]context.Context, 0, maxContexts),
		cancels:      make([]context.CancelFunc, 0, maxContexts),
		semaphore:    make(chan struct{}, maxContexts),
		maxContexts:  maxContexts,
		proxyConfigs: make(map[string]*ProxyConfig),
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func createBrowserOptions(fingerprint *BrowserFingerprint, headless bool, browserType string, customPath string) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", headless),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-web-security", false),
		chromedp.Flag("disable-features", "VizDisplayCompositor,TranslateUI,WebRtcHideLocalIpsWithMdns,WebRtcLogCapturer,MediaRouter"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("disable-plugins", false),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("memory-pressure-off", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("exclude-switches", "enable-automation"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("force-webrtc-ip-handling-policy", fingerprint.WebRTCIPHandling),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceLogging"),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-features", "TranslateUI,VizDisplayCompositor"),
		chromedp.WindowSize(fingerprint.ScreenWidth, fingerprint.ScreenHeight),
		chromedp.UserAgent(fingerprint.UserAgent),
		chromedp.Flag("intl.accept_languages", fingerprint.Language),
		chromedp.Flag("timezone", fingerprint.Timezone),
	}

	// Handle custom browser path first
	if customPath != "" {
		log.Printf("[DEBUG] Using custom browser path: %s", customPath)
		if pathExists(customPath) {
			log.Printf("[DEBUG] ✅ Custom browser path exists")
			opts = append(opts, chromedp.ExecPath(customPath))
			return opts
		} else {
			log.Printf("[ERROR] ❌ Custom browser path does not exist: %s", customPath)
		}
	}

	// Add browser-specific executable path
	if browserType == "brave" {
		log.Printf("[DEBUG] Attempting to locate Brave browser...")

		// Common Brave browser paths
		bravePaths := []string{
			"C:\\Program Files\\BraveSoftware\\Brave-Browser\\Application\\brave.exe",                                        // Windows
			"C:\\Program Files (x86)\\BraveSoftware\\Brave-Browser\\Application\\brave.exe",                                  // Windows 32-bit
			"C:\\Users\\" + os.Getenv("USERNAME") + "\\AppData\\Local\\BraveSoftware\\Brave-Browser\\Application\\brave.exe", // Windows User
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",                                                   // macOS
			"/usr/bin/brave-browser", // Linux
			"/usr/bin/brave",         // Linux alt
			"/snap/bin/brave",        // Linux snap
			"/var/lib/flatpak/exports/bin/com.brave.Browser",                               // Linux flatpak
			"/opt/brave.com/brave/brave",                                                   // Linux opt
			"C:\\Program Files\\BraveSoftware\\Brave-Browser-Beta\\Application\\brave.exe", // Windows Beta
			"C:\\Program Files\\BraveSoftware\\Brave-Browser-Dev\\Application\\brave.exe",  // Windows Dev
		}

		braveFound := false
		for _, path := range bravePaths {
			log.Printf("[DEBUG] Checking Brave path: %s", path)
			if pathExists(path) {
				log.Printf("[DEBUG] ✅ Found Brave browser at: %s", path)
				opts = append(opts, chromedp.ExecPath(path))
				braveFound = true
				break
			}
		}

		if !braveFound {
			log.Printf("[ERROR] ❌ Brave browser not found in any expected locations!")
			log.Printf("[INFO] Will fallback to Chrome. To use Brave, install it or specify custom path.")
			log.Printf("[INFO] Expected locations checked: %v", bravePaths)
		}
	} else {
		log.Printf("[DEBUG] Using default browser: %s", browserType)
	}

	return opts
}

func (bp *BrowserPool) Initialize() error {
	for i := 0; i < bp.maxContexts; i++ {
		fingerprint := generateAdvancedFingerprint()
		opts := createBrowserOptions(fingerprint, true, "chrome", "")

		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
		bp.cancels = append(bp.cancels, cancel)

		ctx, cancel := chromedp.NewContext(allocCtx)
		bp.cancels = append(bp.cancels, cancel)
		bp.contexts = append(bp.contexts, ctx)

		bp.semaphore <- struct{}{}
	}
	return nil
}

func (bp *BrowserPool) GetContext() (context.Context, func()) {
	<-bp.semaphore
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if len(bp.contexts) == 0 {
		fingerprint := generateAdvancedFingerprint()
		opts := createBrowserOptions(fingerprint, true, "chrome", "")

		allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
		ctx, ctxCancel := chromedp.NewContext(allocCtx)

		bp.cancels = append(bp.cancels, cancel)
		bp.cancels = append(bp.cancels, ctxCancel)
		bp.contexts = append(bp.contexts, ctx)
	}

	ctx := bp.contexts[0]
	bp.contexts = bp.contexts[1:]

	return ctx, func() {
		bp.mu.Lock()
		bp.contexts = append(bp.contexts, ctx)
		bp.mu.Unlock()
		bp.semaphore <- struct{}{}
	}
}

func (bp *BrowserPool) GetContextWithProxy(proxyConfig *ProxyConfig, headless bool, browserType string, customPath string) (context.Context, context.CancelFunc, func(), *BrowserFingerprint) {
	fingerprint := generateAdvancedFingerprint()
	opts := createBrowserOptions(fingerprint, headless, browserType, customPath)

	if proxyConfig != nil {
		switch proxyConfig.Type {
		case "socks4", "socks5":
			opts = append(opts, chromedp.ProxyServer(fmt.Sprintf("%s://%s", proxyConfig.Type, proxyConfig.Server)))
		default:
			opts = append(opts, chromedp.ProxyServer(proxyConfig.Server))
		}
	}

	<-bp.semaphore

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	return ctx, func() {
			ctxCancel()
			allocCancel()
		}, func() {
			bp.semaphore <- struct{}{}
			runtime.GC()
		}, fingerprint
}

func (bp *BrowserPool) Cleanup() {
	for _, cancel := range bp.cancels {
		cancel()
	}
}

func generateAdvancedFingerprint() *BrowserFingerprint {
	userAgent := generateRandomUserAgent()
	platform := extractPlatformFromUA(userAgent)

	return &BrowserFingerprint{
		UserAgent:         userAgent,
		Platform:          platform,
		Vendor:            generateRandomVendor(),
		Language:          generateRandomLanguage(),
		Timezone:          generateRandomTimezone(),
		ScreenWidth:       generateRandomScreenWidth(),
		ScreenHeight:      generateRandomScreenHeight(),
		ColorDepth:        generateRandomColorDepth(),
		PixelRatio:        generateRandomPixelRatio(),
		HardwareCores:     generateRandomHardwareCores(),
		MaxTouchPoints:    generateRandomTouchPoints(),
		WebRTCIPHandling:  "default_public_interface_only",
		CanvasFingerprint: generateRandomCanvasFingerprint(),
		WebGLVendor:       generateRandomWebGLVendor(),
		WebGLRenderer:     generateRandomWebGLRenderer(),
		AudioFingerprint:  generateRandomAudioFingerprint(),
		FontFingerprint:   generateRandomFontFingerprint(),
		CPUClass:          generateRandomCPUClass(),
		DeviceMemory:      generateRandomDeviceMemory(),
		NavigatorPlatform: generateRandomNavigatorPlatform(platform),
	}
}

func parseProxy(proxyInput interface{}) *ProxyConfig {
	if proxyInput == nil {
		return nil
	}

	switch v := proxyInput.(type) {
	case string:
		proxyStr := strings.TrimSpace(v)
		if proxyStr == "" {
			return nil
		}
		if u, err := url.Parse(proxyStr); err == nil {
			config := &ProxyConfig{Server: u.Host, Type: "http"}
			if u.Scheme != "" {
				config.Type = u.Scheme
			}
			if u.User != nil {
				config.Username = u.User.Username()
				if pass, hasPass := u.User.Password(); hasPass {
					config.Password = pass
				}
			}
			return config
		}

	case map[string]interface{}:
		server, _ := v["server"].(string)
		if server == "" {
			return nil
		}
		config := &ProxyConfig{Server: server, Type: "http"}
		if proxyType, ok := v["type"].(string); ok {
			config.Type = proxyType
		}
		if username, ok := v["username"].(string); ok {
			config.Username = username
		}
		if password, ok := v["password"].(string); ok {
			config.Password = password
		}
		return config
	}

	return nil
}

func injectAdvancedFingerprintingResistance(ctx context.Context, fingerprint *BrowserFingerprint, logger *Logger) error {
	logger.Debug("Injecting advanced fingerprinting resistance scripts")

	scripts := []string{
		fmt.Sprintf(`
			Object.defineProperty(navigator, 'platform', {
				get: () => '%s'
			});
		`, fingerprint.NavigatorPlatform),

		fmt.Sprintf(`
			Object.defineProperty(navigator, 'vendor', {
				get: () => '%s'
			});
		`, fingerprint.Vendor),

		fmt.Sprintf(`
			Object.defineProperty(navigator, 'hardwareConcurrency', {
				get: () => %d
			});
		`, fingerprint.HardwareCores),

		fmt.Sprintf(`
			Object.defineProperty(navigator, 'maxTouchPoints', {
				get: () => %d
			});
		`, fingerprint.MaxTouchPoints),

		fmt.Sprintf(`
			Object.defineProperty(screen, 'colorDepth', {
				get: () => %d
			});
		`, fingerprint.ColorDepth),

		fmt.Sprintf(`
			Object.defineProperty(window, 'devicePixelRatio', {
				get: () => %f
			});
		`, fingerprint.PixelRatio),

		fmt.Sprintf(`
			Object.defineProperty(navigator, 'deviceMemory', {
				get: () => %d
			});
		`, fingerprint.DeviceMemory),

		`
			Object.defineProperty(navigator, 'webdriver', {
				get: () => undefined
			});
		`,

		`
			window.chrome = {
				runtime: {},
				loadTimes: function() {},
				csi: function() {},
				app: {}
			};
		`,

		`
			const originalQuery = window.navigator.permissions.query;
			window.navigator.permissions.query = (parameters) => (
				parameters.name === 'notifications' ?
					Promise.resolve({ state: Notification.permission }) :
					originalQuery(parameters)
			);
		`,

		fmt.Sprintf(`
			const getParameter = WebGLRenderingContext.prototype.getParameter;
			WebGLRenderingContext.prototype.getParameter = function(parameter) {
				if (parameter === 37445) {
					return '%s';
				}
				if (parameter === 37446) {
					return '%s';
				}
				return getParameter.call(this, parameter);
			};
		`, fingerprint.WebGLVendor, fingerprint.WebGLRenderer),

		`
			const getContext = HTMLCanvasElement.prototype.getContext;
			HTMLCanvasElement.prototype.getContext = function(contextType, ...args) {
				if (contextType === '2d') {
					const context = getContext.call(this, contextType, ...args);
					const originalGetImageData = context.getImageData;
					context.getImageData = function(...args) {
						const imageData = originalGetImageData.apply(this, args);
						for (let i = 0; i < imageData.data.length; i += 4) {
							imageData.data[i] += Math.floor(Math.random() * 3) - 1;
							imageData.data[i + 1] += Math.floor(Math.random() * 3) - 1;
							imageData.data[i + 2] += Math.floor(Math.random() * 3) - 1;
						}
						return imageData;
					};
					return context;
				}
				return getContext.call(this, contextType, ...args);
			};
		`,

		`
			try {
				const audioContext = new (window.AudioContext || window.webkitAudioContext)();
				const originalGetFloatFrequencyData = AnalyserNode.prototype.getFloatFrequencyData;
				AnalyserNode.prototype.getFloatFrequencyData = function(array) {
					originalGetFloatFrequencyData.call(this, array);
					for (let i = 0; i < array.length; i++) {
						array[i] += Math.random() * 0.001 - 0.0005;
					}
				};
			} catch (e) {}
		`,
	}

	for _, script := range scripts {
		var result interface{}
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
			logger.Debug("Failed to inject script: %v", err)
		}
	}

	return nil
}

func waitForNavigationComplete(ctx context.Context, logger *Logger, networkIdleTime int) error {
	logger.Debug("Waiting for comprehensive navigation completion...")

	if networkIdleTime == 0 {
		networkIdleTime = 2000
	}

	var resourceCount int

	return chromedp.Run(ctx,
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			idleTimer := time.NewTimer(time.Duration(networkIdleTime) * time.Millisecond)
			defer idleTimer.Stop()

			chromedp.ListenTarget(ctx, func(ev interface{}) {
				switch ev.(type) {
				case *network.EventRequestWillBeSent:
					resourceCount++
					idleTimer.Reset(time.Duration(networkIdleTime) * time.Millisecond)

				case *network.EventLoadingFinished:
					idleTimer.Reset(time.Duration(networkIdleTime) * time.Millisecond)

				case *network.EventLoadingFailed:
					idleTimer.Reset(time.Duration(networkIdleTime) * time.Millisecond)

				case *page.EventLoadEventFired:
					idleTimer.Reset(time.Duration(networkIdleTime) * time.Millisecond)
				}
			})

			maxWait := time.NewTimer(30 * time.Second)
			defer maxWait.Stop()

			for {
				select {
				case <-idleTimer.C:
					logger.Debug("Network idle detected after %d resources", resourceCount)
					return nil
				case <-maxWait.C:
					logger.Debug("Maximum wait time reached")
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var readyState string
			for attempts := 0; attempts < 10; attempts++ {
				if err := chromedp.Evaluate(`document.readyState`, &readyState).Do(ctx); err != nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				if readyState == "complete" {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}

			var jsComplete bool
			chromedp.Evaluate(`
				(function() {
					return document.readyState === 'complete' && 
						   (!window.jQuery || window.jQuery.active === 0) &&
						   (!window.Angular || window.Angular.element(document).injector().get('$http').pendingRequests.length === 0);
				})()
			`, &jsComplete).Do(ctx)

			if !jsComplete {
				time.Sleep(500 * time.Millisecond)
			}

			return nil
		}),
	)
}

func extractCookiesWithRetry(ctx context.Context, targetURL string, proxyConfig *ProxyConfig, waitTime, timeout, retryCount, networkIdle int, verbose bool, requestID string, headless bool, browserType string, customPath string) (*CookieResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			backoffTime := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			time.Sleep(backoffTime)
		}

		result, err := extractCookies(ctx, targetURL, proxyConfig, waitTime, timeout, networkIdle, verbose, requestID, headless, browserType, customPath, attempt)
		if err == nil && result.Success {
			result.RetryAttempts = attempt
			return result, nil
		}

		lastErr = err
		if result != nil && !result.Success {
			lastErr = fmt.Errorf("extraction failed: %s", result.Message)
		}
	}

	return &CookieResponse{
		Success:       false,
		Cookies:       make(map[string]string),
		Message:       fmt.Sprintf("Failed after %d attempts: %v", retryCount+1, lastErr),
		RequestID:     requestID,
		RetryAttempts: retryCount,
	}, lastErr
}

func extractCookies(ctx context.Context, targetURL string, proxyConfig *ProxyConfig, waitTime, timeout, networkIdle int, verbose bool, requestID string, headless bool, browserType string, customPath string, attempt int) (*CookieResponse, error) {
	logger := NewLogger(verbose, requestID)
	timing := make(map[string]int64)
	startTime := time.Now()

	logger.Info("Starting cookie extraction (attempt %d) for URL: %s", attempt+1, targetURL)
	logger.Info("Headless mode: %t, Browser: %s", headless, browserType)
	if customPath != "" {
		logger.Info("Custom browser path: %s", customPath)
	}
	if proxyConfig != nil {
		logger.Info("Using proxy: %s (%s)", proxyConfig.Server, proxyConfig.Type)
	}

	navInfo := &NavigationInfo{
		RedirectChain: []string{targetURL},
	}

	var fingerprint *BrowserFingerprint

	browserPool.mu.Lock()
	contextWithProxy, ctxCancel, release, fp := browserPool.GetContextWithProxy(proxyConfig, headless, browserType, customPath)
	fingerprint = fp
	browserPool.mu.Unlock()

	defer release()
	defer ctxCancel()

	// Create timeout context from the chromedp context
	timeoutCtx, cancel := context.WithTimeout(contextWithProxy, time.Duration(timeout)*time.Millisecond)
	defer cancel()

	fpMap := map[string]string{
		"user_agent":     fingerprint.UserAgent,
		"platform":       fingerprint.Platform,
		"screen_size":    fmt.Sprintf("%dx%d", fingerprint.ScreenWidth, fingerprint.ScreenHeight),
		"color_depth":    fmt.Sprintf("%d", fingerprint.ColorDepth),
		"hardware_cores": fmt.Sprintf("%d", fingerprint.HardwareCores),
		"device_memory":  fmt.Sprintf("%d", fingerprint.DeviceMemory),
		"webgl_vendor":   fingerprint.WebGLVendor,
		"webgl_renderer": fingerprint.WebGLRenderer,
	}

	chromedp.ListenTarget(timeoutCtx, func(ev interface{}) {
		switch evt := ev.(type) {
		case *network.EventResponseReceived:
			if evt.Type == "Document" {
				navInfo.StatusCode = int(evt.Response.Status)
				if ct, ok := evt.Response.Headers["Content-Type"].(string); ok {
					navInfo.ContentType = ct
				}
			}
			navInfo.ResourceCount++
		case *network.EventRequestWillBeSent:
			if evt.RedirectResponse != nil {
				navInfo.RedirectChain = append(navInfo.RedirectChain, evt.DocumentURL)
			}
		}
	})

	navStart := time.Now()
	err := chromedp.Run(timeoutCtx,
		network.Enable(),
		page.Enable(),
		chromedp.Navigate(targetURL),
	)
	navEnd := time.Now()

	if err != nil {
		errMsg := fmt.Sprintf("Navigation error: %v", err)
		if len(errMsg) > 200 {
			errMsg = errMsg[:200] + "..."
		}
		logger.Error(errMsg)
		return &CookieResponse{
			Success:     false,
			Cookies:     make(map[string]string),
			Message:     errMsg,
			RequestID:   requestID,
			Timing:      timing,
			Fingerprint: fpMap,
		}, nil
	}

	timing["navigation_ms"] = navEnd.Sub(navStart).Milliseconds()

	injectStart := time.Now()
	if err := injectAdvancedFingerprintingResistance(timeoutCtx, fingerprint, logger); err != nil {
		logger.Debug("Fingerprinting resistance injection failed: %v", err)
	}
	timing["fingerprint_injection_ms"] = time.Since(injectStart).Milliseconds()

	waitStart := time.Now()
	if err := waitForNavigationComplete(timeoutCtx, logger, networkIdle); err != nil {
		logger.Debug("Navigation completion error: %v", err)
	}
	timing["wait_complete_ms"] = time.Since(waitStart).Milliseconds()

	if waitTime > 0 {
		logger.Debug("Applying additional wait time: %dms", waitTime)
		time.Sleep(time.Duration(waitTime) * time.Millisecond)
	}

	var jsEnabled bool
	chromedp.Evaluate(`typeof document !== 'undefined'`, &jsEnabled).Do(timeoutCtx)
	navInfo.JavaScriptEnabled = jsEnabled

	cookieStart := time.Now()
	logger.Debug("Starting cookie extraction...")

	// Extract cookies using the chromedp context directly
	var cookies []*network.Cookie
	err = chromedp.Run(timeoutCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var cookieErr error
		cookies, cookieErr = network.GetCookies().Do(ctx)
		if cookieErr != nil {
			logger.Error("GetCookies failed: %v", cookieErr)
			return cookieErr
		}
		logger.Debug("Successfully extracted %d cookies", len(cookies))
		return nil
	}))

	timing["cookie_extraction_ms"] = time.Since(cookieStart).Milliseconds()

	cookieMap := make(map[string]string)
	if err == nil && cookies != nil {
		for _, cookie := range cookies {
			cookieMap[cookie.Name] = cookie.Value
			logger.Debug("Cookie: %s = %s (domain: %s)", cookie.Name, cookie.Value[:min(len(cookie.Value), 50)], cookie.Domain)
		}
		logger.Info("Successfully extracted %d cookies", len(cookieMap))
	} else {
		logger.Error("Cookie extraction failed: %v", err)
	}

	navInfo.LoadTime = time.Since(startTime).Milliseconds()
	timing["total_ms"] = navInfo.LoadTime

	var finalURL string
	if err := chromedp.Run(timeoutCtx, chromedp.Evaluate(`window.location.href`, &finalURL)); err == nil {
		navInfo.FinalURL = finalURL
	} else {
		navInfo.FinalURL = targetURL
	}

	response := &CookieResponse{
		Success:        len(cookieMap) > 0,
		Cookies:        cookieMap,
		Message:        "OK",
		NavigationInfo: navInfo,
		RequestID:      requestID,
		Timing:         timing,
		Fingerprint:    fpMap,
	}

	if len(cookieMap) == 0 {
		response.Message = "No cookies found"
		if verbose {
			response.DOMContent = getDOMContent(timeoutCtx, logger)
		}
	}

	logger.Info("Extraction completed: %d cookies found", len(cookieMap))
	return response, nil
}

func getDOMContent(ctx context.Context, logger *Logger) string {
	var content string
	err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &content))
	if err != nil {
		logger.Debug("DOM extraction failed: %v", err)
		return fmt.Sprintf("DOM error: %v", err)
	}
	if len(content) > 10000 {
		content = content[:10000] + "...[truncated]"
	}
	return content
}

func isDevTool(userAgent string) bool {
	userAgent = strings.ToLower(userAgent)
	devTools := []string{"postman", "postmanruntime", "insomnia", "curl", "wget", "httpie", "rest-client", "paw", "thunder-client", "bruno", "hoppscotch"}
	for _, tool := range devTools {
		if strings.Contains(userAgent, tool) {
			log.Printf("[DEBUG] Detected dev tool: %s in User-Agent: %s", tool, userAgent)
			return true
		}
	}
	log.Printf("[DEBUG] No dev tool detected in User-Agent: %s", userAgent)
	return false
}

// Cryptographically secure random number generation
func secureRandomInt(min, max int) int {
	if min >= max {
		return min
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		// Fallback to math/rand if crypto/rand fails
		return mathrand.Intn(max-min+1) + min
	}
	return int(n.Int64()) + min
}

func secureRandomFloat(min, max float64) float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return mathrand.Float64()*(max-min) + min
	}
	ratio := float64(n.Int64()) / 1000000.0
	return ratio*(max-min) + min
}

// Advanced fingerprint generation functions
func generateRandomUserAgent() string {
	chromeVersions := []string{"120.0.0.0", "121.0.0.0", "122.0.0.0", "123.0.0.0"}
	firefoxVersions := []string{"122.0", "123.0", "124.0"}
	safariVersions := []string{"17.2.1", "17.3", "17.4"}

	osOptions := []struct {
		name      string
		platforms []string
	}{
		{"Windows", []string{"Windows NT 10.0; Win64; x64", "Windows NT 11.0; Win64; x64"}},
		{"Mac", []string{"Macintosh; Intel Mac OS X 10_15_7", "Macintosh; Intel Mac OS X 11_7_0", "Macintosh; Intel Mac OS X 12_6_0"}},
		{"Linux", []string{"X11; Linux x86_64", "X11; Linux i686"}},
	}

	browserChoice := secureRandomInt(0, 2)
	osChoice := secureRandomInt(0, len(osOptions)-1)
	platform := osOptions[osChoice].platforms[secureRandomInt(0, len(osOptions[osChoice].platforms)-1)]

	switch browserChoice {
	case 0: // Chrome
		version := chromeVersions[secureRandomInt(0, len(chromeVersions)-1)]
		return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", platform, version)
	case 1: // Firefox
		version := firefoxVersions[secureRandomInt(0, len(firefoxVersions)-1)]
		return fmt.Sprintf("Mozilla/5.0 (%s; rv:%s) Gecko/20100101 Firefox/%s", platform, version, version)
	default: // Safari (only for Mac)
		if osOptions[osChoice].name == "Mac" {
			version := safariVersions[secureRandomInt(0, len(safariVersions)-1)]
			return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/%s Safari/605.1.15", platform, version)
		}
		// Fallback to Chrome for non-Mac
		version := chromeVersions[secureRandomInt(0, len(chromeVersions)-1)]
		return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", platform, version)
	}
}

func extractPlatformFromUA(userAgent string) string {
	if strings.Contains(userAgent, "Windows NT 11") {
		return "Win32"
	} else if strings.Contains(userAgent, "Windows") {
		return "Win32"
	} else if strings.Contains(userAgent, "Macintosh") {
		return "MacIntel"
	} else if strings.Contains(userAgent, "Linux") {
		return "Linux x86_64"
	}
	return "Win32"
}

func generateRandomVendor() string {
	vendors := []string{"Google Inc.", "Apple Computer, Inc.", "", "Mozilla Foundation"}
	return vendors[secureRandomInt(0, len(vendors)-1)]
}

func generateRandomTimezone() string {
	timezones := []string{
		"America/New_York", "Europe/London", "Asia/Tokyo", "Europe/Paris",
		"Asia/Dubai", "America/Los_Angeles", "Europe/Berlin", "Asia/Shanghai",
		"America/Chicago", "Europe/Rome", "Asia/Singapore", "Australia/Sydney",
		"America/Denver", "Europe/Madrid", "Asia/Seoul", "America/Toronto",
		"Europe/Amsterdam", "Asia/Hong_Kong", "America/Mexico_City", "Europe/Stockholm",
	}
	return timezones[secureRandomInt(0, len(timezones)-1)]
}

func generateRandomLanguage() string {
	languages := []string{
		"en-US,en;q=0.9", "en-GB,en;q=0.9", "fr-FR,fr;q=0.8,en;q=0.6",
		"de-DE,de;q=0.8,en;q=0.6", "es-ES,es;q=0.8,en;q=0.6", "ja-JP,ja;q=0.9,en;q=0.8",
		"zh-CN,zh;q=0.9,en;q=0.8", "it-IT,it;q=0.8,en;q=0.6", "pt-BR,pt;q=0.8,en;q=0.6",
		"ru-RU,ru;q=0.9,en;q=0.8", "ko-KR,ko;q=0.9,en;q=0.8", "ar-SA,ar;q=0.9,en;q=0.8",
	}
	return languages[secureRandomInt(0, len(languages)-1)]
}

func generateRandomScreenWidth() int {
	// Common screen widths with mathematical variance
	commonWidths := []int{1366, 1920, 1440, 1600, 1280, 1536, 2560, 1680, 1024, 1152}
	baseWidth := commonWidths[secureRandomInt(0, len(commonWidths)-1)]
	// Add some variance (-50 to +50 pixels)
	variance := secureRandomInt(-50, 50)
	return int(math.Max(800, float64(baseWidth+variance)))
}

func generateRandomScreenHeight() int {
	// Common screen heights with mathematical variance
	commonHeights := []int{768, 1080, 900, 720, 864, 1440, 1050, 800, 600}
	baseHeight := commonHeights[secureRandomInt(0, len(commonHeights)-1)]
	// Add some variance (-30 to +30 pixels)
	variance := secureRandomInt(-30, 30)
	return int(math.Max(600, float64(baseHeight+variance)))
}

func generateRandomColorDepth() int {
	depths := []int{24, 32}
	return depths[secureRandomInt(0, len(depths)-1)]
}

func generateRandomPixelRatio() float64 {
	// Generate pixel ratio between 1.0 and 3.0 with realistic variance
	ratios := []float64{1.0, 1.25, 1.5, 1.75, 2.0, 2.25, 2.5, 3.0}
	baseRatio := ratios[secureRandomInt(0, len(ratios)-1)]
	// Add small variance
	variance := secureRandomFloat(-0.05, 0.05)
	return math.Max(1.0, baseRatio+variance)
}

func generateRandomHardwareCores() int {
	// Weighted towards common core counts
	coreDistribution := map[int]int{
		2: 10, 4: 30, 6: 20, 8: 25, 12: 10, 16: 4, 24: 1,
	}

	totalWeight := 0
	for _, weight := range coreDistribution {
		totalWeight += weight
	}

	randomValue := secureRandomInt(1, totalWeight)
	currentWeight := 0

	for cores, weight := range coreDistribution {
		currentWeight += weight
		if randomValue <= currentWeight {
			return cores
		}
	}
	return 4 // fallback
}

func generateRandomTouchPoints() int {
	// Realistic touch point distribution
	touchDistribution := map[int]int{
		0: 70, 5: 15, 10: 10, 20: 5,
	}

	totalWeight := 0
	for _, weight := range touchDistribution {
		totalWeight += weight
	}

	randomValue := secureRandomInt(1, totalWeight)
	currentWeight := 0

	for points, weight := range touchDistribution {
		currentWeight += weight
		if randomValue <= currentWeight {
			return points
		}
	}
	return 0 // fallback
}

func generateRandomCanvasFingerprint() string {
	// Generate a more realistic canvas fingerprint
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := secureRandomInt(16, 32)
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		result[i] = chars[secureRandomInt(0, len(chars)-1)]
	}

	return string(result)
}

func generateRandomWebGLVendor() string {
	vendors := []string{
		"Intel Inc.", "NVIDIA Corporation", "AMD", "Apple Inc.",
		"Google Inc. (Intel)", "Google Inc. (NVIDIA)", "Google Inc. (AMD)",
		"Intel Open Source Technology Center", "Mesa Project",
	}
	return vendors[secureRandomInt(0, len(vendors)-1)]
}

func generateRandomWebGLRenderer() string {
	renderers := []string{
		"Intel(R) UHD Graphics 620", "NVIDIA GeForce GTX 1060", "AMD Radeon RX 580",
		"Intel(R) Iris(TM) Plus Graphics", "NVIDIA GeForce RTX 3070", "AMD Radeon Pro 5500M",
		"Intel(R) HD Graphics 630", "Mesa DRI Intel(R) Ivybridge Mobile",
		"ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0)",
		"ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 Direct3D11 vs_5_0 ps_5_0)",
	}
	return renderers[secureRandomInt(0, len(renderers)-1)]
}

func generateRandomAudioFingerprint() string {
	// Generate realistic audio context fingerprint
	sampleRates := []string{"44100", "48000", "96000"}
	channels := []string{"2", "6", "8"}

	sampleRate := sampleRates[secureRandomInt(0, len(sampleRates)-1)]
	channelCount := channels[secureRandomInt(0, len(channels)-1)]

	return fmt.Sprintf("sr:%s_ch:%s", sampleRate, channelCount)
}

func generateRandomFontFingerprint() string {
	// Simulate font availability fingerprint
	fontCount := secureRandomInt(30, 150)
	return fmt.Sprintf("fonts:%d", fontCount)
}

func generateRandomCPUClass() string {
	classes := []string{"x86", "x64", "ARM", "ARM64"}
	return classes[secureRandomInt(0, len(classes)-1)]
}

func generateRandomDeviceMemory() int {
	// Device memory in GB - realistic distribution
	memoryOptions := []int{2, 4, 6, 8, 12, 16, 24, 32}
	weights := []int{5, 25, 15, 30, 10, 10, 3, 2}

	totalWeight := 0
	for _, weight := range weights {
		totalWeight += weight
	}

	randomValue := secureRandomInt(1, totalWeight)
	currentWeight := 0

	for i, weight := range weights {
		currentWeight += weight
		if randomValue <= currentWeight {
			return memoryOptions[i]
		}
	}
	return 8 // fallback
}

func generateRandomNavigatorPlatform(basePlatform string) string {
	// Add slight variations to navigator.platform
	variations := map[string][]string{
		"Win32":        {"Win32", "Win64"},
		"MacIntel":     {"MacIntel", "Intel Mac OS X"},
		"Linux x86_64": {"Linux x86_64", "Linux i686", "X11"},
	}

	if options, exists := variations[basePlatform]; exists {
		return options[secureRandomInt(0, len(options)-1)]
	}
	return basePlatform
}

// Helper function for minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var browserPool *BrowserPool

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	maxConcurrent := runtime.NumCPU() * 4
	if maxConcurrent < 8 {
		maxConcurrent = 8
	}
	if maxConcurrent > 32 {
		maxConcurrent = 32
	}

	browserPool = NewBrowserPool(maxConcurrent)
	if err := browserPool.Initialize(); err != nil {
		log.Fatal("Browser pool init failed:", err)
	}
	defer browserPool.Cleanup()

	r.POST("/cookies", handleCookieRequest)
	r.GET("/", handleRoot)
	r.GET("/health", handleHealth)
	r.GET("/fingerprint", handleFingerprint)

	log.Printf("Enhanced Server starting on :8000 (Concurrency: %d)", maxConcurrent)
	log.Fatal(http.ListenAndServe(":8000", r))
}

func handleCookieRequest(c *gin.Context) {
	requestID := uuid.New().String()[:8]
	userAgent := c.GetHeader("User-Agent")

	var req CookieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid request format",
			"request_id": requestID,
		})
		return
	}

	if req.Timeout < 5000 || req.Timeout > 60000 {
		req.Timeout = 15000
	}
	if req.WaitTime < 0 || req.WaitTime > 15000 {
		req.WaitTime = 1000
	}
	if req.RetryCount < 0 || req.RetryCount > 5 {
		req.RetryCount = 2
	}
	if req.NetworkIdle < 500 || req.NetworkIdle > 10000 {
		req.NetworkIdle = 2000
	}

	if _, err := url.ParseRequestURI(req.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Invalid URL format",
			"request_id": requestID,
		})
		return
	}

	var headless bool
	if req.Headless != nil {
		headless = *req.Headless
		log.Printf("[DEBUG][%s] Headless mode explicitly set to: %t", requestID, headless)
	} else {
		isDevToolDetected := isDevTool(userAgent)
		headless = !isDevToolDetected
		log.Printf("[DEBUG][%s] User-Agent: %s, Dev tool detected: %t, Headless: %t", requestID, userAgent, isDevToolDetected, headless)
	}

	if req.Browser == "" {
		req.Browser = "chrome" // Default to Chrome
	}

	proxyConfig := parseProxy(req.Proxy)

	result, err := extractCookiesWithRetry(
		context.Background(),
		req.URL,
		proxyConfig,
		req.WaitTime,
		req.Timeout,
		req.RetryCount,
		req.NetworkIdle,
		req.Verbose,
		requestID,
		headless,
		req.Browser,
		req.BrowserPath,
	)

	if err != nil && result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":      "Processing failed",
			"request_id": requestID,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

func handleRoot(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service":       "enhanced-cookie-extractor",
		"version":       "3.3.0",
		"concurrency":   browserPool.maxContexts,
		"features":      []string{"advanced-fingerprinting", "crypto-randomization", "navigation-resilience", "proxy-support", "brave-browser-support", "custom-browser-path"},
		"browsers":      []string{"chrome", "brave"},
		"documentation": "POST /cookies with JSON payload",
		"example": map[string]interface{}{
			"url":          "https://example.com",
			"browser":      "brave",
			"browser_path": "C:\\path\\to\\brave.exe",
			"verbose":      true,
			"headless":     false,
		},
	})
}

func handleHealth(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, gin.H{
		"status":     "healthy",
		"contexts":   len(browserPool.contexts),
		"goroutines": runtime.NumGoroutine(),
		"memory_mb":  m.Alloc / 1024 / 1024,
		"gc_cycles":  m.NumGC,
	})
}

func handleFingerprint(c *gin.Context) {
	fp := generateAdvancedFingerprint()

	c.JSON(http.StatusOK, gin.H{
		"fingerprint": map[string]interface{}{
			"user_agent":         fp.UserAgent,
			"platform":           fp.Platform,
			"vendor":             fp.Vendor,
			"language":           fp.Language,
			"timezone":           fp.Timezone,
			"screen_width":       fp.ScreenWidth,
			"screen_height":      fp.ScreenHeight,
			"color_depth":        fp.ColorDepth,
			"pixel_ratio":        fp.PixelRatio,
			"hardware_cores":     fp.HardwareCores,
			"max_touch_points":   fp.MaxTouchPoints,
			"webgl_vendor":       fp.WebGLVendor,
			"webgl_renderer":     fp.WebGLRenderer,
			"audio_fingerprint":  fp.AudioFingerprint,
			"font_fingerprint":   fp.FontFingerprint,
			"cpu_class":          fp.CPUClass,
			"device_memory":      fp.DeviceMemory,
			"navigator_platform": fp.NavigatorPlatform,
		},
	})
}
