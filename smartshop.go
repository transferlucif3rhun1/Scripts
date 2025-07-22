package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const rsaPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCRn+iPY/ENsTQpLsyIDPK/HRzv
irt81Wc8Nl9Iv/Vt10hSsefW98j1vo0RaBOYUYpVeSaM13C/r0LqSFkF/gC6t5vr
U3bJ6vLfLg9IDx33h+G5aT78ZHyVdj1VBiJBIQxmd9tV+xphm1dQsptZEzJ2t/0Y
7U7BSRu35ERVxi+HzwIDAQAB
-----END PUBLIC KEY-----`

func main() {
	gin.SetMode(gin.ReleaseMode)

	// Use gin.New() to avoid the default logger middleware that produces [GIN] logs.
	r := gin.New()

	// Add only the recovery middleware to handle panics without logging requests.
	r.Use(gin.Recovery())

	r.GET("/smartshop", func(c *gin.Context) {
		// Generate timestamp in the same format as JavaScript
		now := time.Now().Format("2006-01-02 15:04:05")
		guid := uuid.New().String()
		prefix := "ss_android_mobile_1k"

		// Create the random string in the same format as JavaScript
		randomString := prefix + "#" + now + "#" + guid

		// Load RSA public key
		pubKey, err := loadRSAPublicKeyFromPEM([]byte(rsaPublicKeyPEM))
		if err != nil {
			log.Printf("Error loading public key: %v", err)
			c.String(http.StatusInternalServerError, "Error loading public key")
			return
		}

		// Encrypt the random string using RSA with PKCS1 padding
		encryptedBytes, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, []byte(randomString))
		if err != nil {
			log.Printf("Error encrypting: %v", err)
			c.String(http.StatusInternalServerError, "Error encrypting text")
			return
		}

		// Convert encrypted data to Base64
		encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedBytes)

		// Combine prefix and encrypted text (matching JavaScript logic)
		finalText := prefix + ":" + encryptedBase64

		// Base64 encode the final result
		finalBase64 := base64.StdEncoding.EncodeToString([]byte(finalText))

		// Return only the final base64 string
		c.String(http.StatusOK, finalBase64)
	})

	// Log server start
	log.Println("Server started on port 3455")

	// Start the server and log fatal errors if any
	if err := r.Run(":4616"); err != nil {
		log.Fatalf("Server exited with error: %v", err)
	}
}

func loadRSAPublicKeyFromPEM(pubPEM []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, x509.CertificateInvalidError{}
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	pubKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return nil, x509.CertificateInvalidError{}
	}

	return pubKey, nil
}
