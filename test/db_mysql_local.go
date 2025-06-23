//go:build !sqlite && !postgres && !mariadb && !mysql && mysql_local

package queries_test

import (
	"os"

	"github.com/Nigel2392/go-django-queries/src/quest"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var db_tag = "mysql_local"

func init() {
	// make db globally available
	var questDb, err = quest.MySQLDatabase(quest.DatabaseConfig{
		DBName: "queries_test",
	})
	if err != nil {
		panic(err)
	}

	go func() {
		if err := questDb.Start(); err != nil {
			panic(err)
		}
	}()

	db, err := questDb.DB()
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

	logger.Debug("Using MySQL (LOCAL) database for queries tests")
}
