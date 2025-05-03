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
}

type _lookups struct {
	m   map[string]func(col string, value []any) (sql string, args []any, err error)
	d_m map[reflect.Type]map[string]func(col string, value []any) (sql string, args []any, err error)
}

var lookups = &_lookups{
	m:   make(map[string]func(col string, value []any) (sql string, args []any, err error)),
	d_m: make(map[reflect.Type]map[string]func(col string, value []any) (sql string, args []any, err error)),
}

func newLookup(driver driver.Driver, field string, lookup string, value []any) (string, []any, error) {
	var m, ok = lookups.d_m[reflect.TypeOf(driver)]
	if !ok {
		m = lookups.m
	}

	fn, ok := m[lookup]
	if !ok {
		fn, ok = lookups.m[lookup]
		if !ok {
			return "", nil, query_errors.ErrUnsupportedLookup
		}
	}

	var sql, args, err = fn(field, value)
	if err != nil {
		return "", nil, err
	}
	return sql, args, nil
}

func RegisterLookup(lookup string, fn func(col string, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	if len(drivers) == 0 {
		lookups.m[lookup] = fn
		return
	}

	for _, driver := range drivers {
		var t = reflect.TypeOf(driver)
		if _, ok := lookups.d_m[t]; !ok {
			lookups.d_m[t] = make(map[string]func(col string, value []any) (sql string, args []any, err error))
		}
		lookups.d_m[t][lookup] = fn
	}
}
