package main

import (
	"database/sql"
	"flag"
	"fmt"
	"gitlab.com/digineat/go-broker-test/cmd"
	"log"
	"time"

	"github.com/go-playground/validator/v10"
)

func main() {
	// Command line flags
	dbPath := flag.String("db", "data.db", "path to SQLite database")
	pollInterval := flag.Duration("poll", 100*time.Millisecond, "polling interval")
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

	log.Printf("Worker started with polling interval: %v", *pollInterval)

	validate := validator.New()

	// Main worker loop
	for {
		err = processTrades(db, validate)
		if err != nil {
			log.Printf("Failed to process trades: %v", err)
		}

		// Sleep for the specified interval
		time.Sleep(*pollInterval)
	}
}

func processTrades(db *sql.DB, validate *validator.Validate) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
        SELECT id, account, symbol, volume, open, close, side 
        FROM trades_q 
        WHERE processed = FALSE`)
	if err != nil {
		return fmt.Errorf("failed to query unprocessed trades: %w", err)
	}

	var trades []cmd.Trade
	for rows.Next() {
		var t cmd.Trade
		if err := rows.Scan(&t.ID, &t.Account, &t.Symbol, &t.Volume, &t.Open, &t.Close, &t.Side); err != nil {
			log.Printf("Error scanning trade row: %v", err)
			continue
		}
		trades = append(trades, t)
	}
	rows.Close()

	for _, t := range trades {
		if err := validate.Struct(t); err != nil {
			log.Printf("Invalid trade ID=%d: %v", t.ID, err)
			continue
		}

		lot := 100000.0
		profit := (t.Close - t.Open) * t.Volume * lot
		if t.Side == "sell" {
			profit = -profit
		}

		_, err = tx.Exec(`
            INSERT INTO account_stats (account, trades, profit)
            VALUES (?, 1, ?)
            ON CONFLICT(account) DO UPDATE SET
                trades = trades + 1,
                profit = profit + ?
            WHERE account = ?`,
			t.Account, profit, profit, t.Account)

		if err != nil {
			return fmt.Errorf("failed to update stats for account %s: %w", t.Account, err)
		}

		_, err = tx.Exec(`UPDATE trades_q SET processed = TRUE WHERE id = ?`, t.ID)
		if err != nil {
			return fmt.Errorf("failed to mark trade as processed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
