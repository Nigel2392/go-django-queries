package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

//
//	type LookupInfo struct {
//		Driver driver.Driver
//	}
//
//	type Lookup struct {
//		Name          string
//		NormalizeArgs func(this *Lookup, value []any) []any
//		SQL           func(inf *LookupInfo, inner SQLWriter, value []any) (SQLWriter, error)
//	}
//
//	type Lookup interface {
//		Name() string                             // name of the lookup
//		Arity() int                               // number of arguments the lookup expects, or -1 for variable arguments
//		NormalizeArgs(value []any) ([]any, error) // normalize the arguments for the lookup
//		SQL(inf *LookupInfo, inner SQLWriter, value []any) (SQLWriter, error)
//	}

func init() {
	RegisterLookup("exact", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s = ?", field), value, nil
	})
	RegisterLookup("not", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s != ?", field), value, nil
	})
	RegisterLookup("bitand", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) != 1 {
			return "", value, fmt.Errorf("lookup requires exactly one value, got %d %+v", len(value), value)
		}
		value = []any{value[0], value[0]}
		return fmt.Sprintf("%s & ? = ?", field), value, nil
	})
	RegisterLookup("bitor", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) != 1 {
			return "", value, fmt.Errorf("lookup requires exactly one value, got %d %+v", len(value), value)
		}
		value = []any{value[0], value[0]}
		return fmt.Sprintf("%s | ? = ?", field), value, nil
	})
	RegisterLookup("bitxor", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) != 1 {
			return "", value, fmt.Errorf("lookup requires exactly one value, got %d %+v", len(value), value)
		}
		value = []any{value[0], value[0]}
		return fmt.Sprintf("%s ^ ? = ?", field), value, nil
	})
	RegisterLookup("iexact", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) = LOWER(?)", field), normalizeArgs("iexact", value), nil
	})
	RegisterLookup("icontains", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("icontains", value), nil
	})
	RegisterLookup("istartswith", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("istartswith", value), nil
	})
	RegisterLookup("iendswith", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", field), normalizeArgs("iendswith", value), nil
	})
	RegisterLookup("contains", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("contains", value), nil
	})
	RegisterLookup("startswith", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("startswith", value), nil
	})
	RegisterLookup("endswith", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s LIKE ?", field), normalizeArgs("endswith", value), nil
	})
	RegisterLookup("gt", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s > ?", field), value, nil
	})
	RegisterLookup("gte", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s >= ?", field), value, nil
	})
	RegisterLookup("lt", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s < ?", field), value, nil
	})
	RegisterLookup("lte", func(d driver.Driver, field string, value []any) (string, []any, error) {
		return fmt.Sprintf("%s <= ?", field), value, nil
	})
	RegisterLookup("in", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) == 0 {
			return "", value, fmt.Errorf("no values provided for IN lookup")
		}

		var inList = make([]any, 0, len(value))
		for _, v := range value {
			var rV = reflect.ValueOf(v)
			if !rV.IsValid() {
				inList = append(inList, nil)
				continue
			}

			if rV.Kind() == reflect.Slice || rV.Kind() == reflect.Array {
				for i := 0; i < rV.Len(); i++ {
					var elem = rV.Index(i).Interface()
					inList = append(inList, normalizeDefinerArg(elem))
				}
				continue
			}

			inList = append(inList, normalizeDefinerArg(v))
		}

		var placeholders = make([]string, len(inList))
		for i := range inList {
			placeholders[i] = "?"
		}

		return fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")), inList, nil
	})
	RegisterLookup("isnull", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) != 1 {
			return "", value, fmt.Errorf("ISNULL lookup requires exactly one value, got %d %+v", len(value), value)
		}
		if value[0] == nil || value[0] == false {
			return fmt.Sprintf("%s IS NOT NULL", field), []any{}, nil
		}
		return fmt.Sprintf("%s IS NULL", field), []any{}, nil
	})
	RegisterLookup("range", func(d driver.Driver, field string, value []any) (string, []any, error) {
		if len(value) != 2 {
			return "", value, fmt.Errorf("RANGE lookup requires exactly two values")
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", field), value, nil
	})

	RegisterFunc("SUM", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("SUM(%s)", col), value, nil
	})
	RegisterFunc("COUNT", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("COUNT(%s)", col), value, nil
	})
	RegisterFunc("AVG", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("AVG(%s)", col), value, nil
	})
	RegisterFunc("MAX", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("MAX(%s)", col), value, nil
	})
	RegisterFunc("MIN", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("MIN(%s)", col), value, nil
	})
	RegisterFunc("COALESCE", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("COALESCE(%s)", col), value, nil
	})
	RegisterFunc("CONCAT", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("CONCAT(%s)", col), value, nil
	})
	RegisterFunc("SUBSTR", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		if len(value) != 2 {
			return "", value, fmt.Errorf("SUBSTR lookup requires exactly two values")
		}
		return fmt.Sprintf("SUBSTR(%s, ?, ?)", col), value, nil
	})
	RegisterFunc("TRIM", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("TRIM(%s)", col), value, nil
	})
	RegisterFunc("UPPER", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("UPPER(%s)", col), value, nil
	})
	RegisterFunc("LOWER", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("LOWER(%s)", col), value, nil
	})
	RegisterFunc("LENGTH", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		col := c.(string)
		return fmt.Sprintf("LENGTH(%s)", col), value, nil
	})
	RegisterFunc("NOW", func(d driver.Driver, c any, value []any) (sql string, args []any, err error) {
		return "NOW()", value, nil
	})
	RegisterFunc("CAST", func(d driver.Driver, c any, value []any) (string, []any, error) {
		var (
			col         = c.(string)
			castTypeObj = value[0]
			castType    CastType
			ok          bool
		)

		if castType, ok = castTypeObj.(CastType); !ok {
			return "", value, fmt.Errorf("CAST type must be of type expr.CastType, got %T", castType)
		}

		var castLookupArgs []any
		if len(value) > 1 {
			castLookupArgs = value[1:]
		}
		var castTypeSql, _, err = castLookups.lookup(d, col, castType, castLookupArgs)
		if err != nil {
			return "", value, fmt.Errorf("error looking up CAST type %d: %w", castType, err)
		}

		if castTypeSql == "" {
			return "", value, fmt.Errorf(
				"CAST type %d is not implemented: %w",
				castType, ErrCastTypeNotImplemented,
			)
		}

		var sql = fmt.Sprintf("CAST(%s AS %s)", col, castTypeSql)
		return sql, []any{}, nil
	})
}

func normalizeDefinerArg(v any) any {
	if definer, ok := v.(attrs.Definer); ok {
		var fieldDefs = definer.FieldDefs()
		var pk = fieldDefs.Primary()
		return pk.GetValue()
	}
	return v
}

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

func RegisterLookup(lookup string, fn func(d driver.Driver, col string, value []any) (sql string, args []any, err error), drivers ...driver.Driver) {
	typeLookups.register(lookup, fn, drivers...)
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
