//go:build !sqlite && !postgres && !mysql && !mysql_local && mariadb

package queries_test

import (
	"database/sql"
	"os"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var db_tag = "mariadb"

func init() {
	// make db globally available
	var db, err = sql.Open("mariadb", "root:my-secret-pw@tcp(127.0.0.1:3307)/queries_test?parseTime=true&multiStatements=true&interpolateParams=true")
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

	logger.Debug("Using MariaDB database for queries tests")
}
