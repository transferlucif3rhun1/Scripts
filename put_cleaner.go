package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Transfer struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type listResponse struct {
	Transfers []Transfer `json:"transfers"`
}

func fetchTransfers(apiKey string) ([]Transfer, error) {
	req, err := http.NewRequest("GET", "https://api.put.io/v2/transfers/list", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var lr listResponse
	err = json.Unmarshal(body, &lr)
	if err != nil {
		return nil, err
	}
	return lr.Transfers, nil
}

func cancelSeeding(apiKey string, id int64) error {
	payload := map[string]string{"transfer_ids": strconv.FormatInt(id, 10)}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", "https://api.put.io/v2/transfers/cancel", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func main() {
	apiKeyFlag := flag.String("api-key", "", "")
	flag.Parse()
	apiKey := *apiKeyFlag
	if apiKey == "" {
		apiKey = "OG2J6ZQ6CVL2X36QQEYY"
	}
	if apiKey == "" {
		log.Fatal("API key required")
	}
	watched := make(map[int64]bool)
	for {
		transfers, err := fetchTransfers(apiKey)
		if err != nil {
			log.Printf("error fetching transfers: %v", err)
			time.Sleep(30 * time.Second)
			continue
		}
		finishingExists := false
		for _, t := range transfers {
			switch t.Status {
			case "FINISHING":
				if !watched[t.ID] {
					log.Printf("transfer %d (%s) is FINISHING", t.ID, t.Name)
					watched[t.ID] = true
				}
				finishingExists = true
			case "SEEDING":
				err := cancelSeeding(apiKey, t.ID)
				if err != nil {
					log.Printf("error cancelling seeding for %d (%s): %v", t.ID, t.Name, err)
				} else {
					log.Printf("cancelled SEEDING for %d (%s)", t.ID, t.Name)
				}
			case "COMPLETED":
				if watched[t.ID] {
					err := cancelSeeding(apiKey, t.ID)
					if err != nil {
						log.Printf("error cancelling seeding for completed %d (%s): %v", t.ID, t.Name, err)
					} else {
						log.Printf("cancelled seeding for COMPLETED %d (%s)", t.ID, t.Name)
					}
					delete(watched, t.ID)
				}
			default:
				log.Printf("transfer %d (%s) status: %s", t.ID, t.Name, t.Status)
			}
		}
		if finishingExists {
			time.Sleep(5 * time.Second)
		} else {
			time.Sleep(10 * time.Second)
		}
	}
}
