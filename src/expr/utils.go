package expr

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var (
	exprFieldRegex = regexp.MustCompile(`!\[([^\]]*)\]`)
	exprValueRegex = regexp.MustCompile(`\?\[([^\]][0-9]*)\]`)
)

// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	 # sets the field name to the first field found in the statement, I.E. ![Field1]:
//
//		stmt, fields, values := ParseExprStatement("![Field1] = ![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
func ParseExprStatement(statement string, value []any) (newStatement string, fields []string, values []any) {
	fields = make([]string, 0)
	for _, m := range exprFieldRegex.FindAllStringSubmatch(statement, -1) {
		if len(m) > 1 {
			fields = append(fields, m[1]) // m[0] is full match, m[1] is capture group
		}
	}

	var valuesIndices = exprValueRegex.FindAllStringSubmatch(statement, -1)
	values = make([]any, len(valuesIndices))
	for i, m := range valuesIndices {
		var idx, err = strconv.Atoi(m[1])
		if err != nil {
			panic(fmt.Errorf("invalid index %q in statement %q: %w", m[1], statement, err))
		}

		idx -= 1 // convert to 0-based index
		if idx < 0 || idx >= len(value) {
			panic(fmt.Errorf("index %d out of range in statement %q, index is 1-based and must be between 1 and %d", idx+1, statement, len(value)))
		}

		values[i] = value[idx]
	}

	if len(valuesIndices) == 0 && len(value) > 0 {
		values = make([]any, len(value))
		copy(values, value)
	}

	statement = strings.Replace(statement, "%", "%%", -1)
	statement = exprFieldRegex.ReplaceAllString(statement, "%s")
	statement = exprValueRegex.ReplaceAllString(statement, "?")
	return statement, fields, values
}

func expressionFromInterface[T Expression](exprValue interface{}) []T {
	var exprs = make([]T, 0)
	switch v := exprValue.(type) {
	case Expression:
		exprs = append(exprs, v.(T))
	case []Expression:
		for _, expr := range v {
			exprs = append(exprs, expr.(T))
		}
	case []T:
		exprs = append(exprs, v...)
	case []any:
		for _, expr := range v {
			exprs = append(exprs, expressionFromInterface[T](expr)...)
		}
	case string:
		exprs = append(exprs, Field(v).(T))
	default:
		var rTyp = reflect.TypeOf(exprValue)
		var rVal = reflect.ValueOf(exprValue)
		switch rTyp.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < rVal.Len(); i++ {
				var elem = rVal.Index(i).Interface()
				exprs = append(exprs, expressionFromInterface[T](elem)...)
			}
		default:
			exprs = append(exprs, Value(v).(T))
		}
	}

	return exprs
}

func Express(key interface{}, vals ...interface{}) []ClauseExpression {
	switch v := key.(type) {
	case string:
		if len(vals) == 0 {
			panic(fmt.Errorf("no values provided for key %q", v))
		}
		return []ClauseExpression{Q(v, vals...)}
	case Expression:
		var expr = &ExprGroup{children: make([]Expression, 0, len(vals)+1), op: OpAnd}
		expr.children = append(expr.children, v)
		for _, val := range vals {
			var v, ok = val.(Expression)
			if !ok {
				panic(fmt.Errorf("value %v is not an Expression", val))
			}
			expr.children = append(expr.children, v)
		}
		return []ClauseExpression{expr}
	case []Expression:
		var expr = &ExprGroup{children: make([]Expression, 0, len(vals)+1), op: OpAnd}
		expr.children = append(expr.children, v...)
		for _, val := range vals {
			var v, ok = val.(Expression)
			if !ok {
				panic(fmt.Errorf("value %v is not an Expression", val))
			}
			expr.children = append(expr.children, v)
		}
		return []ClauseExpression{expr}
	case []ClauseExpression:
		var expr = &ExprGroup{children: make([]Expression, 0, len(vals)+len(v)), op: OpAnd}
		for _, e := range v {
			expr.children = append(expr.children, e)
		}
		for _, val := range vals {
			var v, ok = val.(Expression)
			if !ok {
				panic(fmt.Errorf("value %v is not an Expression", val))
			}
			expr.children = append(expr.children, v)
		}
		return []ClauseExpression{expr}
	case map[string]interface{}:
		var expr = make([]ClauseExpression, 0, len(v))
		for k, val := range v {
			expr = append(expr, Q(k, val))
		}
		return expr
	default:
		panic(fmt.Errorf("unsupported type %T", key))
	}
}

type LookupField interface {
	attrs.FieldDefinition
	AllowedTransforms() []string
	AllowedLookups() []string
}

type ResolvedField struct {
	FieldPath         string
	Field             string
	Column            string
	AllowedTransforms []string
	AllowedLookups    []string
}

func newResolvedField(fieldPath, column string, field attrs.FieldDefinition) *ResolvedField {
	var (
		transforms []string
		lookups    []string
	)
	if v, ok := field.(LookupField); ok {
		transforms = v.AllowedTransforms()
		lookups = v.AllowedLookups()
	}
	return &ResolvedField{
		FieldPath:         fieldPath,
		Field:             field.Name(),
		Column:            column,
		AllowedTransforms: transforms,
		AllowedLookups:    lookups,
	}
}

func ResolveExpressionField(inf *ExpressionInfo, field string) *ResolvedField {
	var current, _, f, chain, aliases, isRelated, err = internal.WalkFields(inf.Model, field, inf.AliasGen)
	if err != nil {
		panic(err)
	}

	var col = &TableColumn{}
	if (!inf.ForUpdate) || (isRelated || len(chain) > 0) {
		var aliasStr string
		if len(aliases) > 0 {
			aliasStr = aliases[len(aliases)-1]
		} else {
			aliasStr = current.FieldDefs().TableName()
		}

		if vF, ok := f.(interface{ Alias() string }); ok {
			col.FieldAlias = inf.AliasGen.GetFieldAlias(
				aliasStr, vF.Alias(),
			)
		} else {
			col.TableOrAlias = aliasStr
			col.FieldColumn = f
		}

		var sql, _ = inf.FormatField(col)
		return newResolvedField(
			field, sql, f,
		)
	}

	col.FieldColumn = f
	var sql, _ = inf.FormatField(col)
	return newResolvedField(
		field, sql, f,
	)
}
