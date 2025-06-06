package expr

import (
	"database/sql/driver"
	"fmt"
)

func init() {
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
