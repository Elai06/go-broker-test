package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"gitlab.com/digineat/go-broker-test/cmd"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func setupMockServer(mockDB *sql.DB) *httptest.Server {
	validate := validator.New()
	_ = validate.RegisterValidation("matches", func(fl validator.FieldLevel) bool {
		regex := regexp.MustCompile(fl.Param())
		return regex.MatchString(fl.Field().String())
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /trades", func(w http.ResponseWriter, r *http.Request) {
		var trade cmd.Trade
		if err := json.NewDecoder(r.Body).Decode(&trade); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := validate.Struct(trade); err != nil {
			http.Error(w, "Validation failed: "+err.Error(), http.StatusBadRequest)
			return
		}

		result, err := mockDB.Exec(`
        INSERT INTO trades_q (account, symbol, volume, open, close, side)
        VALUES (?, ?, ?, ?, ?, ?)`,
			trade.Account, trade.Symbol, trade.Volume, trade.Open, trade.Close, trade.Side)

		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		id, _ := result.LastInsertId()
		fmt.Printf("Saved trade ID=%d for account %s\n", id, trade.Account)

		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /stats/{acc}", func(w http.ResponseWriter, r *http.Request) {
		account := r.PathValue("acc")

		var acc string
		var trades int
		var profit float64

		err := mockDB.QueryRow("SELECT account, trades, profit FROM account_stats WHERE account = ?", account).
			Scan(&acc, &trades, &profit)

		if err == sql.ErrNoRows {
			http.Error(w, "Account not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		response := struct {
			Account string  `json:"account"`
			Trades  int     `json:"trades"`
			Profit  float64 `json:"profit"`
		}{
			Account: acc,
			Trades:  trades,
			Profit:  profit,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := mockDB.Ping(); err != nil {
			http.Error(w, "Database unreachable", http.StatusInternalServerError)
			return
		}

		w.Write([]byte("OK\n"))
	})

	return httptest.NewServer(mux)
}

func TestPostTrades_ValidTrade(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("INSERT INTO trades_q").
		WithArgs("test123", "EURUSD", 1.0, 1.1000, 1.1050, "buy").
		WillReturnResult(sqlmock.NewResult(1, 1))

	ts := setupMockServer(db)
	defer ts.Close()

	trade := cmd.Trade{
		Account: "test123",
		Symbol:  "EURUSD",
		Volume:  1.0,
		Open:    1.1000,
		Close:   1.1050,
		Side:    "buy",
	}
	body, _ := json.Marshal(trade)

	resp, err := http.Post(ts.URL+"/trades", "application/json", bytes.NewBuffer(body))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPostTrades_InvalidSymbol(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer db.Close()

	ts := setupMockServer(db)
	defer ts.Close()

	trade := cmd.Trade{
		Account: "test123",
		Symbol:  "eurusd", // lowercase — invalid
		Volume:  1.0,
		Open:    1.1000,
		Close:   1.1050,
		Side:    "buy",
	}
	body, _ := json.Marshal(trade)

	resp, err := http.Post(ts.URL+"/trades", "application/json", bytes.NewBuffer(body))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetStats_AccountExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"account", "trades", "profit"}).
		AddRow("test123", 5, 1000.0)

	mock.ExpectQuery("SELECT account, trades, profit FROM account_stats WHERE account = \\?").
		WithArgs("test123").
		WillReturnRows(rows)

	ts := setupMockServer(db)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/stats/test123")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		Account string  `json:"account"`
		Trades  int     `json:"trades"`
		Profit  float64 `json:"profit"`
	}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	assert.Equal(t, "test123", out.Account)
	assert.Equal(t, 5, out.Trades)
	assert.Equal(t, 1000.0, out.Profit)
}

func TestHealthz(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer db.Close()

	mock.ExpectPing()

	ts := setupMockServer(db)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "OK\n", body.String())
}
