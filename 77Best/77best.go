package main

import (
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	EncryptionKey = "E7wQ#@%wfXfdAnQMT%@77vMu"
	IV            = "3C@L4Xx!"
)

type ReqData struct {
	AppID          string `json:"appId"`
	CaptchaID      string `json:"captchaId"`
	Password       string `json:"password"`
	Platform       string `json:"platform"`
	Username       string `json:"username"`
	VerifyCode     string `json:"verifyCode"`
	PlayerName     string `json:"playerName,omitempty"`
	ReferralCode   string `json:"referralCode,omitempty"`
	RepeatPassword string `json:"repeatPassword,omitempty"`
}

type Payload struct {
	Sign          string  `json:"sign"`
	Device        string  `json:"device"`
	TimeStamp     int64   `json:"timeStamp"`
	ReqData       ReqData `json:"reqData"`
	IP            string  `json:"ip"`
	AppID         string  `json:"appId"`
	Platform      string  `json:"platform"`
	GpsAdid       string  `json:"gpsAdid"`
	Adid          string  `json:"adid"`
	Client        string  `json:"client"`
	FirebaseToken string  `json:"firebaseToken"`
	Version       string  `json:"Version"`
}

type EncryptRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Type         string `json:"type"`
	ReferralCode string `json:"referralCode"`
}

type DecryptRequest struct {
	Data string `json:"data"`
}

type SuccessResponse struct {
	EncryptedData string `json:"encryptedData"`
}

type DecryptSuccessResponse struct {
	DecryptedData string `json:"decryptedData"`
}

type ErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    int    `json:"code"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.Use(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Success: false,
					Error:   "Internal server error",
					Code:    500,
				})
				c.Abort()
			}
		}()
		c.Next()
	})

	r.POST("/encrypt/login", encryptLoginHandler)
	r.GET("/encrypt/token", encryptTokenHandler)
	r.POST("/decrypt", decryptHandler)

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Success: false,
			Error:   "Endpoint not found",
			Code:    404,
		})
	})

	fmt.Println("Server started")
	r.Run(":8080")
}

func encryptLoginHandler(c *gin.Context) {
	var req EncryptRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Invalid JSON format",
			Code:    400,
		})
		return
	}

	if req.Username == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Username is required",
			Code:    400,
		})
		return
	}

	if req.Password == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Password is required",
			Code:    400,
		})
		return
	}

	formattedUsername, err := validateAndFormatUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   err.Error(),
			Code:    400,
		})
		return
	}

	if len(req.Password) < 4 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Password must be at least 4 characters",
			Code:    400,
		})
		return
	}

	encrypted, err := encryptLogin(formattedUsername, req.Password, req.Type, req.ReferralCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   "Encryption failed",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		EncryptedData: encrypted,
	})
}

func encryptTokenHandler(c *gin.Context) {
	encrypted, err := encryptToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Error:   "Token encryption failed",
			Code:    500,
		})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{
		EncryptedData: encrypted,
	})
}

func decryptHandler(c *gin.Context) {
	var req DecryptRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Invalid JSON format",
			Code:    400,
		})
		return
	}

	if req.Data == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Success: false,
			Error:   "Data field is required",
			Code:    400,
		})
		return
	}

	decrypted, err := decrypt3DES(req.Data)
	if err != nil {
		var errorMsg string
		var code int

		if strings.Contains(err.Error(), "illegal base64") {
			errorMsg = "Invalid encrypted data format"
			code = 400
		} else if strings.Contains(err.Error(), "invalid padding") {
			errorMsg = "Decryption failed - corrupted data"
			code = 400
		} else {
			errorMsg = "Decryption failed"
			code = 500
		}

		c.JSON(http.StatusOK, ErrorResponse{
			Success: false,
			Error:   errorMsg,
			Code:    code,
		})
		return
	}

	unescapedData := decrypted
	if len(decrypted) > 0 && decrypted[0] == '"' && decrypted[len(decrypted)-1] == '"' {
		if unquoted, err := strconv.Unquote(decrypted); err == nil {
			unescapedData = unquoted
		}
	}

	c.JSON(http.StatusOK, DecryptSuccessResponse{
		DecryptedData: unescapedData,
	})
}

func validateAndFormatUsername(username string) (string, error) {
	if username == "" {
		return "", fmt.Errorf("username cannot be empty")
	}

	username = strings.TrimSpace(username)
	username = strings.ReplaceAll(username, " ", "")
	username = strings.ReplaceAll(username, "-", "")

	digitRegex := regexp.MustCompile(`^\d+$`)

	if strings.HasPrefix(username, "+91") {
		phoneNumber := username[3:]
		if len(phoneNumber) != 10 || !digitRegex.MatchString(phoneNumber) {
			return "", fmt.Errorf("invalid phone number format - must be +91 followed by 10 digits")
		}
		return phoneNumber, nil
	}

	if strings.HasPrefix(username, "91") && len(username) == 12 {
		phoneNumber := username[2:]
		if !digitRegex.MatchString(phoneNumber) {
			return "", fmt.Errorf("invalid phone number format - must be 91 followed by 10 digits")
		}
		return phoneNumber, nil
	}

	if len(username) == 10 && digitRegex.MatchString(username) {
		return username, nil
	}

	if len(username) == 11 && strings.HasPrefix(username, "0") && digitRegex.MatchString(username) {
		return username[1:], nil
	}

	return "", fmt.Errorf("invalid phone number format - must be 10 digits, 91+10 digits, or +91+10 digits")
}

func md5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func encryptLogin(username, password, reqType, referralCode string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	hashedPassword := md5Hash(password)
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	reqData := ReqData{
		AppID:      "1000001",
		CaptchaID:  "",
		Password:   hashedPassword,
		Platform:   "winers_ly01",
		Username:   username,
		VerifyCode: "",
	}

	if reqType == "register" {
		reqData.PlayerName = username
		reqData.ReferralCode = referralCode
		reqData.RepeatPassword = hashedPassword
	}

	reqDataJSON, err := marshalSorted(reqData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request data")
	}

	signatureString := "1000001" + strconv.FormatInt(timestamp, 10) + string(reqDataJSON)
	signature := md5Hash(signatureString)

	payload := Payload{
		Sign:          signature,
		Device:        "",
		TimeStamp:     timestamp,
		ReqData:       reqData,
		IP:            "",
		AppID:         "1000001",
		Platform:      "winers_ly01",
		GpsAdid:       "",
		Adid:          "",
		Client:        "",
		FirebaseToken: "",
		Version:       "1.0.32",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload")
	}

	encrypted, err := encrypt3DES(string(payloadJSON))
	if err != nil {
		return "", fmt.Errorf("encryption failed")
	}

	return encrypted, nil
}

func encryptToken() (string, error) {
	defer func() {
		if r := recover(); r != nil {
			return
		}
	}()

	timestamp := time.Now().UnixNano() / int64(time.Millisecond)

	emptyReqData := map[string]interface{}{}
	reqDataJSON, _ := json.Marshal(emptyReqData)

	signatureString := "1000001" + strconv.FormatInt(timestamp, 10) + string(reqDataJSON)
	signature := md5Hash(signatureString)

	payload := map[string]interface{}{
		"sign":          signature,
		"device":        "",
		"timeStamp":     timestamp,
		"reqData":       emptyReqData,
		"ip":            "",
		"appId":         "1000001",
		"platform":      "winers_ly01",
		"gpsAdid":       "",
		"adid":          "",
		"client":        "",
		"firebaseToken": "",
		"Version":       "1.0.32",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload")
	}

	encrypted, err := encrypt3DES(string(payloadJSON))
	if err != nil {
		return "", fmt.Errorf("encryption failed")
	}

	return encrypted, nil
}

func encrypt3DES(plaintext string) (string, error) {
	if plaintext == "" {
		return "", fmt.Errorf("plaintext cannot be empty")
	}

	key := []byte(EncryptionKey)
	iv := []byte(IV)

	if len(key) != 24 {
		return "", fmt.Errorf("invalid key length")
	}

	if len(iv) != 8 {
		return "", fmt.Errorf("invalid IV length")
	}

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher")
	}

	paddedData := pkcs7Pad([]byte(plaintext), des.BlockSize)
	ciphertext := make([]byte, len(paddedData))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedData)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt3DES(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", fmt.Errorf("ciphertext cannot be empty")
	}

	key := []byte(EncryptionKey)
	iv := []byte(IV)

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("illegal base64 data")
	}

	if len(data) == 0 {
		return "", fmt.Errorf("decoded data is empty")
	}

	if len(data)%des.BlockSize != 0 {
		return "", fmt.Errorf("invalid block size")
	}

	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher")
	}

	plaintext := make([]byte, len(data))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, data)

	unpaddedData, err := pkcs7Unpad(plaintext)
	if err != nil {
		return "", err
	}

	return string(unpaddedData), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(data, padtext...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 || padding > 8 {
		return nil, fmt.Errorf("invalid padding")
	}

	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:len(data)-padding], nil
}

func marshalSorted(reqData ReqData) ([]byte, error) {
	data := make(map[string]interface{})

	if reqData.AppID != "" {
		data["appId"] = reqData.AppID
	}
	data["captchaId"] = reqData.CaptchaID
	if reqData.Password != "" {
		data["password"] = reqData.Password
	}
	if reqData.Platform != "" {
		data["platform"] = reqData.Platform
	}
	if reqData.PlayerName != "" {
		data["playerName"] = reqData.PlayerName
	}
	if reqData.PlayerName != "" || reqData.ReferralCode != "" {
		data["referralCode"] = reqData.ReferralCode
	}
	if reqData.RepeatPassword != "" {
		data["repeatPassword"] = reqData.RepeatPassword
	}
	if reqData.Username != "" {
		data["username"] = reqData.Username
	}
	data["verifyCode"] = reqData.VerifyCode

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var jsonParts []string
	for _, k := range keys {
		value := data[k]
		var valueStr string

		switch v := value.(type) {
		case string:
			valueStr = fmt.Sprintf(`"%s"`, strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`))
		case nil:
			valueStr = `""`
		default:
			valueBytes, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			valueStr = string(valueBytes)
		}

		jsonParts = append(jsonParts, fmt.Sprintf(`"%s":%s`, k, valueStr))
	}

	result := "{" + strings.Join(jsonParts, ",") + "}"
	return []byte(result), nil
}
