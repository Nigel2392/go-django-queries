package migrator

import (
	"context"
	"database/sql"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type CanSQL[T any] interface {
	SQL(T) (string, []any)
}

type SchemaEditor interface {
	Setup() error
	StoreMigration(appName string, modelName string, migrationName string) error
	HasMigration(appName string, modelName string, migrationName string) (bool, error)
	RemoveMigration(appName string, modelName string, migrationName string) error

	Execute(ctx context.Context, query string, args ...any) (sql.Result, error)

	CreateTable(table Table) error
	DropTable(table Table) error
	RenameTable(table Table, newName string) error

	AddIndex(table Table, index Index) error
	DropIndex(table Table, index Index) error
	RenameIndex(table Table, oldName string, newName string) error

	//	AlterUniqueTogether(table Table, unique bool) error
	//	AlterIndexTogether(table Table, unique bool) error

	AddField(table Table, col Column) error
	AlterField(table Table, old Column, newCol Column) error
	RemoveField(table Table, col Column) error
}

type Table interface {
	TableName() string
	Model() attrs.Definer
	Columns() []*Column
	Comment() string
	Indexes() []Index
}
