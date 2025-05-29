package expr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
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
		return []LogicalExpression{expr}
	case LogicalExpression:
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
	case []LogicalExpression:
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

func ResolveExpressionArgs(inf *ExpressionInfo, arguments []any) []any {
	var args = make([]any, 0, len(arguments))

	for _, arg := range arguments {

		if expr, ok := arg.(Expression); ok {
			var (
				sb    strings.Builder
				exCpy = expr.Resolve(inf)
				extra = exCpy.SQL(&sb)
				sql   = sb.String()
			)

			args = append(args, sql)
			args = append(args, extra...)
			continue
		}

		args = append(args, arg)
	}

	return args
}

func ResolveExpressionField(inf *ExpressionInfo, field string, forUpdate bool) string {
	var current, _, f, chain, aliases, isRelated, err = internal.WalkFields(inf.Model, field, inf.AliasGen)
	if err != nil {
		panic(err)
	}

	if (!forUpdate) || (isRelated || len(chain) > 0) {
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
				inf.Quote,
				inf.AliasGen.GetFieldAlias(aliasStr, vF.Alias()),
				inf.Quote,
			)
		} else {
			col = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				inf.Quote, aliasStr, inf.Quote,
				inf.Quote, f.ColumnName(), inf.Quote,
			)
		}
		return col
	}

	return fmt.Sprintf(
		"%s%s%s",
		inf.Quote, f.ColumnName(), inf.Quote,
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
