package main

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-playground/validator/v10"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func TestProcessTrades_MockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	validate := validator.New()

	_ = validate.RegisterValidation("matches", func(fl validator.FieldLevel) bool {
		regex := regexp.MustCompile(fl.Param())
		return regex.MatchString(fl.Field().String())
	})
	defer db.Close()

	mock.ExpectBegin()

	rows := sqlmock.NewRows([]string{"id", "account", "symbol", "volume", "open", "close", "side"}).
		AddRow(1, "acc123", "EURUSD", 1.0, 1.1000, 1.1050, "buy")

	mock.ExpectQuery(`SELECT id, account, symbol, volume, open, close, side FROM trades_q WHERE processed = FALSE`).
		WillReturnRows(rows)

	mock.ExpectExec("INSERT INTO account_stats").
		WithArgs("acc123", sqlmock.AnyArg(), sqlmock.AnyArg(), "acc123").
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec("UPDATE trades_q SET processed = TRUE WHERE id = \\?").
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err = processTrades(db, validate)
	assert.NoError(t, err)

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}
