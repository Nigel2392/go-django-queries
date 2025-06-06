package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
)

type _lookups[T1 any, T2 comparable] struct {
	m              map[T2]func(d driver.Driver, col T1, value []any) (sql string, args []any, err error)
	d_m            map[reflect.Type]map[T2]func(d driver.Driver, col T1, value []any) (sql string, args []any, err error)
	onBeforeLookup func(col T1, lookup T2, value []any) (T1, []any, error)
}

func (l *_lookups[T1, T2]) lookupFunc(driver driver.Driver, lookup T2) (func(d driver.Driver, col T1, value []any) (sql string, args []any, err error), bool) {
	var m, ok = l.d_m[reflect.TypeOf(driver)]
	if !ok {
		m = l.m
	}

	fn, ok := m[lookup]
	if !ok {
		fn, ok = l.m[lookup]
	}
	return fn, ok
}

func (l *_lookups[T1, T2]) lookup(driver driver.Driver, col T1, lookup T2, value []any) (string, []any, error) {
	var m, ok = l.d_m[reflect.TypeOf(driver)]
	if !ok {
		m = l.m
	}

	fn, ok := m[lookup]
	if !ok {
		fn, ok = l.m[lookup]
		if !ok {
			return "", nil, query_errors.ErrUnsupportedLookup
		}
	}

	if l.onBeforeLookup != nil {
		var err error
		col, value, err = l.onBeforeLookup(col, lookup, value)
		if err != nil {
			return "", nil, fmt.Errorf("error in onBeforeLookup for lookup \"%v\": %w", lookup, err)
		}
	}

	var sql, args, err = fn(driver, col, value)
	if err != nil {
		return "", nil, fmt.Errorf("error in lookup \"%v\": %w", lookup, err)
	}

	return sql, args, nil
}

func (l *_lookups[T1, T2]) register(lookup T2, fn func(d driver.Driver, col T1, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	if len(drivers) == 0 {
		l.m[lookup] = fn
		return
	}

	for _, drv := range drivers {
		var t = reflect.TypeOf(drv)
		if _, ok := l.d_m[t]; !ok {
			l.d_m[t] = make(map[T2]func(d driver.Driver, col T1, value []any) (sql string, args []any, err error))
		}
		l.d_m[t][lookup] = fn
	}
}

var typeLookups = &_lookups[string, string]{
	m:   make(map[string]func(d driver.Driver, col string, value []any) (sql string, args []any, err error)),
	d_m: make(map[reflect.Type]map[string]func(d driver.Driver, col string, value []any) (sql string, args []any, err error)),
}

func handleExprLookups[T1 comparable](col any, lookup T1, value []any) (any, []any, error) {
	switch c := col.(type) {
	case string:
		return c, value, nil
	case Expression:
		var sb strings.Builder
		var args = c.SQL(&sb)
		return sb.String(), append(args, value...), nil
	default:
		return "", nil, fmt.Errorf("unsupported column type %T", col)
	}
}

var funcLookups = &_lookups[any, string]{
	m:              make(map[string]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	d_m:            make(map[reflect.Type]map[string]func(d driver.Driver, col any, value []any) (sql string, args []any, err error)),
	onBeforeLookup: handleExprLookups[string],
}

func RegisterFunc(funcName string, fn func(d driver.Driver, col any, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	funcLookups.register(funcName, fn, drivers...)
}
