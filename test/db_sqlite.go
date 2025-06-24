//go:build (!mysql && !mysql_local && !postgres && !mariadb) || (!mysql && !mysql_local && !postgres && !mariadb && !sqlite)

package queries_test

import (
	"context"
	"os"

	"github.com/Nigel2392/go-django-queries/src/drivers"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var db_tag = "sqlite"

func init() {
	// make db globally available
	// var db, err = drivers.Open(context.Background(),"mysql", "root:my-secret-pw@tcp(127.0.0.1:3306)/queries_test?parseTime=true&multiStatements=true")
	var db, err = drivers.Open(context.Background(), "sqlite3", "file:queries_memory?mode=memory&cache=shared")
	// var db, err = drivers.Open(context.Background(),"sqlite3", "file:queries_test.db")
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
