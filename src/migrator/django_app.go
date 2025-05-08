package migrator

import (
	"database/sql"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/apps"
	"github.com/Nigel2392/go-django/src/core/command"
)

type MigratorAppConfig struct {
	*apps.DBRequiredAppConfig
	MigrationDir string
	engine       *MigrationEngine
}

var app = &MigratorAppConfig{
	DBRequiredAppConfig: apps.NewDBAppConfig("migrator"),
}

func NewAppConfig() *MigratorAppConfig {

	var migrationDir, ok = django.ConfigGetOK(
		django.Global.Settings,
		APPVAR_MIGRATION_DIR,
		"migrations",
	)

	if ok {
		app.MigrationDir = migrationDir
	}

	app.Init = func(settings django.Settings, db *sql.DB) error {

		var schemaEditor, err = GetSchemaEditor(db.Driver())
		if err != nil {
			return err
		}

		app.engine = NewMigrationEngine(
			app.MigrationDir,
			schemaEditor,
		)
		return nil
	}

	app.Cmd = []command.Command{
		commandMakeMigrations,
		commandMigrate,
	}

	return app
}
