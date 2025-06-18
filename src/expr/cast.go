package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/drivers"
	"github.com/Nigel2392/go-django/src/core/errs"
)

type CastType uint

const (
	ErrCastTypeNotImplemented   errs.Error = "cast type is not implemented"
	ErrCastTypeNoColumnProvided errs.Error = "cast type requires a column to be provided"

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

type castExpr struct {
	funcEntry *CastFuncEntry
	typ       CastType
	col       NamedExpression
	args      []any
	used      bool
}

func (c *castExpr) FieldName() string {
	return c.col.FieldName()
}

func (c *castExpr) Clone() Expression {
	return &castExpr{
		typ:       c.typ,
		funcEntry: c.funcEntry,
		col:       c.col.Clone().(NamedExpression),
		args:      slices.Clone(c.args),
		used:      c.used,
	}
}

func (c *castExpr) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || c.used {
		return c
	}
	var nE = c.Clone().(*castExpr)
	nE.used = true
	nE.col = nE.col.Resolve(inf).(NamedExpression)

	var funcEntry, ok = casts.global[c.typ]
	if !ok {
		var byType, ok = casts.byType[reflect.TypeOf(inf.Driver)]
		if !ok {
			panic(fmt.Errorf("%w: %d", ErrCastTypeNotImplemented, c.typ))
		}
		funcEntry, ok = byType[c.typ]
		if !ok {
			panic(fmt.Errorf("%w: %d", ErrCastTypeNotImplemented, c.typ))
		}
	}

	if funcEntry.Arity != len(nE.args) {
		panic(fmt.Errorf(
			"cast type %d requires %d arguments, got %d",
			c.typ, funcEntry.Arity, len(nE.args),
		))
	}

	nE.funcEntry = &funcEntry

	return nE
}

func (c *castExpr) SQL(sb *strings.Builder) []any {
	var sprintParams = make([]any, 0, c.funcEntry.Arity+1)
	var colBuilder strings.Builder
	var args = c.col.SQL(&colBuilder)
	sprintParams = append(sprintParams, colBuilder.String())
	sprintParams = append(sprintParams, c.args...)
	sb.WriteString(fmt.Sprintf(c.funcEntry.SQL, sprintParams...))
	return args
}

func Cast(typ CastType, col any, value ...any) NamedExpression {
	var exprs = expressionFromInterface[NamedExpression](col, false)
	if len(exprs) == 0 {
		panic(ErrCastTypeNoColumnProvided)
	}

	return &castExpr{
		typ:  typ,
		col:  exprs[0],
		args: value,
	}
}

func registerCastTypeFunc(d driver.Driver, arity int, castType CastType, sqlText string) {
	if d == nil {
		RegisterCastType(castType, CastFuncEntry{Arity: arity, SQL: sqlText})
		return
	}
	RegisterCastType(castType, CastFuncEntry{Arity: arity, SQL: sqlText}, d)
}

func newCastFunc(typ CastType) func(col any, value ...any) NamedExpression {
	return func(col any, value ...any) NamedExpression {
		return Cast(typ, col, value...)
	}
}

type castRegistry struct {
	global map[CastType]CastFuncEntry
	byType map[reflect.Type]map[CastType]CastFuncEntry
}

type CastFuncEntry struct {
	Arity int
	SQL   string
}

var casts = &castRegistry{
	global: make(map[CastType]CastFuncEntry),
	byType: make(map[reflect.Type]map[CastType]CastFuncEntry),
}

func RegisterCastType(castType CastType, entry CastFuncEntry, drivers ...driver.Driver) {
	if len(drivers) == 0 {
		casts.global[castType] = entry
		return
	}

	for _, drv := range drivers {
		var t = reflect.TypeOf(drv)
		if _, ok := casts.byType[t]; !ok {
			casts.byType[t] = make(map[CastType]CastFuncEntry)
		}
		casts.byType[t][castType] = entry
	}
}
