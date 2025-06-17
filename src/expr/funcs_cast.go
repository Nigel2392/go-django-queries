package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django-queries/src/drivers"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/errs"
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
	registerCastTypeFunc(&drivers.DriverMySQL{}, 1, CastTypeString, "CAST(%s AS CHAR(%d))")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeInt, "CAST(%s AS SIGNED)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 2, CastTypeFloat, "CAST(%s AS DECIMAL(%d,%d))")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeBool, "CAST(%s AS UNSIGNED)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeBytes, "CAST(%s AS BINARY)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 2, CastTypeDecimal, "CAST(%s AS DECIMAL(%d,%d))")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeJSON, "CAST(%s AS JSON)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeUUID, "CAST(%s AS CHAR(36))")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&drivers.DriverMySQL{}, 0, CastTypeArray, "CAST(%s AS JSON)")

	registerCastTypeFunc(&drivers.DriverSQLite{}, 1, CastTypeString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 2, CastTypeFloat, "CAST(%s AS REAL)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeBool, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeDate, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeTime, "CAST(%s AS TIMESTAMP)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeBytes, "CAST(%s AS BLOB)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 2, CastTypeDecimal, "CAST(%s AS REAL)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeJSON, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeUUID, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&drivers.DriverSQLite{}, 0, CastTypeArray, "CAST(%s AS TEXT)")

	registerCastTypeFunc(&drivers.DriverPostgres{}, 1, CastTypeString, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeText, "CAST(%s AS TEXT)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeInt, "CAST(%s AS INTEGER)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 2, CastTypeFloat, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeBool, "CAST(%s AS BOOLEAN)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeDate, "CAST(%s AS DATE)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeTime, "CAST(%s AS TIME)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeBytes, "CAST(%s AS BYTEA)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 2, CastTypeDecimal, "CAST(%s AS NUMERIC(%d,%d))")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeJSON, "CAST(%s AS JSONB)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeUUID, "CAST(%s AS UUID)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeNull, "CAST(%s AS NULL)")
	registerCastTypeFunc(&drivers.DriverPostgres{}, 0, CastTypeArray, "CAST(%s AS JSONB)")
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
