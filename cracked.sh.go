package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type ThreadData struct {
	URL        string
	BumpURL    string
	LastBump   time.Time
	LastParsed time.Time
}

var (
	threads    = make(map[string]*ThreadData)
	httpClient = &http.Client{Timeout: 30 * time.Second}
	userAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36"
	cookie     = "mybb[lastactive]=1749018970;sid=a190f6643df859e1656576ba5e1c7bc6;loginattempts=1;__ddg1_=BzVMAMGM9e3eqTdX4UvA;mybb[threadread]=...;mybbuser=2741847_c033910e63bd1855be0b71b34759561203ecf8a8e"
)

func logMessage(level, msg string) {
	fmt.Printf("[%s] %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), level, msg)
}

func loadThreads(file string) error {
	f, err := os.Open(file)
	if err != nil {
		logMessage("ERROR", fmt.Sprintf("opening %s: %v", file, err))
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		u := strings.TrimSpace(scanner.Text())
		if u != "" && strings.HasPrefix(u, "http") {
			if _, ok := threads[u]; !ok {
				threads[u] = &ThreadData{URL: u}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logMessage("ERROR", fmt.Sprintf("reading %s: %v", file, err))
		return err
	}
	return nil
}

func makeRequest(method, target string, data url.Values) (*http.Response, error) {
	var req *http.Request
	var err error
	if method == "POST" && data != nil {
		req, err = http.NewRequest(method, target, strings.NewReader(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, target, nil)
	}
	if err != nil {
		logMessage("ERROR", fmt.Sprintf("creating %s to %s: %v", method, target, err))
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	req.Header.Set("Cookie", cookie)
	resp, err := httpClient.Do(req)
	if err != nil {
		logMessage("WARN", fmt.Sprintf("request to %s failed: %v", target, err))
		return nil, err
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		logMessage("WARN", fmt.Sprintf("status %d from %s", resp.StatusCode, target))
	}
	return resp, nil
}

func parseBumpURL(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	sel := doc.Find("#pjax-container > div.thread-header > a")
	href, ok := sel.Attr("href")
	if !ok || !strings.Contains(href, "bump_thread") {
		return ""
	}
	if strings.HasPrefix(href, "http") {
		return href
	}
	return "https://cracked.sh/" + strings.TrimPrefix(href, "/")
}

func parseFormInputs(html string) map[string]string {
	inputs := make(map[string]string)
	if m := regexp.MustCompile(`newreply\.php\?tid=(\d+)`).FindStringSubmatch(html); len(m) > 1 {
		inputs["tid"] = m[1]
	}
	for _, m := range regexp.MustCompile(`<input[^>]*name="([^"]*)"[^>]*value="([^"]*)"`).FindAllStringSubmatch(html, -1) {
		if len(m) >= 3 {
			inputs[m[1]] = m[2]
		}
	}
	if m := regexp.MustCompile(`lastpid[^>]*value="(\d+)"`).FindStringSubmatch(html); len(m) > 1 {
		inputs["lastpid"] = m[1]
	}
	return inputs
}

func extractThreadID(u string) string {
	parts := strings.Split(u, "/")
	return strings.TrimPrefix(parts[len(parts)-1], "Thread-")
}

func bumpThreadType(t *ThreadData) int {
	if time.Since(t.LastBump) < 24*time.Hour {
		return 0
	}
	if t.BumpURL == "" {
		if t.LastParsed.IsZero() || time.Since(t.LastParsed) >= 24*time.Hour {
			resp, err := makeRequest("GET", t.URL, nil)
			if err != nil {
				return 0
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			t.BumpURL = parseBumpURL(string(b))
			t.LastParsed = time.Now()
		} else {
			return 0
		}
	}
	if t.BumpURL == "" {
		return 0
	}
	resp, err := makeRequest("GET", t.BumpURL, nil)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200, 302:
		t.LastBump = time.Now()
		return 1
	case 404:
		resp2, err := makeRequest("GET", t.URL, nil)
		if err != nil {
			return 0
		}
		defer resp2.Body.Close()
		b2, _ := io.ReadAll(resp2.Body)
		inputs := parseFormInputs(string(b2))
		if inputs["tid"] == "" || inputs["my_post_key"] == "" {
			return 0
		}
		data := url.Values{
			"my_post_key":            {inputs["my_post_key"]},
			"subject":                {inputs["subject"]},
			"action":                 {"do_newreply"},
			"posthash":               {inputs["posthash"]},
			"lastpid":                {inputs["lastpid"]},
			"from_page":              {"1"},
			"tid":                    {inputs["tid"]},
			"method":                 {"quickreply"},
			"message":                {"Bump"},
			"postoptions[signature]": {"1"},
		}
		reply, err := makeRequest("POST", "https://cracked.sh/newreply.php?ajax=1", data)
		if err != nil {
			return 0
		}
		defer reply.Body.Close()
		if reply.StatusCode == 200 || reply.StatusCode == 302 {
			t.LastBump = time.Now()
			resp3, err := makeRequest("GET", t.URL, nil)
			if err == nil {
				defer resp3.Body.Close()
				b3, _ := io.ReadAll(resp3.Body)
				if nu := parseBumpURL(string(b3)); nu != "" {
					t.BumpURL = nu
				}
			}
			return 2
		}
	}
	return 0
}

func main() {
	logMessage("INFO", "started")
	cycle := 0
	for {
		cycle++
		if err := loadThreads("cracked.sh.txt"); err != nil {
			logMessage("ERROR", fmt.Sprintf("load cycle %d failed", cycle))
			time.Sleep(5 * time.Minute)
			continue
		}
		logMessage("INFO", fmt.Sprintf("loaded %d threads", len(threads)))
		if len(threads) == 0 {
			logMessage("WARN", "no threads")
			time.Sleep(5 * time.Minute)
			continue
		}
		total := len(threads)
		direct, fallback, waiting, noURL := 0, 0, 0, 0
		i := 0
		for _, t := range threads {
			i++
			logMessage("INFO", fmt.Sprintf("processing %d/%d: %s", i, total, t.URL))
			if time.Since(t.LastBump) < 24*time.Hour {
				waiting++
				logMessage("INFO", fmt.Sprintf("skipped %s (waiting)", extractThreadID(t.URL)))
			} else {
				typ := bumpThreadType(t)
				switch typ {
				case 1:
					direct++
					logMessage("INFO", fmt.Sprintf("bumped %s via direct link", extractThreadID(t.URL)))
				case 2:
					fallback++
					logMessage("INFO", fmt.Sprintf("replied %s via fallback", extractThreadID(t.URL)))
				default:
					if t.BumpURL == "" {
						noURL++
						logMessage("INFO", fmt.Sprintf("no bump URL for %s", extractThreadID(t.URL)))
					} else {
						logMessage("INFO", fmt.Sprintf("no action for %s", extractThreadID(t.URL)))
					}
				}
			}
			time.Sleep(2 * time.Second)
		}
		logMessage("INFO", fmt.Sprintf("cycle %d: %d direct, %d fallback, %d waiting, %d no-url", cycle, direct, fallback, waiting, noURL))
		time.Sleep(30 * time.Minute)
	}
}
