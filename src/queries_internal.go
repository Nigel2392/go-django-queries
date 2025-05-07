package queries

import (
	"database/sql"
	"database/sql/driver"

	"github.com/Nigel2392/go-django-queries/internal"

	_ "unsafe"
)

type SupportsReturning = internal.SupportsReturning

const (
	SupportsReturningNone         SupportsReturning = internal.SupportsReturningNone
	SupportsReturningLastInsertId SupportsReturning = internal.SupportsReturningLastInsertId
	SupportsReturningColumns      SupportsReturning = internal.SupportsReturningColumns
)

//go:linkname DBSupportsReturning github.com/Nigel2392/go-django-queries/internal.DBSupportsReturning
func DBSupportsReturning(db *sql.DB) SupportsReturning

// RegisterDriver registers a driver with the given database name.
//
// This is used to determine the database type when using sqlx.
//
// If your driver is not one of:
// - github.com/go-sql-driver/mysql.MySQLDriver
// - github.com/mattn/go-sqlite3.SQLiteDriver
// - github.com/jackc/pgx/v5/stdlib.Driver
//
// Then it explicitly needs to be registered here.
//
//go:linkname RegisterDriver github.com/Nigel2392/go-django-queries/internal.RegisterDriver
func RegisterDriver(driver driver.Driver, database string, supportsReturning ...SupportsReturning)
