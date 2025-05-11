package expr

import (
	"fmt"
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
			panic(fmt.Errorf("index %d out of range in statement %q", idx, statement))
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

func Express(key interface{}, vals ...interface{}) []LogicalExpression {
	switch v := key.(type) {
	case string:
		if len(vals) == 0 {
			panic(fmt.Errorf("no values provided for key %q", v))
		}
		return []LogicalExpression{Q(v, vals...)}
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
		return []LogicalExpression{expr}
	case map[string]interface{}:
		var expr = make([]LogicalExpression, 0, len(v))
		for k, val := range v {
			expr = append(expr, Q(k, val))
		}
		return expr
	default:
		panic(fmt.Errorf("unsupported type %T", key))
	}
}

func ResolveExpressionField(m attrs.Definer, field string, quote string, forUpdate bool) string {
	var current, _, f, _, aliases, isRelated, err = internal.WalkFields(m, field)
	if err != nil {
		panic(err)
	}

	if (!forUpdate) || (isRelated || len(aliases) > 0) {
		var aliasStr string
		if len(aliases) > 0 {
			aliasStr = aliases[len(aliases)-1]
		} else {
			aliasStr = current.FieldDefs().TableName()
		}

		var col string
		if vF, ok := f.(interface{ Alias() string }); ok {
			col = fmt.Sprintf(
				"%s%s%s",
				quote, internal.NewAlias(aliasStr, vF.Alias()), quote,
			)
		} else {
			col = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				quote, aliasStr, quote,
				quote, f.ColumnName(), quote,
			)
		}
		return col
	}

	return fmt.Sprintf(
		"%s%s%s",
		quote, f.ColumnName(), quote,
	)
}

func normalizeArgs(op string, value []any) []any {
	if len(value) > 0 {
		switch op {
		case "icontains", "contains":
			for i := range value {
				if s, ok := value[i].(string); ok {
					value[i] = "%" + s + "%"
				}
			}
		case "istartswith", "startswith":
			for i := range value {
				if s, ok := value[i].(string); ok {
					value[i] = s + "%"
				}
			}
		case "iendswith", "endswith":
			for i := range value {
				if s, ok := value[i].(string); ok {
					value[i] = "%" + s
				}
			}
		}
	}
	return value
}
