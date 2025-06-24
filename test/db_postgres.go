//go:build !mysql && !mysql_local && !mariadb && !sqlite && postgres

package queries_test

import (
	"context"
	"fmt"
	"os"

	"github.com/Nigel2392/go-django-queries/src/drivers"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var db_tag = "postgres"

func init() {
	// make db globally available
	var db, err = drivers.Open(context.Background(), "postgres", "postgres://root:my-secret-pw@localhost:5432/queries_test?sslmode=disable&TimeZone=UTC")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
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
