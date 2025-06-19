//go:build !mysql && !mysql_local && !mariadb && !sqlite

package queries_test

import (
	"database/sql"
	"os"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

func init() {
	// make db globally available
	var db, err = sql.Open("pgx", "postgres://root:my-secret-pw@localhost:5432/queries_test?sslmode=disable&TimeZone=UTC")
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

	logger.Debug("Using Postgres database for queries tests")
}
