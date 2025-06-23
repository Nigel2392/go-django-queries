//go:build !sqlite && !postgres && !mariadb && !mysql_local

package queries_test

import (
	"database/sql"
	"os"
	"time"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

func init() {
	// make db globally available
	var db, err = sql.Open("mysql", "root:my-secret-pw@tcp(127.0.0.1:3306)/queries_test?parseTime=true&multiStatements=true&interpolateParams=true")
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

	logger.Debug("Using MySQL database for queries tests")

	for {
		//  Wait for the database to be ready
		if err := db.Ping(); err == nil {
			break
		}
		logger.Debug("Waiting for MySQL database to be ready...")
		time.Sleep(5 * time.Second)
	}

	db.Exec("SET SESSION sql_mode = REPLACE(@@sql_mode, 'ONLY_FULL_GROUP_BY', '')")
}
