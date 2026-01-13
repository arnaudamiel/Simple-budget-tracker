package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Configuration constants
const (
	port                = ":8910"
	httpsPort           = ":8911"
	dbFile              = "budget.dat"
	usersFile           = "users"
	logDir              = "/var/log/budget"
	logFile             = logDir + "/transactions.csv"
	unauthLogFile       = logDir + "/unauthorized.log"
	certFile            = "cert.pem"
	keyFile             = "key.pem"
	maxBalance    int32 = 2000000000 // Cap at ~£20m to prevent overflow wrapping in 32-bit math
)

// ThreadSafeLogger is a wrapper around os.File that ensures atomic writes
// to a log file from multiple goroutines.
type ThreadSafeLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewLogger creates specific logger for a given filename.
// Opens file in append mode.
func NewLogger(filename string) (*ThreadSafeLogger, error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &ThreadSafeLogger{file: f}, nil
}

// Log writes a formatted string to the file with mutex protection.
func (l *ThreadSafeLogger) Log(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.file, format, args...)
}

// Close closes the underlying file handle.
func (l *ThreadSafeLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.file.Close()
}

// Server holds the application state.
// It uses a mutex to protect the shared 'balance' and 'budget' variables.
//
// Fields:
// - mu: Mutex for thread-safe access to balance and budget.
// - balance: Current account balance in pence.
// - budget: Target budget in pence.
// - users: Map of authorized user IDs.
// - transLogger: Logger for financial transactions.
// - unauthLogger: Logger for unauthorized access attempts.
type Server struct {
	mu           sync.Mutex
	balance      int32 // Current account balance in pence
	budget       int32 // Stores the initial budget
	users        map[string]bool
	transLogger  *ThreadSafeLogger
	unauthLogger *ThreadSafeLogger
}

// SetRequest defines the JSON payload for setting the absolute balance.
type SetRequest struct {
	Amount int32 `json:"amount"`
}

// SpendRequest defines the JSON payload for spending (reducing) the balance.
type SpendRequest struct {
	Amount int32 `json:"amount"`
}

// SetBudgetRequest defines the JSON payload for setting the budget.
type SetBudgetRequest struct {
	Budget int32 `json:"budget"`
}

// GetResponse defines the JSON response for the get endpoint.
type GetResponse struct {
	Balance int32 `json:"balance"`
	Budget  int32 `json:"budget"`
}

func main() {
	// Initialize Loggers (thread-safe for concurrent access)
	tl, err := NewLogger(logFile)
	if err != nil {
		log.Fatalf("Failed to open transaction log: %v", err)
	}
	defer tl.Close()

	ul, err := NewLogger(unauthLogFile)
	if err != nil {
		log.Fatalf("Failed to open unauthorized log: %v", err)
	}
	defer ul.Close()

	// Initialize Server state
	srv := &Server{
		users:        make(map[string]bool),
		transLogger:  tl,
		unauthLogger: ul,
	}

	// Load valid users whitelist
	if err := srv.loadUsers(); err != nil {
		log.Fatalf("Failed to load users: %v", err)
	}

	// Load existing balance/budget from disk
	if err := srv.loadData(); err != nil {
		log.Printf("Warning: Failed to load data (starting at 0): %v", err)
	}

	// Route Handlers with Auth Middleware
	http.HandleFunc("/get", srv.authMiddleware(srv.handleGet))
	http.HandleFunc("/set", srv.authMiddleware(srv.handleSet))
	http.HandleFunc("/spend", srv.authMiddleware(srv.handleSpend))
	http.HandleFunc("/set_budget", srv.authMiddleware(srv.handleSetBudget))

	// start the HTTP server in a background goroutine
	go func() {
		log.Printf("HTTP Server listening on %s", port)
		if err := http.ListenAndServe(port, nil); err != nil {
			log.Fatalf("HTTP Server failed: %v", err)
		}
	}()

	// Check for SSL certificates to optionally start HTTPS server
	// This enables PWA installation on mobile devices.
	if _, err := os.Stat(certFile); err == nil {
		log.Printf("HTTPS Server listening on %s", httpsPort)
		if err := http.ListenAndServeTLS(httpsPort, certFile, keyFile, nil); err != nil {
			log.Fatalf("HTTPS Server failed: %v", err)
		}
	} else {
		log.Println("No cert.pem/key.pem found. HTTPS disabled. Running in HTTP-only mode.")
		// Block forever to keep the main goroutine alive
		select {}
	}
}

// loadUsers reads the 'users' whitelist file into a map.
func (s *Server) loadUsers() error {
	file, err := os.Open(usersFile)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		user := strings.TrimSpace(scanner.Text())
		if user != "" {
			s.users[user] = true
		}
	}
	return scanner.Err()
}

// loadData reads the data from disk.
// Supports migration: 4 bytes (Balance) -> 8 bytes (Balance + Budget).
// Returns nil if file doesn't exist (initial state).
func (s *Server) loadData() error {
	data, err := os.ReadFile(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			s.balance = 0
			s.budget = 0
			return nil
		}
		return err
	}

	if len(data) == 4 {
		// Migration: Old format (Balance only)
		s.balance = int32(binary.LittleEndian.Uint32(data))
		s.budget = 0
		log.Println("Migrated database from 4 bytes to 8 bytes (added default Budget: 0)")
		return s.saveData() // immediately save in new format
	} else if len(data) == 8 {
		// New format: Balance (4) + Budget (4)
		s.balance = int32(binary.LittleEndian.Uint32(data[0:4]))
		s.budget = int32(binary.LittleEndian.Uint32(data[4:8]))
		return nil
	}

	return fmt.Errorf("invalid data length: %d", len(data))
}

// saveData writes the current balance and budget to disk as 8 bytes little-endian.
//
// TODO: Implement atomic save to prevent data corruption during a crash.
// Current implementation truncates the file before writing perfectly, which is risky.
// Proposed fix:
// 1. Write data to a temporary file (e.g., budget.dat.tmp).
// 2. Sync the temp file to disk.
// 3. Rename the temp file to dbFile (atomic operation on POSIX).
func (s *Server) saveData() error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], uint32(s.balance))
	binary.LittleEndian.PutUint32(data[4:8], uint32(s.budget))

	f, err := os.OpenFile(dbFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync() // Ensure data is flushed to physical disk
}

// authMiddleware enforces presence of a valid 'Authorization' header.
// Responds with 401 Unauthorized if the user is not in the whitelist.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS headers for local testing convenience
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		user := r.Header.Get("Authorization")
		if user == "" || !s.users[user] {
			s.logUnauthorized(user, r.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// handleGet returns the current balance and budget as JSON.
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	resp := GetResponse{
		Balance: s.balance,
		Budget:  s.budget,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSet sets the balance to a specific absolute value.
func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.Amount > maxBalance {
		http.Error(w, "Amount exceeds limit", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.balance = req.Amount
	if err := s.saveData(); err != nil {
		log.Printf("Error saving data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log the SET action
	user := r.Header.Get("Authorization")
	s.logTransaction(user, "SET", req.Amount)

	fmt.Fprintf(w, "%d", s.balance) // Keep returning raw int for now, or update to JSON?
	// Requirement implies app overhaul. The client expects raw int from /spend /set based on old code.
	// I should probably keep it consistent or updated.
	// Old client code: updateDisplay(parseInt(text, 10));
	// So returning raw int is safer for /set and /spend until I update client fully.
	// But /get must return JSON.
}

// handleSpend subtracts an amount from the balance.
func (s *Server) handleSpend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SpendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Overflow/Data Safety Check
	// Prevent massive transactions that could overflow int32 or are unreasonable.
	if req.Amount > 100000000 || req.Amount < -100000000 { // Limit single transaction to ~£1m
		http.Error(w, "Transaction too large", http.StatusBadRequest)
		return
	}

	s.balance -= req.Amount
	if err := s.saveData(); err != nil {
		log.Printf("Error saving data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log the SPEND action
	user := r.Header.Get("Authorization")
	s.logTransaction(user, "SPEND", req.Amount)

	fmt.Fprintf(w, "%d", s.balance)
}

// handleSetBudget sets the budget and adjusts the balance.
func (s *Server) handleSetBudget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SetBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Basic validation: Budget must be positive and reasonable
	if req.Budget < 0 || req.Budget > maxBalance {
		http.Error(w, "Invalid budget amount", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	oldBudget := s.budget
	diff := req.Budget - oldBudget

	s.budget = req.Budget
	s.balance += diff

	if err := s.saveData(); err != nil {
		log.Printf("Error saving data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log the BUDGET_CHANGE action
	user := r.Header.Get("Authorization")
	s.logTransaction(user, "BUDGET_CHANGE", req.Budget)

	// Return the new Balance (to keep consistent with other endpoints returning the int)
	// Or return JSON? The client will likely want both.
	// For now, let's return JSON here as this is a new endpoint.
	resp := GetResponse{
		Balance: s.balance,
		Budget:  s.budget,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// logTransaction writes a valid transaction to the CSV log.
func (s *Server) logTransaction(user, action string, amount int32) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("15:04:05")
	s.transLogger.Log("%s,%s,%s,%s,%d\n", dateStr, timeStr, user, action, amount)
}

// logUnauthorized writes an invalid access attempt to the separate log.
func (s *Server) logUnauthorized(user, ip string) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("15:04:05")
	s.unauthLogger.Log("%s,%s,%s,%s\n", dateStr, timeStr, user, ip)
}
