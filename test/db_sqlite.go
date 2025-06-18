//go:build (!mysql && !postgres) || (!mysql && !postgres && !sqlite)

package queries_test

import (
	"database/sql"
	"os"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

func init() {
	// make db globally available
	// var db, err = sql.Open("mysql", "root:my-secret-pw@tcp(127.0.0.1:3306)/queries_test?parseTime=true&multiStatements=true")
	var db, err = sql.Open("sqlite3", "file:queries_memory?mode=memory&cache=shared")
	// var db, err = sql.Open("sqlite3", "file:queries_test.db")
	if err != nil {
		panic(err)
	}
	var settings = map[string]interface{}{
		django.APPVAR_DATABASE: db,
	}

	logger.Setup(&logger.Logger{
		Level:       logger.DBG,
		WrapPrefix:  logger.ColoredLogWrapper,
		OutputDebug: os.Stdout,
		OutputInfo:  os.Stdout,
		OutputWarn:  os.Stdout,
		OutputError: os.Stdout,
	})

	django.App(django.Configure(settings))

	logger.Debug("Using SQLite database for queries tests")
}
