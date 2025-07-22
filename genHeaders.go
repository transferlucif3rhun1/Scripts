package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "math/rand"
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/gorilla/mux"
)

func init() {
    rand.Seed(time.Now().UnixNano())
}

type Header struct {
    Key   string
    Value string
}

func main() {
    router := mux.NewRouter()
    router.HandleFunc("/generateHeaders", generateHeadersHandler).Methods("POST")
    router.HandleFunc("/auth", authHandler).Methods("POST")

    server := &http.Server{
        Handler:      router,
        Addr:         "0.0.0.0:8050",
        WriteTimeout: 15 * time.Second,
        ReadTimeout:  15 * time.Second,
        IdleTimeout:  30 * time.Second,
    }

    log.Println("Server has started on port 8050")
    if err := server.ListenAndServe(); err != nil {
        log.Fatalf("Error starting server: %v", err)
    }
}

func generateHeadersHandler(w http.ResponseWriter, r *http.Request) {
    var request struct {
        UA    string `json:"ua"`
        Proxy string `json:"proxy"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    if request.UA == "" || request.Proxy == "" {
        http.Error(w, "Missing required fields 'ua' and/or 'proxy'", http.StatusBadRequest)
        return
    }

    // Generate the headers based on the UA string provided in the request
    headers := generateHeaders(request.UA)
    proxyAddress := formatIpAddress(request.Proxy)

    // Creating a slice of maps to maintain the order of headers in the response
    var orderedHeaders []map[string]string
    for _, header := range headers {
        orderedHeaders = append(orderedHeaders, map[string]string{header.Key: header.Value})
    }

    // Constructing the final response
    response := map[string]interface{}{
        "headers": orderedHeaders, // This maintains order but changes the structure
        "proxy":   proxyAddress,
    }

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(response); err != nil {
        log.Printf("Error encoding JSON response: %v", err)
        http.Error(w, "Failed to encode response", http.StatusInternalServerError)
    }
}

func authHandler(w http.ResponseWriter, r *http.Request) {
    var request struct {
        Proxy string `json:"proxy"`
        User  string `json:"user"`
        Pass  string `json:"pass"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    if request.Proxy == "" || request.User == "" || request.Pass == "" {
        http.Error(w, "Missing required fields 'proxy', 'user', and/or 'pass'", http.StatusBadRequest)
        return
    }

    // Generate headers
    headers := generateHeaders("Mozilla/5.0")
    headersMap := make(map[string]string)
    for _, header := range headers {
        headersMap[header.Key] = header.Value
    }

    // Format proxy address
    proxyAddress := formatIpAddress(request.Proxy)

    // Create payload
    payload := map[string]interface{}{
        "tlsClientIdentifier":            "chrome_117",
        "followRedirects":                true,
        "insecureSkipVerify":             false,
        "withoutCookieJar":               false,
        "withDefaultCookieJar":           true,
        "isByteRequest":                  false,
        "forceHttp1":                     false,
        "withRandomTLSExtensionOrder":    true,
        "timeoutSeconds":                 30,
        "timeoutMilliseconds":            0,
        "sessionId":                      nil,
        "proxyUrl":                       fmt.Sprintf("http://%s", proxyAddress),
        "certificatePinningHosts":        map[string]interface{}{},
        "headers":                        headersMap,
        "requestUrl":                     "https://auth.ticketmaster.com/json/sign-in/.ico",
        "requestMethod":                  "POST",
        "requestBody":                    fmt.Sprintf(`{"email":"%s","password":"%s","rememberMe":true,"externalUserToken":null}`, request.User, request.Pass),
        "requestCookies":                 []interface{}{},
    }

    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        log.Printf("Error marshalling payload: %v", err)
        http.Error(w, "Failed to create request payload", http.StatusInternalServerError)
        return
    }

    // Make request to the specified IP
    apiURL := fmt.Sprintf("http://localhost:8080/api/forward")
    req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payloadBytes))
    if err != nil {
        log.Printf("Error creating request: %v", err)
        http.Error(w, "Failed to create request", http.StatusInternalServerError)
        return
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("x-api-key", "my-auth-key-1")

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Error making request: %v", err)
        http.Error(w, "Failed to make request", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        log.Printf("Error reading response: %v", err)
        http.Error(w, "Failed to read response", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(body)
}

func generateHeaders(ua string) []Header {

    headers := []Header{
        {"tm-integrator-id", "prd1741.iccp"},
        {"tm-oauth-type", "tm-auth"},
        {"tm-placement-id", "mytmlogin"},
        {"sec-ch-ua", `"Not/A},Brand";v="8", "Chromium";v="126", "Brave";v="126"`},
        {"dnt", "1"},
        {"accept-language", "en-us"},
        {"sec-ch-ua-mobile", "?0"},
        {"user-agent", ua},
        {"content-type", "application/json"},
        {"tm-site-token", "tm-us"},
        {"tm-client-id", "8bf7204a7e97.web.ticketmaster.us"},
        {"sec-ch-ua-platform", `"macOS"`},
        {"accept", "*/*"},
        {"sec-gpc", "1"},
        {"origin", "https://auth.ticketmaster.com"},
        {"sec-fetch-site", "same-origin"},
        {"sec-fetch-mode", "cors"},
        {"sec-fetch-dest", "empty"},
        {"referer", "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=8bf7204a7e97.web.ticketmaster.us&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.com/exchange&visualPresets=tm&lang=en-us&placementId=mytmlogin&hideLeftPanel=false&integratorId=prd1741.iccp&intSiteToken=tm-us&deviceId=mxsMbMf0W7m3ucG5tLfAtsG5t8AR1%2F9mPDcCUQ"},
        {"accept-encoding", "gzip, deflate, br, zstd"},
    }
    return headers
}

func formatIpAddress(input string) string {
    parts := strings.Split(input, ":")
    if len(parts) == 4 {
        // Format is ip:port:user:pass, convert to user:pass@ip:port
        return fmt.Sprintf("%s:%s@%s:%s", parts[2], parts[3], parts[0], parts[1])
    }
    return input // Return input if not in ip:port:user:pass format
}

func generateRandomToken() string {
    return uuid.NewString()
}

func generateRandomDate() string {
    daysAgo := rand.Intn(100)
    randomDate := time.Now().AddDate(0, 0, -daysAgo).Format(time.RFC1123)
    return randomDate
}

func randomChoice(options []string) string {
    n := rand.Intn(len(options))
    return options[n]
}

func generateRandomHttpMethods(count int, methods []string) string {
    chosenMethods := make([]string, count)
    for i := 0; i < count; i++ {
        chosenMethods[i] = methods[rand.Intn(len(methods))]
    }
    return strings.Join(chosenMethods, ", ")
}
