package drivers

import (
	"database/sql"
	"database/sql/driver"

	"github.com/go-sql-driver/mysql"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/mattn/go-sqlite3"

	"reflect"
)

/*
Package drivers provides a shortcut to access the registered drivers
and their capabilities. It allows you to check if a driver supports
returning values, and to get the name of the driver for a given SQL database.
*/

type SupportsReturningType string

const (
	SupportsReturningNone         SupportsReturningType = ""
	SupportsReturningLastInsertId SupportsReturningType = "last_insert_id"
	SupportsReturningColumns      SupportsReturningType = "columns"
)

var Drivers = make(map[reflect.Type]driverData)

type (
	DriverPostgres = pg_stdlib.Driver
	DriverMySQL    = mysql.MySQLDriver
	DriverSQLite   = sqlite3.SQLiteDriver
)

type driverData struct {
	Name              string
	SupportsReturning SupportsReturningType
}

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
func RegisterDriver(driver driver.Driver, database string, supportsReturning ...SupportsReturningType) {
	var s SupportsReturningType
	if len(supportsReturning) > 0 {
		s = supportsReturning[0]
	}
	Drivers[reflect.TypeOf(driver)] = driverData{
		Name:              database,
		SupportsReturning: s,
	}
}

// SupportsReturning returns the type of returning supported by the database.
// It can be one of the following:
//
// - SupportsReturningNone: no returning supported
// - SupportsReturningLastInsertId: last insert id supported
// - SupportsReturningColumns: returning columns supported
func SupportsReturning(db *sql.DB) SupportsReturningType {
	var driver = reflect.TypeOf(db.Driver())
	if driver == nil {
		return SupportsReturningNone
	}
	if data, ok := Drivers[driver]; ok {
		return data.SupportsReturning
	}
	return SupportsReturningNone
}
