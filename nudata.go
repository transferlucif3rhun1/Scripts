package main

import (
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "math/rand"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    "github.com/gin-contrib/cors"
    "github.com/gin-gonic/gin"
)

// -------------------- CONSTANTS --------------------//

var (
    C    *int64 = nil
    aa   *int64 = nil
    fa   *int64 = nil
    mu         = sync.Mutex{}

    platforms = []string{
        "Win32",
        //"MacIntel",
    }

    screens = []string{
        "1068:970:1920:1040:1920:1040",
    }

    plugins = []string{
        "p,Chrome PDF Viewer,Chromium PDF Viewer,Microsoft Edge PDF Viewer,PDF Viewer,WebKit built-in PDF",
    }

    fingerprints = []string{
        "1080x1920 24",
    }

    video = []string{
        "fv,ogg,mp4,webm",
    }

    audio = []string{
        "fa,mpeg,ogg,wav",
    }

    signatures = []string{
        "data:image/png;base64,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",
    }

    touch = []string{
        `{"mtp":0,"ts":false,"te":false}`,
    }

    cookie = []string{
        `{"tc":true,"nc":true}`,
    }

    webGL = []string{
        `{"ContextName":"webgl","VENDOR":"WebKit","VERSION":"WebGL 1.0 (OpenGL ES 2.0 Chromium)","RENDERER":"WebKit WebGL","SHADING_LANGUAGE_VERSION":"WebGL GLSL ES 1.0 (OpenGL ES GLSL ES 1.0 Chromium)","DEPTH_BITS":24,"MAX_VERTEX_ATTRIBS":16,"MAX_VERTEX_TEXTURE_IMAGE_UNITS":16,"MAX_VARYING_VECTORS":30,"MAX_VERTEX_UNIFORM_VECTORS":4095,"MAX_COMBINED_TEXTURE_IMAGE_UNITS":32,"MAX_TEXTURE_SIZE":16384,"MAX_CUBE_MAP_TEXTURE_SIZE":16384,"MAX_RENDERBUFFER_SIZE":16384,"MAX_VIEWPORT_DIMS":{"0":32767,"1":32767},"ALIASED_LINE_WIDTH_RANGE":{"0":1,"1":1},"ALIASED_POINT_SIZE_RANGE":{"0":1,"1":1024}}`,
    }

    fonts = []string{
        "620.05078125,620.05078125,620.05078125,620.05078125,647.75390625,791.9296875,620.05078125,620.05078125,647.75390625,620.05078125,620.05078125,620.05078125,620.05078125,620.05078125,624.7265625,658.16015625,658.16015625,620.05078125,620.05078125,620.05078125,620.05078125,618.9609375,514.6171875,561.69140625,561.69140625,620.05078125,620.05078125,696.515625,647.75390625,620.05078125,614.28515625,620.05078125,620.05078125,563.9765625,620.05078125,620.05078125,620.05078125,620.05078125,620.05078125,734.625,649.01953125,620.05078125,620.05078125,468,620.05078125,574.3125,620.05078125,649.01953125,620.05078125,620.05078125,620.05078125,620.05078125,698.484375,802.0546875,815.34375,672.46875,635.9765625,694.16015625,672.46875,654.328125,620.05078125,620.05078125,620.05078125,660.65625,759.5859375,849.55078125,620.05078125,620.05078125",
    }
)

// -------------------- CONSTANTS END --------------------//

// --------------------- LOGIC START ---------------------//

func getWkr() int {
    return rand.Intn(1_000_000) + 1_000
}

func getRandomItem(array []string) string {
    return array[rand.Intn(len(array))]
}

func getTimestamp() int64 {
    return time.Now().UnixNano() / int64(time.Millisecond)
}

func getShortTimestamp() int64 {
    return time.Now().Unix()
}

func hashWidget(a string) string {
    var result strings.Builder
    for _, char := range a {
        if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
            if char <= 'M' || char <= 'm' {
                result.WriteRune(char + 13)
            } else {
                result.WriteRune(char - 13)
            }
        } else {
            result.WriteRune(char)
        }
    }
    return result.String()
}

func base64ToBytes(a string) []byte {
    data, err := base64.StdEncoding.DecodeString(a)
    if err != nil {
        return []byte{}
    }
    return data
}

func bytesToBase64(a []byte) string {
    return base64.StdEncoding.EncodeToString(a)
}

func nsgmdzxs(a string) string {
    b := ""
    c := []int{41, 8, 49, 48, 51, 44, 63, 0, 19, 61, 43, 63, 57, 15, 34, 6, 42, 59, 41, 19, 10, 45, 54, 0, 44, 34, 57, 36, 48}
    e := false
    if strings.HasPrefix(a, "BNDX:") {
        a = a[5:]
        data := base64ToBytes(a)
        a = string(data)
        a = a[4:]
        e = true
    } else {
        b = "NDX:"
    }

    f := 0
    for _, char := range a {
        t := int(char) - 32
        if t >= 0 && t < 94 {
            if e && t < 64 {
                t ^= c[f%len(c)]
            }

            // Corrected the ternary-like expression with if-else
            if e {
                t += 47 + (-1 * f * 31)
            } else {
                t += 47 + (1 * f * 31)
            }

            t = (t % 94 + 94) % 94

            if !e && t < 64 {
                t ^= c[f%len(c)]
            }
            f++
        }
        b += string(t + 32)
    }

    if e {
        return b
    } else {
        encoded := bytesToBase64([]byte(b))
        return "BNDX:" + encoded
    }
}

func quickHash(a string) string {
    b := 0
    c := 0
    for i, char := range a {
        if i%2 == 0 {
            b = ((b << 5) - b) + int(char)
            b = int(int32(b)) // Simulate 32-bit integer overflow
        } else {
            c = ((c << 5) - c) + int(char)
            c = int(int32(c)) // Simulate 32-bit integer overflow
        }
    }
    if b < 0 {
        b = 4294967295 + b + 1
    }
    if c < 0 {
        c = 4294967295 + c + 1
    }
    return fmt.Sprintf("%x%x", b, c)
}

func randomRange(min, max int) int {
    return rand.Intn(max-min+1) + min
}

func bFunc(a string, b int64, c []interface{}) string {
    mu.Lock()
    defer mu.Unlock()

    if C == nil {
        // To prevent dereferencing a nil pointer
        return ""
    }

    b -= *C
    // The original code had '1 < 1' which is always false, likely a placeholder
    // So we skip the conditional division by 1
    // if 1 < 1 { b = int64(math.Round(float64(b) / 1)) }

    result := fmt.Sprintf("%s,%x", a, b)
    if c != nil && len(c) > 0 {
        var parts []string
        for _, item := range c {
            if item == nil {
                continue
            }
            switch v := item.(type) {
            case float64:
                parts = append(parts, fmt.Sprintf("%x", int(v+0.5)))
            case int:
                parts = append(parts, fmt.Sprintf("%x", v))
            case string:
                parts = append(parts, v)
            default:
                parts = append(parts, fmt.Sprintf("%v", v))
            }
        }
        if len(parts) > 0 {
            result += "," + strings.Join(parts, ",")
        }
    }
    return result
}

func updateIPR(a string, d []interface{}, val string) string {
    mu.Lock()
    defer mu.Unlock()

    e := getTimestamp()

    if C == nil {
        // Initialize C with current timestamp
        C = &e
        // Append 'ncip' data
        val += bFunc("ncip", e, []interface{}{getShortTimestamp(), 2, 1}) + ";"
    }

    val += bFunc(a, e, d) + ";"

    if aa == nil || e-*aa >= 15000 {
        if fa == nil {
            fa = &e
        }
        val += bFunc("ts", e, []interface{}{e - *fa}) + ";"
        aa = &e
        *C = e
    }

    return val
}

type WidgetData struct {
    Bd    string `json:"bd"`   // Screen Parameters
    Jsv   string `json:"jsv"`  // Script Version
    Wv    string `json:"wv"`   // Widget Version
    Bp    string `json:"bp"`   // Browser Plugins (Hashed)
    Sr    string `json:"sr"`   // Screen Fingerprint
    Didtz string `json:"didtz"` // Device Timezone
    Wkr   int    `json:"wkr"`  // Random Value
    Flv   string `json:"flv"`  // Flash Installed?
    Fv    string `json:"fv"`   // Supported Video Formats
    Fa    string `json:"fa"`   // Supported Audio Formats
    Hf    string `json:"hf"`   // Canvas Signature
    Pl    string `json:"pl"`   // Device Platform
    Ft    string `json:"ft"`   // Device Touch Settings
    Fc    string `json:"fc"`   // Cookies enabled?
    Fs    string `json:"fs"`   // HTML5 LocalStorage
    Wg    string `json:"wg"`   // WebGL Information
    Fm    string `json:"fm"`   // Font Metrics
    Ipr   string `json:"ipr"`  // IPR Value
    Ic    string `json:"ic"`   // Widget Values. "de" = ALL
    Af    string `json:"af"`   // Always null
    Dit   string `json:"dit"`  // Always null
}

type WidgetResponse struct {
    WidgetData WidgetData `json:"widgetData"`
    Wt         string     `json:"wt"`
}

func getWidgetData(widgetType string) (string, error) {
    // Fetch the external JS data
    resp, err := http.Get("https://api-ticketmaster.nd.nudatasecurity.com/2.2/w/w-481390/sync/js/")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }
    body := string(bodyBytes)

    // Extract wt, version, wVersion
    wtSplit := strings.Split(body, `"wt":"`)
    if len(wtSplit) < 2 {
        return "", fmt.Errorf("wt not found")
    }
    wt := strings.Split(wtSplit[1], `"`)[0]

    versionSplit := strings.Split(body, `ndjsStaticVersion="`)
    if len(versionSplit) < 2 {
        return "", fmt.Errorf("ndjsStaticVersion not found")
    }
    version := strings.Split(versionSplit[1], `"`)[0]

    wVersionSplit := strings.Split(body, `ndsWidgetVersion="`)
    if len(wVersionSplit) < 2 {
        return "", fmt.Errorf("ndsWidgetVersion not found")
    }
    wVersion := strings.Split(wVersionSplit[1], `"`)[0]

    // Initialize IPR
    var IPR string

    // Update IPR based on widget type
    switch widgetType {
    case "checkout-regular":
        IPR = updateIPR("st", []interface{}{""}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), "__next"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{randomRange(5000, 7000), 10, "NOP"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), "__next"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(290, 300), randomRange(300, 310), "placeOrderOptIn1input"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
    case "checkout-fanwallet":
        IPR = updateIPR("st", []interface{}{""}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        for i := 0; i < 7; i++ {
            IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        }
        IPR = updateIPR("mm", []interface{}{randomRange(320, 340), randomRange(80, 120), ""}, IPR)
    case "verifiedfan":
        IPR = updateIPR("st", []interface{}{""}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("st", []interface{}{"email,42,first_name,6,last_name,5,zip,5,intl-phone-number,12,avionrewardsemail,42"}, IPR)
        IPR = updateIPR("st", []interface{}{"email,42,first_name,6,last_name,5,zip,5,intl-phone-number,12,avionrewardsemail,42"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(320, 340), randomRange(80, 120), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("kk", []interface{}{0, "avionrewardsemail"}, IPR)
        IPR = updateIPR("ff", []interface{}{"avionrewardsemail"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(260, 270), randomRange(265, 275), "avionrewardsemail"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(320, 340), randomRange(80, 120), "avionrewardsemail"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(260, 270), randomRange(265, 275), "market"}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(260, 270), randomRange(265, 275), "optional_markets-0"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(300, 310), randomRange(265, 275), ""}, IPR)
    default:
        return "", fmt.Errorf("unknown widget type")
    }

    data := WidgetData{
        Bd:    getRandomItem(screens),
        Jsv:   version,
        Wv:    wVersion,
        Bp:    quickHash(getRandomItem(plugins)),
        Sr:    getRandomItem(fingerprints),
        Didtz: "0",
        Wkr:   getWkr(),
        Flv:   getRandomItem([]string{"true", "false"}),
        Fv:    getRandomItem(video),
        Fa:    getRandomItem(audio),
        Hf:    quickHash(getRandomItem(signatures)),
        Pl:    getRandomItem(platforms),
        Ft:    getRandomItem(touch),
        Fc:    getRandomItem(cookie),
        Fs:    "true",
        Wg:    quickHash(getRandomItem(webGL)),
        Fm:    quickHash(getRandomItem(fonts)),
        Ipr:   IPR,
        Ic:    "0,de;",
        Af:    "",
        Dit:   "",
    }

    widgetResponse := WidgetResponse{
        WidgetData: data,
        Wt:         wt,
    }

    jsonData, err := json.Marshal(widgetResponse)
    if err != nil {
        return "", err
    }

    return nsgmdzxs(string(jsonData)), nil
}

// ---------------------- LOGIC END ----------------------//

// -------------------- ROUTES START --------------------//

func main() {
    // Initialize random seed
    rand.Seed(time.Now().UnixNano())

    // Initialize Gin router
    router := gin.Default()

    // Setup CORS
    router.Use(cors.Default())

    // Initialize global variables
    initialTimestamp := getTimestamp()
    C = &initialTimestamp
    aa = &initialTimestamp
    fa = &initialTimestamp

    // Define /login route
    router.GET("/login", func(c *gin.Context) {
        resp, err := http.Get("https://api-ticketmaster.nd.nudatasecurity.com/2.2/w/w-481390/sync/js/")
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch external JS"})
            return
        }
        defer resp.Body.Close()

        bodyBytes, err := io.ReadAll(resp.Body)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read external JS"})
            return
        }
        body := string(bodyBytes)

        // Extract wt, version, wVersion
        wtSplit := strings.Split(body, `"wt":"`)
        if len(wtSplit) < 2 {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "wt not found"})
            return
        }
        wt := strings.Split(wtSplit[1], `"`)[0]

        versionSplit := strings.Split(body, `ndjsStaticVersion="`)
        if len(versionSplit) < 2 {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "ndjsStaticVersion not found"})
            return
        }
        version := strings.Split(versionSplit[1], `"`)[0]

        wVersionSplit := strings.Split(body, `ndsWidgetVersion="`)
        if len(wVersionSplit) < 2 {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "ndsWidgetVersion not found"})
            return
        }
        wVersion := strings.Split(wVersionSplit[1], `"`)[0]

        // Initialize IPR
        var IPR string

        // Update IPR for /login
        IPR = updateIPR("st", []interface{}{"email[objectobject]__input,0,password[objectobject]__input,0"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{0, 10, "NOP"}, IPR)
        IPR = updateIPR("mms", []interface{}{randomRange(7000, 12000), 10, "NOP"}, IPR)
        IPR = updateIPR("kk", []interface{}{0, "email[objectobject]__input"}, IPR)
        IPR = updateIPR("ff", []interface{}{"email[objectobject]__input"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(260, 270), randomRange(265, 275), "email[objectobject]__input"}, IPR)
        IPR = updateIPR("mms", []interface{}{randomRange(7000, 12000), 10, "NOP"}, IPR)
        IPR = updateIPR("kd", []interface{}{0}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("fb", []interface{}{"email[objectobject]__input"}, IPR)
        IPR = updateIPR("kk", []interface{}{0, "password[objectobject]__input"}, IPR)
        IPR = updateIPR("ff", []interface{}{"password[objectobject]__input"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mms", []interface{}{randomRange(5000, 7000), 10, "NOP"}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(275, 285), randomRange(285, 295), "password[objectobject]__input"}, IPR)
        IPR = updateIPR("kd", []interface{}{1}, IPR)
        IPR = updateIPR("fb", []interface{}{"password[objectobject]__input"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(290, 300), randomRange(300, 310), "rememberMetrueinput"}, IPR)
        IPR = updateIPR("mm", []interface{}{randomRange(280, 320), randomRange(20, 65), ""}, IPR)
        IPR = updateIPR("mc", []interface{}{randomRange(290, 300), randomRange(300, 310), "sign-in"}, IPR)

        data := WidgetData{
            Bd:    getRandomItem(screens),
            Jsv:   version,
            Wv:    wVersion,
            Bp:    quickHash(getRandomItem(plugins)),
            Sr:    getRandomItem(fingerprints),
            Didtz: "0",
            Wkr:   getWkr(),
            Flv:   getRandomItem([]string{"true", "false"}),
            Fv:    getRandomItem(video),
            Fa:    getRandomItem(audio),
            Hf:    quickHash(getRandomItem(signatures)),
            Pl:    getRandomItem(platforms),
            Ft:    getRandomItem(touch),
            Fc:    getRandomItem(cookie),
            Fs:    "true",
            Wg:    quickHash(getRandomItem(webGL)),
            Fm:    quickHash(getRandomItem(fonts)),
            Ipr:   IPR,
            Ic:    "0,de;",
            Af:    "",
            Dit:   "",
        }

        widgetResponse := WidgetResponse{
            WidgetData: data,
            Wt:         wt,
        }

        jsonData, err := json.Marshal(widgetResponse)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal JSON"})
            return
        }

        nsgData := nsgmdzxs(string(jsonData))
        c.Data(http.StatusOK, "application/json", []byte(nsgData))
    })

    // Define /checkout route
    router.GET("/checkout", func(c *gin.Context) {
        widgetOne, err := getWidgetData("checkout-regular")
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get widget data for checkout-regular"})
            return
        }

        widgetTwo, err := getWidgetData("checkout-fanwallet")
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get widget data for checkout-fanwallet"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "nuData":          widgetOne,
            "fanWalletNuData": widgetTwo,
        })
    })

    // Define /verifiedfan route
    router.GET("/verifiedfan", func(c *gin.Context) {
        widgetData, err := getWidgetData("verifiedfan")
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get widget data for verifiedfan"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "nuData": widgetData,
        })
    })

    // Start server
    port := getEnv("PORT")
    if port == "" {
        port = "1337"
    }
    if err := router.Run("0.0.0.0:" + port); err != nil {
        log.Fatalf("Failed to run server: %v", err)
    }
}

// getEnv retrieves environment variable or returns empty string
func getEnv(key string) string {
    return os.Getenv(key)
}
