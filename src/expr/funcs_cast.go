package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/errs"
	"github.com/go-sql-driver/mysql"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"

	"github.com/mattn/go-sqlite3"
)

type CastType uint

const (
	ErrCastTypeNotImplemented errs.Error = "cast type is not implemented"

	CastUnknown CastType = iota
	CastString
	CastText
	CastInt
	CastFloat
	CastBool
	CastDate
	CastTime
	CastBytes
	CastDecimal
	CastJSON
	CastUUID
	CastNull
	CastArray
)

func init() {
	registerCastTypeFunc(&mysql.MySQLDriver{}, 1, CastString, "CAST(%s AS CHAR(%d))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastInt, "CAST(%s AS SIGNED)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 2, CastFloat, "CAST(%s AS DECIMAL(%d,%d))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastBool, "CAST(%s AS UNSIGNED)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastBytes, "CAST(%s AS BINARY)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastDecimal, "CAST(%s AS DECIMAL(10,2))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastJSON, "CAST(%s AS JSON)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastUUID, "CAST(%s AS CHAR(36))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastArray, "CAST(%s AS JSON)")

	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 1, CastString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 2, CastFloat, "CAST(%s AS REAL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastBool, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastDate, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTime, "CAST(%s AS TIMESTAMP)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastBytes, "CAST(%s AS BLOB)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastDecimal, "CAST(%s AS REAL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastJSON, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastUUID, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastArray, "CAST(%s AS TEXT)")

	registerCastTypeFunc(&pg_stdlib.Driver{}, 1, CastString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 2, CastFloat, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastBool, "CAST(%s AS BOOLEAN)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastBytes, "CAST(%s AS BYTEA)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 2, CastDecimal, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastJSON, "CAST(%s AS JSONB)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastUUID, "CAST(%s AS UUID)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastArray, "CAST(%s AS JSONB)")
}

func castTypeFunc(sqlText string, arity int) func(d driver.Driver, col any, value []any) (sql string, args []any, err error) {
	return func(d driver.Driver, col any, value []any) (sql string, args []any, err error) {
		if len(value) != arity {
			return "", nil, query_errors.ErrFieldNotFound
		}

		var sprintParams = make([]any, 0, arity+1)
		sprintParams = append(sprintParams, col)
		sprintParams = append(sprintParams, value...)
		return fmt.Sprintf(sqlText, sprintParams...), []any{}, nil
	}
}

func registerCastTypeFunc(d driver.Driver, arity int, castType CastType, sqlText string) {
	if d == nil {
		castLookups.register(castType, castTypeFunc(sqlText, arity))
		return
	}
	castLookups.register(castType, castTypeFunc(sqlText, arity), d)
}

func Cast(typ CastType, col any, value ...any) Expression {
	return newFunc(castLookups, typ, value, col)
}

var castLookups = &_lookups[any, CastType]{
	m:              make(map[CastType]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	d_m:            make(map[reflect.Type]map[CastType]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	onBeforeLookup: handleExprLookups[CastType],
}

func RegisterCastType(castType CastType, fn func(d driver.Driver, col any, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	castLookups.register(castType, fn, drivers...)
}
