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

	CastTypeUnknown CastType = iota
	CastTypeString
	CastTypeText
	CastTypeInt
	CastTypeFloat
	CastTypeBool
	CastTypeDate
	CastTypeTime
	CastTypeBytes
	CastTypeDecimal
	CastTypeJSON
	CastTypeUUID
	CastTypeNull
	CastTypeArray
)

func init() {
	registerCastTypeFunc(&mysql.MySQLDriver{}, 1, CastTypeString, "CAST(%s AS CHAR(%d))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeInt, "CAST(%s AS SIGNED)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 2, CastTypeFloat, "CAST(%s AS DECIMAL(%d,%d))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeBool, "CAST(%s AS UNSIGNED)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeBytes, "CAST(%s AS BINARY)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeDecimal, "CAST(%s AS DECIMAL(10,2))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeJSON, "CAST(%s AS JSON)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeUUID, "CAST(%s AS CHAR(36))")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&mysql.MySQLDriver{}, 0, CastTypeArray, "CAST(%s AS JSON)")

	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 1, CastTypeString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 2, CastTypeFloat, "CAST(%s AS REAL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeBool, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeDate, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeTime, "CAST(%s AS TIMESTAMP)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeBytes, "CAST(%s AS BLOB)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeDecimal, "CAST(%s AS REAL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeJSON, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeUUID, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&sqlite3.SQLiteDriver{}, 0, CastTypeArray, "CAST(%s AS TEXT)")

	registerCastTypeFunc(&pg_stdlib.Driver{}, 1, CastTypeString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 2, CastTypeFloat, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeBool, "CAST(%s AS BOOLEAN)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeBytes, "CAST(%s AS BYTEA)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 2, CastTypeDecimal, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeJSON, "CAST(%s AS JSONB)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeUUID, "CAST(%s AS UUID)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&pg_stdlib.Driver{}, 0, CastTypeArray, "CAST(%s AS JSONB)")
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
		RegisterCastType(castType, castTypeFunc(sqlText, arity))
		return
	}
	RegisterCastType(castType, castTypeFunc(sqlText, arity), d)
}

func Cast(typ CastType, col any, value ...any) NamedExpression {
	return newFunc(castLookups, typ, value, col)
}

func newCastFunc(typ CastType) func(col any, value ...any) NamedExpression {
	return func(col any, value ...any) NamedExpression {
		return Cast(typ, col, value...)
	}
}

var (
	CastString  = newCastFunc(CastTypeString)
	CastText    = newCastFunc(CastTypeText)
	CastInt     = newCastFunc(CastTypeInt)
	CastFloat   = newCastFunc(CastTypeFloat)
	CastBool    = newCastFunc(CastTypeBool)
	CastDate    = newCastFunc(CastTypeDate)
	CastTime    = newCastFunc(CastTypeTime)
	CastBytes   = newCastFunc(CastTypeBytes)
	CastDecimal = newCastFunc(CastTypeDecimal)
	CastJSON    = newCastFunc(CastTypeJSON)
	CastUUID    = newCastFunc(CastTypeUUID)
	CastNull    = newCastFunc(CastTypeNull)
	CastArray   = newCastFunc(CastTypeArray)
)

var castLookups = &_lookups[any, CastType]{
	m:              make(map[CastType]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	d_m:            make(map[reflect.Type]map[CastType]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	onBeforeLookup: handleExprLookups[CastType],
}

func RegisterCastType(castType CastType, fn func(d driver.Driver, col any, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	castLookups.register(castType, fn, drivers...)
}
