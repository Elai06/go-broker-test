package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/go-playground/validator/v10"
	"gitlab.com/digineat/go-broker-test/cmd"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "data.db", "path to SQLite database")
	listenAddr := flag.String("listen", "8080", "HTTP server listen address")
	flag.Parse()

	// Initialize database connection
	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Initialize HTTP server
	mux := http.NewServeMux()
	validate := validator.New()

	// POST /trades endpoint
	mux.HandleFunc("POST /trades", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Write code here
		var trade cmd.Trade
		if err := readJSON(w, r, &trade); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := validate.Struct(trade); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		result, err := db.Exec(`
        INSERT INTO trades_q (account, symbol, volume, open, close, side)
        VALUES (?, ?, ?, ?, ?, ?)`,
			trade.Account, trade.Symbol, trade.Volume, trade.Open, trade.Close, trade.Side,
		)
		if err != nil {
			log.Printf("Failed to save trade: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		id, _ := result.LastInsertId()
		log.Printf("Saved trade ID=%d for account %s", id, trade.Account)

		w.WriteHeader(http.StatusOK)
	})

	// GET /stats/{acc} endpoint
	mux.HandleFunc("GET /stats/{acc}", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Write code here
		account := r.PathValue("acc")

		var acc string
		var trades int
		var profit float64

		err := db.QueryRow("SELECT account, trades, profit FROM account_stats WHERE account = ?", account).
			Scan(&acc, &trades, &profit)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Failed to fetch stats: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := fmt.Sprintf(`{"account":"%s","trades":%d,"profit":%.2f}`+"\n", acc, trades, profit)
		w.Header().Set("Content-Type", "application/json")
		status, err := w.Write([]byte(response))
		if err != nil {
			log.Printf("Failed to write response: %v", err)
		}

		if status != http.StatusOK {
			log.Printf("Failed to write response: %d", status)
			return
		}

		err = json.NewEncoder(w).Encode(&response)

		w.WriteHeader(http.StatusOK)
	})

	// GET /healthz endpoint
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Write code here
		// 1. Check database connection
		if err := db.Ping(); err != nil {
			log.Printf("Database health check failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 2. Return health status
		status, err := w.Write([]byte("OK\n"))
		if err != nil {
			log.Printf("Failed to write response: %v", err)
		}
		if status != http.StatusOK {
			log.Printf("Failed to write response: %d", status)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Start server
	serverAddr := fmt.Sprintf(":%s", *listenAddr)
	log.Printf("Starting server on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}
