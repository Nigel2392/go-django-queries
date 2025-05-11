package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
)

func init() {
	RegisterLookup("exact", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s = ?", field), value, nil
	})
	RegisterLookup("iexact", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) = LOWER(?)", field), normalizeArgs("iexact", value), nil
	})
	RegisterLookup("icontains", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("icontains", value), nil
	})
	RegisterLookup("istartswith", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("istartswith", value), nil
	})
	RegisterLookup("iendswith", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("iendswith", value), nil
	})
	RegisterLookup("contains", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("contains", value), nil
	})
	RegisterLookup("startswith", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("startswith", value), nil
	})
	RegisterLookup("endswith", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("endswith", value), nil
	})
	RegisterLookup("gt", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s > ?", field), value, nil
	})
	RegisterLookup("gte", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s >= ?", field), value, nil
	})
	RegisterLookup("lt", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s < ?", field), value, nil
	})
	RegisterLookup("lte", func(field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s <= ?", field), value, nil
	})
	RegisterLookup("in", func(field string, value []any) (string, []any, error) {
		if len(value) == 0 {
			return "", value, fmt.Errorf("no values provided for IN lookup")
		}
		var placeholders = make([]string, len(value))
		for i := range value {
			placeholders[i] = "?"
		}
		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")), value, nil
	})
	RegisterLookup("isnull", func(field string, value []any) (string, []any, error) {
		if len(value) != 1 {
			return "", value, fmt.Errorf("ISNULL lookup requires exactly one value, got %d %+v", len(value), value)
		}
		if value[0] == nil || value[0] == false {
			return fmt.Sprintf("%s IS NOT NULL", field), []any{}, nil
		}
		return fmt.Sprintf("%s IS NULL", field), []any{}, nil
	})
	RegisterLookup("range", func(field string, value []any) (string, []any, error) {
		if len(value) != 2 {
			return "", value, fmt.Errorf("RANGE lookup requires exactly two values")
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", field), value, nil
	})

	RegisterFunc("SUM", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("SUM(%s)", col), value, nil
	})
	RegisterFunc("COUNT", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("COUNT(%s)", col), value, nil
	})
	RegisterFunc("AVG", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("AVG(%s)", col), value, nil
	})
	RegisterFunc("MAX", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("MAX(%s)", col), value, nil
	})
	RegisterFunc("MIN", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("MIN(%s)", col), value, nil
	})
	RegisterFunc("COALESCE", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("COALESCE(%s)", col), value, nil
	})
	RegisterFunc("CONCAT", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("CONCAT(%s)", col), value, nil
	})
	RegisterFunc("SUBSTR", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		if len(value) != 2 {
			return "", value, fmt.Errorf("SUBSTR lookup requires exactly two values")
		}
		return fmt.Sprintf("SUBSTR(%s, ?, ?)", col), value, nil
	})
	RegisterFunc("TRIM", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("TRIM(%s)", col), value, nil
	})
	RegisterFunc("UPPER", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("UPPER(%s)", col), value, nil
	})
	RegisterFunc("LENGTH", func(c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("LENGTH(%s)", col), value, nil
	})
	RegisterFunc("NOW", func(c any, value []any) (sql string, args []any, err error) {
		return "NOW()", value, nil
	})
}

type _lookups[T1 any] struct {
	m              map[string]func(col T1, value []any) (sql string, args []any, err error)
	d_m            map[reflect.Type]map[string]func(col T1, value []any) (sql string, args []any, err error)
	onBeforeLookup func(col T1, lookup string, value []any) (T1, []any, error)
}

func (l *_lookups[T1]) lookupFunc(driver driver.Driver, lookup string) (func(col T1, value []any) (sql string, args []any, err error), bool) {
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

func (l *_lookups[T1]) lookup(driver driver.Driver, col T1, lookup string, value []any) (string, []any, error) {
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
	}

	var sql, args, err = fn(col, value)
	if err != nil {
		return "", nil, err
	}

	return sql, args, nil
}

func (l *_lookups[T1]) register(lookup string, fn func(col T1, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	if len(drivers) == 0 {
		l.m[lookup] = fn
		return
	}

	for _, driver := range drivers {
		var t = reflect.TypeOf(driver)
		if _, ok := l.d_m[t]; !ok {
			l.d_m[t] = make(map[string]func(col T1, value []any) (sql string, args []any, err error))
		}
		l.d_m[t][lookup] = fn
	}
}

var typeLookups = &_lookups[string]{
	m:   make(map[string]func(col string, value []any) (sql string, args []any, err error)),
	d_m: make(map[reflect.Type]map[string]func(col string, value []any) (sql string, args []any, err error)),
}

func RegisterLookup(lookup string, fn func(col string, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	typeLookups.register(lookup, fn, drivers...)
}

var funcLookups = &_lookups[any]{
	m:   make(map[string]func(col any, value []any) (sql string, args []any, err error)),
	d_m: make(map[reflect.Type]map[string]func(col any, value []any) (sql string, args []any, err error)),
	onBeforeLookup: func(col any, lookup string, value []any) (any, []any, error) {
		switch c := col.(type) {
		case string:
			return c, value, nil
		case Expression:
			var sb strings.Builder
			var args = c.SQL(&sb)
			return sb.String(), args, nil
		default:
			return "", nil, fmt.Errorf("unsupported column type %T", col)
		}
	},
}

func RegisterFunc(funcName string, fn func(col any, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	funcLookups.register(funcName, fn, drivers...)
}
