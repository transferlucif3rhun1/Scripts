package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/gin-gonic/gin"
)

const (
	AccountPrefix   = "a:"
	StatePrefix     = "s:"
	NumberPrefix    = "n:"
	FilePrefix      = "f:"
	LastFileKey     = "lastfile"
	AccountCountKey = "account_count"
)

type Config struct {
	DatabasePath        string
	Port                string
	CooldownPeriodHours int
	EnableLogs          bool
}

type Account struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	ID       uint32 `json:"id"`
}

type AccountState struct {
	InUse            bool      `json:"in_use"`
	AssignedNumber   string    `json:"assigned_number,omitempty"`
	RequestCount     int       `json:"request_count"`
	CooldownUntil    time.Time `json:"cooldown_until,omitempty"`
	LastAssignedTime time.Time `json:"last_assigned_time,omitempty"`
}

type FileRecord struct {
	Path     string    `json:"path"`
	LastUsed time.Time `json:"last_used"`
	FileHash string    `json:"file_hash,omitempty"`
}

var ErrNoAccountsAvailable = errors.New("no accounts available")

type AccountManager struct {
	db             *badger.DB
	config         Config
	accountsFile   string
	mutex          sync.Mutex
	fileMonitorMtx sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	workerWg       sync.WaitGroup
	logger         *log.Logger
}

func NewAccountManager(config Config) (*AccountManager, error) {
	if err := os.MkdirAll(config.DatabasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	opts := badger.DefaultOptions(config.DatabasePath).
		WithMemTableSize(64 << 20).
		WithNumMemtables(2).
		WithValueLogFileSize(256 << 20).
		WithSyncWrites(false).
		WithLogger(nil)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var logger *log.Logger
	if config.EnableLogs {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	manager := &AccountManager{
		db:     db,
		config: config,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}

	return manager, nil
}

func (m *AccountManager) Close() error {
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Println("All background workers stopped gracefully")
	case <-time.After(5 * time.Second):
		m.logger.Println("Warning: Timed out waiting for some background workers to stop")
	}

	return m.db.Close()
}

func (m *AccountManager) SetAccountsFile(filePath string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.accountsFile = filePath
}

func (m *AccountManager) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func (m *AccountManager) LoadAccountsFromFile() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.accountsFile == "" {
		return fmt.Errorf("no accounts file specified")
	}

	currentStates := make(map[string]AccountState)
	numberToAccountMap := make(map[string]uint32)

	err := m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(NumberPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			numberKey := string(item.Key()[len(NumberPrefix):])

			err := item.Value(func(val []byte) error {
				accountID := binary.LittleEndian.Uint32(val)
				numberToAccountMap[numberKey] = accountID
				return nil
			})
			if err != nil {
				m.logger.Printf("Warning: Failed to read number mapping: %v", err)
				continue
			}
		}

		opts.Prefix = []byte(StatePrefix)
		it = txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			stateKey := string(item.Key())

			var state AccountState
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &state)
			})
			if err != nil {
				m.logger.Printf("Warning: Failed to read account state: %v", err)
				continue
			}

			currentStates[stateKey] = state
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to read current account states: %w", err)
	}

	var isSameFile bool
	lastFile, err := m.GetLastUsedFile()
	if err == nil && lastFile == m.accountsFile {
		isSameFile = true
		m.logger.Printf("Reloading the same file: %s, preserving cooldowns and states", m.accountsFile)
	}

	fileHash, err := m.calculateFileHash(m.accountsFile)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	lastFileHash := ""
	if isSameFile {
		fileRecord, err := m.GetFileStats(m.accountsFile)
		if err == nil && fileRecord != nil {
			lastFileHash = fileRecord.FileHash
		}
	}

	if lastFileHash == fileHash && isSameFile {
		m.logger.Printf("File content has not changed (same hash: %s), skipping reload", fileHash[:8])
		return nil
	}

	if !isSameFile {
		if err := m.clearAllAccounts(); err != nil {
			return fmt.Errorf("failed to clear existing accounts: %w", err)
		}
	} else {
		if err := m.clearAccountsOnly(); err != nil {
			return fmt.Errorf("failed to clear existing account data: %w", err)
		}
	}

	file, err := os.Open(m.accountsFile)
	if err != nil {
		return fmt.Errorf("failed to open accounts file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("account file is empty, please add accounts in email:password format")
	}

	scanner := bufio.NewScanner(file)

	var accountCount uint32 = 0
	const batchSize = 1000
	var accounts []Account
	var loadedAccounts uint32
	var lineCount int

	for scanner.Scan() {
		lineCount++
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			m.logger.Printf("Warning: Invalid line format at line %d: %s", lineCount, line)
			continue
		}

		email := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])

		if email == "" || password == "" {
			m.logger.Printf("Warning: Empty email or password at line %d", lineCount)
			continue
		}

		accountCount++
		accounts = append(accounts, Account{
			Email:    email,
			Password: password,
			ID:       accountCount,
		})

		if len(accounts) >= batchSize {
			if err := m.saveAccountBatch(accounts); err != nil {
				return fmt.Errorf("failed to save account batch: %w", err)
			}
			loadedAccounts += uint32(len(accounts))
			accounts = accounts[:0]
		}
	}

	if len(accounts) > 0 {
		if err := m.saveAccountBatch(accounts); err != nil {
			return fmt.Errorf("failed to save remaining accounts: %w", err)
		}
		loadedAccounts += uint32(len(accounts))
	}

	if loadedAccounts == 0 {
		return fmt.Errorf("no valid accounts found in file, please check the format (email:password)")
	}

	if isSameFile {
		err = m.db.Update(func(txn *badger.Txn) error {
			for stateKey, state := range currentStates {
				parts := strings.Split(stateKey, ":")
				if len(parts) != 2 {
					continue
				}

				idStr := parts[1]
				id, err := strconv.ParseUint(idStr, 10, 32)
				if err != nil {
					continue
				}

				if uint32(id) > accountCount {
					continue
				}

				if !state.CooldownUntil.IsZero() {
					stateData, err := json.Marshal(state)
					if err != nil {
						continue
					}

					if err := txn.Set([]byte(stateKey), stateData); err != nil {
						return err
					}
					m.logger.Printf("Preserved cooldown for account ID %d until %s", id, state.CooldownUntil.Format(time.RFC3339))
				}
			}

			for number, accountID := range numberToAccountMap {
				if accountID > accountCount {
					continue
				}

				numberKey := fmt.Sprintf("%s%s", NumberPrefix, number)
				idBytes := make([]byte, 4)
				binary.LittleEndian.PutUint32(idBytes, accountID)

				if err := txn.Set([]byte(numberKey), idBytes); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			m.logger.Printf("Warning: Failed to restore some account states: %v", err)
		}
	}

	err = m.db.Update(func(txn *badger.Txn) error {
		countBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(countBytes, accountCount)
		return txn.Set([]byte(AccountCountKey), countBytes)
	})
	if err != nil {
		return fmt.Errorf("failed to update account count: %w", err)
	}

	if err := m.recordFileUse(m.accountsFile, fileHash); err != nil {
		return fmt.Errorf("failed to record file use: %w", err)
	}

	m.logger.Printf("Successfully loaded %d accounts from %s", loadedAccounts, m.accountsFile)
	return nil
}

func (m *AccountManager) clearAccountsOnly() error {
	return m.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(AccountPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			if err := txn.Delete(it.Item().Key()); err != nil {
				return err
			}
		}

		countBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(countBytes, 0)
		return txn.Set([]byte(AccountCountKey), countBytes)
	})
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	config := Config{
		DatabasePath:        "accountdb",
		Port:                "3456",
		CooldownPeriodHours: 24,
		EnableLogs:          false,
	}

	manager, err := NewAccountManager(config)
	if err != nil {
		log.Fatalf("Failed to initialize service: %v", err)
	}
	defer manager.Close()

	for {
		displayMenu()
		choice := readInput(reader, "")

		switch choice {
		case "1":
			fmt.Println("\n========== Select New File ==========")
			filePath := selectFile(reader)
			if filePath != "" {
				manager.SetAccountsFile(filePath)
				if err := manager.LoadAccountsFromFile(); err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}
				startServer(manager, &config)
			}

		case "2":
			fmt.Println("\n========== Start Where Left Off ==========")
			lastFile, err := manager.GetLastUsedFile()
			if err != nil {
				fmt.Println("No previous file found. Please select a new file.")
				continue
			}

			if lastFile == "" {
				fmt.Println("No previous file found. Please select a new file.")
				continue
			}

			if _, err := os.Stat(lastFile); os.IsNotExist(err) {
				fmt.Printf("Error: File %s no longer exists. Please select a new file.\n", lastFile)
				continue
			}

			manager.SetAccountsFile(lastFile)
			fmt.Printf("Using last file: %s\n", lastFile)

			if err := manager.LoadAccountsFromFile(); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			startServer(manager, &config)

		case "3":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Println("Invalid choice. Please try again.")
		}
	}
}

func (m *AccountManager) clearAllAccounts() error {
	return m.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			if bytes.HasPrefix(key, []byte(AccountPrefix)) ||
				bytes.HasPrefix(key, []byte(StatePrefix)) ||
				bytes.HasPrefix(key, []byte(NumberPrefix)) {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
		}

		countBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(countBytes, 0)
		return txn.Set([]byte(AccountCountKey), countBytes)
	})
}

func (m *AccountManager) saveAccountBatch(accounts []Account) error {
	return m.db.Update(func(txn *badger.Txn) error {
		for _, account := range accounts {
			accountData, err := json.Marshal(account)
			if err != nil {
				return err
			}

			accountKey := fmt.Sprintf("%s%d", AccountPrefix, account.ID)
			if err := txn.Set([]byte(accountKey), accountData); err != nil {
				return err
			}

			state := AccountState{
				InUse:        false,
				RequestCount: 0,
			}

			stateData, err := json.Marshal(state)
			if err != nil {
				return err
			}

			stateKey := fmt.Sprintf("%s%d", StatePrefix, account.ID)
			if err := txn.Set([]byte(stateKey), stateData); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *AccountManager) recordFileUse(filePath string, fileHash string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	return m.db.Update(func(txn *badger.Txn) error {
		var found bool

		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(FilePrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var record FileRecord
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &record)
			})
			if err != nil {
				continue
			}

			if record.Path == absPath {
				record.LastUsed = time.Now()
				record.FileHash = fileHash
				recordData, err := json.Marshal(record)
				if err != nil {
					return err
				}

				if err := txn.Set(item.Key(), recordData); err != nil {
					return err
				}

				found = true
				break
			}
		}

		if !found {
			record := FileRecord{
				Path:     absPath,
				LastUsed: time.Now(),
				FileHash: fileHash,
			}

			recordData, err := json.Marshal(record)
			if err != nil {
				return err
			}

			fileKey := fmt.Sprintf("%s%d", FilePrefix, 1)
			if err := txn.Set([]byte(fileKey), recordData); err != nil {
				return err
			}
		}

		return txn.Set([]byte(LastFileKey), []byte(absPath))
	})
}

func (m *AccountManager) GetLastUsedFile() (string, error) {
	var lastFile string

	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(LastFileKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		return item.Value(func(val []byte) error {
			lastFile = string(val)
			return nil
		})
	})

	return lastFile, err
}

func (m *AccountManager) GetFileStats(filePath string) (*FileRecord, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	var fileRecord *FileRecord

	err = m.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(FilePrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var record FileRecord

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &record)
			})
			if err != nil {
				continue
			}

			if record.Path == absPath {
				fileRecord = &record
				return nil
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if fileRecord == nil {
		return nil, fmt.Errorf("file stats not found for %s", filePath)
	}

	return fileRecord, nil
}

func (m *AccountManager) GetAccountForNumber(number string, change bool) (*Account, error) {
	if number == "" {
		return nil, fmt.Errorf("number cannot be empty")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	var account *Account

	err := m.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(AccountCountKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return ErrNoAccountsAvailable
			}
			return fmt.Errorf("failed to get account count: %w", err)
		}

		var totalAccounts uint32
		err = item.Value(func(val []byte) error {
			totalAccounts = binary.LittleEndian.Uint32(val)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to read account count: %w", err)
		}

		if totalAccounts == 0 {
			return ErrNoAccountsAvailable
		}

		numberKey := fmt.Sprintf("%s%s", NumberPrefix, number)
		item, err = txn.Get([]byte(numberKey))

		if err == nil {
			var accountID uint32
			err = item.Value(func(val []byte) error {
				accountID = binary.LittleEndian.Uint32(val)
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to read account ID: %w", err)
			}

			if accountID == 0 || accountID > totalAccounts {
				m.logger.Printf("Warning: Invalid account ID %d for number %s, reassigning", accountID, number)
				if err := txn.Delete([]byte(numberKey)); err != nil {
					return fmt.Errorf("failed to delete invalid number mapping: %w", err)
				}
				return m.assignNewAccount(txn, number, now, &account)
			}

			state, err := m.getAccountState(txn, accountID)
			if err != nil {
				return fmt.Errorf("failed to get account state: %w", err)
			}

			if change {
				m.logger.Printf("Change parameter is true for number %s, switching from account ID %d",
					number, accountID)

				if err := txn.Delete([]byte(numberKey)); err != nil {
					return fmt.Errorf("failed to delete number mapping: %w", err)
				}

				state.InUse = false
				state.RequestCount = 0
				state.AssignedNumber = ""

				if err := m.saveAccountState(txn, accountID, state); err != nil {
					return fmt.Errorf("failed to save account state: %w", err)
				}

				return m.assignNewAccount(txn, number, now, &account)
			}

			m.logger.Printf("Using account ID %d for number %s", accountID, number)

			acc, err := m.getAccount(txn, accountID)
			if err != nil {
				return fmt.Errorf("failed to get account: %w", err)
			}

			account = acc
			return nil
		} else if err == badger.ErrKeyNotFound {
			return m.assignNewAccount(txn, number, now, &account)
		}

		return fmt.Errorf("database error: %w", err)
	})

	if err != nil {
		m.logger.Printf("Error in GetAccountForNumber for number %s: %v", number, err)
		return nil, err
	}

	return account, nil
}

func (m *AccountManager) assignNewAccount(txn *badger.Txn, number string, now time.Time, accountOut **Account) error {
	accountID, acc, state, err := m.findAvailableAccount(txn, now)
	if err != nil {
		return err
	}

	if acc == nil {
		return fmt.Errorf("no valid account found to assign")
	}

	state.InUse = true
	state.AssignedNumber = number
	state.RequestCount = 0
	state.LastAssignedTime = now

	m.logger.Printf("Assigning new account ID %d to number %s", accountID, number)

	if err := m.saveAccountState(txn, accountID, state); err != nil {
		return fmt.Errorf("failed to save account state: %w", err)
	}

	numberKey := fmt.Sprintf("%s%s", NumberPrefix, number)
	idBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(idBytes, accountID)
	if err := txn.Set([]byte(numberKey), idBytes); err != nil {
		return fmt.Errorf("failed to map number to account: %w", err)
	}

	*accountOut = acc
	return nil
}

func (m *AccountManager) getAccountState(txn *badger.Txn, accountID uint32) (AccountState, error) {
	stateKey := fmt.Sprintf("%s%d", StatePrefix, accountID)
	item, err := txn.Get([]byte(stateKey))
	if err != nil {
		return AccountState{}, fmt.Errorf("state not found for account %d: %w", accountID, err)
	}

	var state AccountState
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &state)
	})
	if err != nil {
		return AccountState{}, fmt.Errorf("failed to unmarshal state for account %d: %w", accountID, err)
	}

	return state, nil
}

func (m *AccountManager) saveAccountState(txn *badger.Txn, accountID uint32, state AccountState) error {
	stateData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	stateKey := fmt.Sprintf("%s%d", StatePrefix, accountID)
	return txn.Set([]byte(stateKey), stateData)
}

func (m *AccountManager) getAccount(txn *badger.Txn, accountID uint32) (*Account, error) {
	accountKey := fmt.Sprintf("%s%d", AccountPrefix, accountID)
	item, err := txn.Get([]byte(accountKey))
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	var account Account
	err = item.Value(func(val []byte) error {
		return json.Unmarshal(val, &account)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %w", err)
	}

	return &account, nil
}

func (m *AccountManager) findAvailableAccount(txn *badger.Txn, now time.Time) (uint32, *Account, AccountState, error) {
	item, err := txn.Get([]byte(AccountCountKey))
	if err != nil {
		return 0, nil, AccountState{}, fmt.Errorf("failed to get account count: %w", err)
	}

	var totalAccounts uint32
	err = item.Value(func(val []byte) error {
		totalAccounts = binary.LittleEndian.Uint32(val)
		return nil
	})
	if err != nil {
		return 0, nil, AccountState{}, fmt.Errorf("failed to read account count: %w", err)
	}

	if totalAccounts == 0 {
		return 0, nil, AccountState{}, ErrNoAccountsAvailable
	}

	type accountCandidate struct {
		id    uint32
		state AccountState
		acc   Account
	}

	var availableAccounts []accountCandidate
	var inCooldownCount int

	for id := uint32(1); id <= totalAccounts; id++ {
		stateKey := fmt.Sprintf("%s%d", StatePrefix, id)
		stateItem, err := txn.Get([]byte(stateKey))
		if err != nil {
			continue
		}

		var state AccountState
		err = stateItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &state)
		})
		if err != nil {
			continue
		}

		if !state.CooldownUntil.IsZero() && now.Before(state.CooldownUntil) {
			inCooldownCount++
			continue
		}

		if state.InUse {
			continue
		}

		accountKey := fmt.Sprintf("%s%d", AccountPrefix, id)
		accountItem, err := txn.Get([]byte(accountKey))
		if err != nil {
			continue
		}

		var acc Account
		err = accountItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &acc)
		})
		if err != nil {
			continue
		}

		availableAccounts = append(availableAccounts, accountCandidate{
			id:    id,
			state: state,
			acc:   acc,
		})
	}

	if len(availableAccounts) == 0 {
		if inCooldownCount > 0 {
			m.logger.Printf("All accounts are in cooldown (%d of %d accounts)", inCooldownCount, totalAccounts)
		} else {
			m.logger.Printf("No available accounts found out of %d total accounts", totalAccounts)
		}
		return 0, nil, AccountState{}, ErrNoAccountsAvailable
	}

	sort.Slice(availableAccounts, func(i, j int) bool {
		if availableAccounts[i].state.LastAssignedTime.IsZero() {
			return true
		}
		if availableAccounts[j].state.LastAssignedTime.IsZero() {
			return false
		}
		return availableAccounts[i].state.LastAssignedTime.Before(availableAccounts[j].state.LastAssignedTime)
	})

	selected := availableAccounts[0]
	return selected.id, &selected.acc, selected.state, nil
}

func (m *AccountManager) cleanupExpiredCooldowns() {
	now := time.Now()
	m.logger.Println("Running cooldown cleanup task...")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	err := m.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(StatePrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		var updated int
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var state AccountState

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &state)
			})
			if err != nil {
				continue
			}

			if !state.CooldownUntil.IsZero() && now.After(state.CooldownUntil) {
				state.CooldownUntil = time.Time{}

				stateData, err := json.Marshal(state)
				if err != nil {
					continue
				}

				if err := txn.Set(item.Key(), stateData); err != nil {
					continue
				}
				updated++
			}
		}

		if updated > 0 {
			m.logger.Printf("Cleared cooldown for %d accounts", updated)
		}

		return nil
	})

	if err != nil {
		m.logger.Printf("Error in cooldown cleanup: %v", err)
	}
}

func (m *AccountManager) StartFileMonitorWorker(interval time.Duration) {
	m.workerWg.Add(1)
	go func() {
		defer m.workerWg.Done()

		timer := time.NewTicker(interval)
		defer timer.Stop()

		var lastHash string

		for {
			select {
			case <-timer.C:
				if m.accountsFile == "" {
					continue
				}

				func() {
					m.fileMonitorMtx.Lock()
					defer m.fileMonitorMtx.Unlock()

					fileInfo, err := os.Stat(m.accountsFile)
					if err != nil {
						return
					}

					if fileInfo.Size() == 0 {
						return
					}

					hash, err := m.calculateFileHash(m.accountsFile)
					if err != nil {
						m.logger.Printf("Error calculating file hash: %v", err)
						return
					}

					if hash == lastHash {
						return
					}

					currentFileHash := ""
					fileRecord, err := m.GetFileStats(m.accountsFile)
					if err == nil && fileRecord != nil {
						currentFileHash = fileRecord.FileHash
					}

					if hash != currentFileHash {
						oldHashDisplay := "N/A"
						if len(currentFileHash) >= 8 {
							oldHashDisplay = currentFileHash[:8]
						}
						m.logger.Printf("Detected change in accounts file %s (hash changed: %s -> %s)",
							m.accountsFile, oldHashDisplay, hash[:8])

						if err := m.LoadAccountsFromFile(); err != nil {
							m.logger.Printf("Error reloading accounts file after change: %v", err)
						} else {
							m.logger.Printf("Successfully reloaded accounts after file change")
						}
					}

					lastHash = hash
				}()
			case <-m.ctx.Done():
				m.logger.Println("File monitor worker stopped")
				return
			}
		}
	}()

	m.logger.Printf("File monitor worker started (interval: %v)", interval)
}

func (m *AccountManager) RunCleanupTask(interval time.Duration) {
	m.workerWg.Add(1)
	go func() {
		defer m.workerWg.Done()

		timer := time.NewTicker(interval)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				m.cleanupExpiredCooldowns()
			case <-m.ctx.Done():
				m.logger.Println("Cooldown cleanup worker stopped")
				return
			}
		}
	}()

	m.logger.Printf("Cooldown cleanup worker started (interval: %v)", interval)
}

type AccountRequest struct {
	Number string `json:"number" binding:"required"`
	Change bool   `json:"change"`
}

type AccountResponse struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Success  bool   `json:"success"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func readInput(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Error reading input: %v", err)
	}
	return strings.TrimSpace(input)
}

func selectFile(reader *bufio.Reader) string {
	fmt.Print("Enter path to accounts file: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	path := strings.TrimSpace(input)

	if path == "" {
		fmt.Println("Path cannot be empty.")
		return ""
	}

	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		fmt.Printf("Warning: File %s does not exist. Do you want to create it? (y/n): ", path)
		createInput, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Error reading input: %v", err)
		}

		createInput = strings.TrimSpace(createInput)
		if strings.ToLower(createInput) == "y" {
			file, err := os.Create(path)
			if err != nil {
				log.Fatalf("Error creating file: %v", err)
			}
			file.Close()
			fmt.Println("Empty file created. Please add accounts in email:password format.")
			fmt.Println("Example format: user@example.com:password123")
		} else {
			fmt.Println("Operation cancelled.")
			return ""
		}
	} else if err == nil {
		if fileInfo.IsDir() {
			fmt.Printf("Error: %s is a directory, not a file.\n", path)
			return ""
		}

		if fileInfo.Size() == 0 {
			fmt.Printf("Warning: File %s is empty. Do you want to use it anyway? (y/n): ", path)
			useInput, err := reader.ReadString('\n')
			if err != nil {
				log.Fatalf("Error reading input: %v", err)
			}

			useInput = strings.TrimSpace(useInput)
			if strings.ToLower(useInput) != "y" {
				fmt.Println("Operation cancelled.")
				return ""
			}

			fmt.Println("Using empty file. Please add accounts in email:password format.")
			fmt.Println("Example format: user@example.com:password123")
		} else {
			file, err := os.Open(path)
			if err != nil {
				log.Printf("Error opening file: %v", err)
			} else {
				defer file.Close()

				scanner := bufio.NewScanner(file)
				lineCount := 0
				validCount := 0

				for scanner.Scan() && lineCount < 5 {
					line := scanner.Text()
					lineCount++

					if line == "" {
						continue
					}

					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
						validCount++
					}
				}

				if validCount == 0 && lineCount > 0 {
					fmt.Println("Warning: No valid account entries found in preview.")
					fmt.Println("File should contain accounts in email:password format.")
					fmt.Println("Example format: user@example.com:password123")
					fmt.Printf("Do you want to use this file anyway? (y/n): ")

					useInput, err := reader.ReadString('\n')
					if err != nil {
						log.Fatalf("Error reading input: %v", err)
					}

					useInput = strings.TrimSpace(useInput)
					if strings.ToLower(useInput) != "y" {
						fmt.Println("Operation cancelled.")
						return ""
					}
				}
			}
		}
	} else {
		fmt.Printf("Error checking file: %v\n", err)
		return ""
	}

	return path
}

func displayMenu() {
	fmt.Println("\n========== Account API Manager ==========")
	fmt.Println("1) Select New File")
	fmt.Println("2) Start Where Left Off")
	fmt.Println("3) Exit")
	fmt.Print("Enter your choice (1-3): ")
}

func startServer(manager *AccountManager, config *Config) {
	var accountCount uint32
	err := manager.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(AccountCountKey))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			accountCount = binary.LittleEndian.Uint32(val)
			return nil
		})
	})

	if err != nil && config.EnableLogs {
		log.Printf("Error checking account count: %v", err)
	}

	if config.EnableLogs {
		filePath := manager.accountsFile
		fileStats, err := manager.GetFileStats(filePath)

		fmt.Println("\n===== Server Information =====")
		fmt.Printf("File Path: %s\n", filePath)
		fmt.Printf("Account Count: %d\n", accountCount)

		if err == nil && fileStats != nil {
			fmt.Printf("Last Used: %s\n", fileStats.LastUsed.Format("2006-01-02 15:04:05"))
		}
		fmt.Println("=============================\n")

		var cooldownCount int
		manager.db.View(func(txn *badger.Txn) error {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = []byte(StatePrefix)
			it := txn.NewIterator(opts)
			defer it.Close()

			for it.Rewind(); it.Valid(); it.Next() {
				var state AccountState
				err := it.Item().Value(func(val []byte) error {
					return json.Unmarshal(val, &state)
				})
				if err != nil {
					continue
				}

				if !state.CooldownUntil.IsZero() && time.Now().Before(state.CooldownUntil) {
					cooldownCount++
				}
			}
			return nil
		})

		fmt.Printf("Accounts in cooldown: %d\n", cooldownCount)
		fmt.Printf("Accounts available: %d\n\n", int(accountCount)-cooldownCount)

		if accountCount == 0 {
			fmt.Println("\n⚠️  WARNING: No accounts loaded in the database!")
			fmt.Println("The API will return 503 errors for all requests.")
			fmt.Println("Please check your accounts file and make sure it contains valid accounts.")
			fmt.Print("Do you want to continue anyway? (y/n): ")

			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(response)

			if strings.ToLower(response) != "y" {
				fmt.Println("Server startup cancelled.")
				return
			}
		}
	} else {
		if accountCount == 0 {
			fmt.Println("\n⚠️  WARNING: No accounts loaded in the database!")
			fmt.Println("The API will return 503 errors for all requests.")
			fmt.Print("Do you want to continue anyway? (y/n): ")

			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(response)

			if strings.ToLower(response) != "y" {
				fmt.Println("Server startup cancelled.")
				return
			}
		}
	}

	manager.RunCleanupTask(5 * time.Minute)
	manager.StartFileMonitorWorker(30 * time.Second)

	_, serverCancel := context.WithCancel(context.Background())

	ln, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		fmt.Printf("\n⚠️  Port %s is already in use!\n", config.Port)
		fmt.Println("Please choose a different port or stop the existing server.")
		fmt.Print("Enter a new port (or leave empty to cancel): ")

		reader := bufio.NewReader(os.Stdin)
		newPort, _ := reader.ReadString('\n')
		newPort = strings.TrimSpace(newPort)

		if newPort == "" {
			fmt.Println("Server startup cancelled.")
			serverCancel()
			return
		}

		config.Port = newPort
		ln, err = net.Listen("tcp", ":"+config.Port)
		if err != nil {
			fmt.Printf("Port %s is also in use. Server startup cancelled.\n", config.Port)
			serverCancel()
			return
		}
	}
	ln.Close()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	setupAPI(router, manager)

	srv := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	serverError := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	fmt.Printf("\nServer started on port %s\n", config.Port)
	fmt.Println("\nType 'stop' and press Enter to stop the server and return to menu...")

	inputCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				continue
			}

			input = strings.TrimSpace(input)
			if strings.ToLower(input) == "stop" || input == "" {
				inputCh <- input
				return
			}
		}
	}()

	select {
	case input := <-inputCh:
		if input == "" {
			fmt.Println("Empty input received, stopping server...")
		} else {
			fmt.Println("Stopping server...")
		}
	case err := <-serverError:
		fmt.Printf("\nServer stopped due to error: %v\n", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if config.EnableLogs {
			log.Printf("Server forced to shutdown: %v", err)
		}
	}

	srv.Close()
	serverCancel()
	fmt.Println("Returning to menu...")
}

func setupAPI(router *gin.Engine, manager *AccountManager) {
	router.POST("/accounts", func(c *gin.Context) {
		var request AccountRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(400, ErrorResponse{Error: "Invalid request format"})
			return
		}

		if request.Number == "" {
			c.JSON(400, ErrorResponse{Error: "Number cannot be empty"})
			return
		}

		account, err := manager.GetAccountForNumber(request.Number, request.Change)
		if err != nil {
			if errors.Is(err, ErrNoAccountsAvailable) {
				c.JSON(503, ErrorResponse{Error: "All accounts are currently in cooldown, please try again later"})
			} else {
				errorMsg := fmt.Sprintf("Error processing request: %v", err)
				c.JSON(500, ErrorResponse{Error: errorMsg})
			}
			return
		}

		if account == nil {
			c.JSON(500, ErrorResponse{Error: "Internal server error: No account found"})
			return
		}

		if account.Email == "" || account.Password == "" {
			c.JSON(500, ErrorResponse{Error: "Internal server error: Invalid account data"})
			return
		}

		c.JSON(200, AccountResponse{
			Email:    account.Email,
			Password: account.Password,
			Success:  true,
		})
	})
}
