package expr

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Expression interface {
	SQL(sb *strings.Builder)
	Args() []any
	Clone() Expression
	With(d driver.Driver, model attrs.Definer, quote string) Expression
}

type LogicalExpression interface {
	Expression
	IsNot() bool
	Not(b bool) LogicalExpression
	And(...Expression) LogicalExpression
	Or(...Expression) LogicalExpression
}

type NamedExpression interface {
	Expression
	FieldName() string
}

type LogicalOp string

const (
	OpAnd LogicalOp = "AND"
	OpOr  LogicalOp = "OR"
)

func Q(fieldLookup string, value ...any) *ExprNode {
	var split = strings.Split(fieldLookup, "__")
	var field string
	var lookup = "exact"
	if len(split) > 1 {
		field = split[0]
		lookup = split[1]
	} else {
		field = fieldLookup
	}

	return Expr(field, lookup, value...)
}

func And(exprs ...Expression) Expression {
	return &ExprGroup{children: exprs, op: OpAnd}
}

func Or(exprs ...Expression) Expression {
	return &ExprGroup{children: exprs, op: OpOr}
}

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

type ExprNode struct {
	sql    string
	args   []any
	not    bool
	model  attrs.Definer
	field  string
	lookup string
	used   bool
}

func Expr(field string, operation string, value ...any) *ExprNode {
	return &ExprNode{
		args:   value,
		field:  field,
		lookup: operation,
	}
}

func (e *ExprNode) With(d driver.Driver, m attrs.Definer, quote string) Expression {
	var nE = e.Clone().(*ExprNode)

	if m == nil {
		panic("model is nil")
	}

	if nE.used {
		return nE
	}

	nE.used = true

	nE.model = m

	var current, _, field, _, aliases, _, err = internal.WalkFields(m, nE.field)
	if err != nil {
		panic(err)
	}

	var col string
	var alias string
	if len(aliases) > 0 {
		alias = aliases[len(aliases)-1]
	} else {
		alias = current.FieldDefs().TableName()
	}

	// THIS HAS TO KEEP IN LINE WITH queries.AliasField.Alias()!!!
	if vF, ok := field.(interface{ Alias() string }); ok {
		col = fmt.Sprintf(
			"%s%s_%s%s",
			quote, alias, vF.Alias(), quote,
		)
	} else {
		col = fmt.Sprintf(
			"%s%s%s.%s%s%s",
			quote, alias, quote,
			quote, field.ColumnName(), quote,
		)
	}

	nE.sql, nE.args, err = newLookup(
		d, col, nE.lookup, slices.Clone(nE.args),
	)
	if err != nil {
		panic(err)
	}

	return nE
}

func (e *ExprNode) SQL(sb *strings.Builder) {
	if e.not {
		sb.WriteString("NOT (")
		sb.WriteString(e.sql)
		sb.WriteString(")")
	} else {
		sb.WriteString(e.sql)
	}
}

func (e *ExprNode) Args() []any {
	return e.args
}

func (e *ExprNode) Not(not bool) LogicalExpression {
	e.not = not
	return e
}

func (e *ExprNode) IsNot() bool {
	return e.not
}

func (e *ExprNode) And(exprs ...Expression) LogicalExpression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpAnd}
}

func (e *ExprNode) Or(exprs ...Expression) LogicalExpression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpOr}
}

func (e *ExprNode) Clone() Expression {
	return &ExprNode{
		sql:    e.sql,
		args:   e.args,
		not:    e.not,
		field:  e.field,
		lookup: e.lookup,
		model:  e.model,
		used:   e.used,
	}
}

// ExprGroup
type ExprGroup struct {
	children []Expression
	op       LogicalOp
	not      bool
}

func (g *ExprGroup) SQL(sb *strings.Builder) {
	if g.not {
		sb.WriteString("NOT ")
	}
	sb.WriteString("(")
	for i, child := range g.children {
		if i > 0 {
			sb.WriteString(" ")
			sb.WriteString(string(g.op))
			sb.WriteString(" ")
		}
		child.SQL(sb)
	}
	sb.WriteString(")")
}

func (g *ExprGroup) Args() []any {
	args := make([]any, 0)
	for _, e := range g.children {
		args = append(args, e.Args()...)
	}
	return args
}

func (g *ExprGroup) Not(not bool) LogicalExpression {
	g.not = not
	return g
}

func (g *ExprGroup) IsNot() bool {
	return g.not
}

func (g *ExprGroup) And(exprs ...Expression) LogicalExpression {
	return &ExprGroup{children: append([]Expression{g}, exprs...), op: OpAnd}
}

func (g *ExprGroup) Or(exprs ...Expression) LogicalExpression {
	return &ExprGroup{children: append([]Expression{g}, exprs...), op: OpOr}
}

func (g *ExprGroup) Clone() Expression {
	clone := make([]Expression, len(g.children))
	for i, c := range g.children {
		clone[i] = c.Clone()
	}
	return &ExprGroup{
		children: clone,
		op:       g.op,
		not:      g.not,
	}
}

func (g *ExprGroup) With(d driver.Driver, m attrs.Definer, quote string) Expression {
	var gClone = g.Clone().(*ExprGroup)
	for i, e := range gClone.children {
		gClone.children[i] = e.With(d, m, quote)
	}
	return gClone
}

// RawExpr is a function expression for SQL queries.
// It is used to represent a function call in SQL queries.
//
// It can be used like so:
//
//		RawExpr{
//			// Represent the SQL function call, with each %s being replaced by the corresponding field in fields.
//			sql:    `SUBSTR(TRIM(%s, " "), 0, 2) = ?``,
//	     	// The fields to be used in the SQL function call. Each field will be replaced by the corresponding value in args.
//			fields: []string{"myField"},
//			// The arguments to be used in the SQL function call. Each argument will be replaced by the corresponding value in args.
//			args:   []any{"ab"},
//		}
type RawExpr = RawNamedExpression

func Raw(statement string, value ...any) Expression {
	var str, fields, values = ParseExprStatement(statement, value)
	return &RawExpr{
		Statement: str,
		Fields:    fields,
		Params:    values,
	}
}

type RawNamedExpression struct {
	Statement string
	Params    []any
	Fields    []string
	Field     string
	forUpdate bool
	not       bool
	used      bool
}

var (
	exprFieldRegex = regexp.MustCompile(`!\[([^\]]*)\]`)
	exprValueRegex = regexp.MustCompile(`\?\[([^\]][0-9]*)\]`)
)

// UpdateExpr creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// The first field in the statement is used as the field name for the expression, and the rest of the fields are used as placeholders for the values.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	 # sets the field name to the first field found in the statement, I.E. ![Field1]:
//
//		expr := UpdateExpr("![Field1] = ![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//		fmt.Println(expr.SQL()) // prints: "field1 = age + ? + height + ?"
//		fmt.Println(expr.Args()) // prints: [3, 4]
//
//	 # sets the field name to the first field found in the statement, I.E. ![Field1]:
//
//		expr := UpdateExpr("![Field1] = ? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//		fmt.Println(expr.SQL()) // prints: "field1 = ? + ? + height + ? * ?"
//		fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func UpdateExpr(statement string, value ...any) NamedExpression {

	statement, fields, values := ParseExprStatement(statement, value)

	var field string
	if len(fields) > 0 {
		field = fields[0]
	} else {
		panic("no field found in statement")
	}

	return &RawNamedExpression{
		forUpdate: true,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     field,
	}
}

// F creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// The first field in the statement is used as the field name for the expression, and the rest of the fields are used as placeholders for the values.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	 # sets the field name to the first field found in the statement, I.E. ![Age]:
//
//		expr := F("![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//		fmt.Println(expr.SQL()) // prints: "table.age + ? + table.height + ?"
//		fmt.Println(expr.Args()) // prints: [3, 4]

//	 # sets the field name to the first field found in the statement, I.E. ![Height]:
//
//		expr := F("? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//		fmt.Println(expr.SQL()) // prints: "? + ? + table.height + ? * ?"
//		fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func F(statement string, value ...any) NamedExpression {
	statement, fields, values := ParseExprStatement(statement, value)

	var fieldName string
	if len(fields) > 0 {
		fieldName = fields[0]
	} else {
		panic("no field found in statement")
	}

	return &RawNamedExpression{
		forUpdate: false,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     fieldName,
	}
}

// NamedF creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	expr := NamedF("Field1", "![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//	fmt.Println(expr.SQL()) // prints: "table.age + ? + table.height + ?"
//	fmt.Println(expr.Args()) // prints: [3, 4]
//
//	expr := NamedF("Field1", "? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//	fmt.Println(expr.SQL()) // prints: "? + ? + table.height + ? * ?"
//	fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func NamedF(fieldName, stmt string, value ...any) NamedExpression {
	statement, fields, values := ParseExprStatement(stmt, value)
	return &RawNamedExpression{
		forUpdate: false,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     fieldName,
	}
}

func (e *RawNamedExpression) FieldName() string {
	return e.Field
}

func (e *RawNamedExpression) SQL(sb *strings.Builder) {
	if len(e.Fields) == 0 {
		sb.WriteString(e.Statement)
		return
	}

	var fields = make([]any, len(e.Fields))
	for i, field := range e.Fields {
		fields[i] = field
	}

	var str = fmt.Sprintf(e.Statement, fields...)
	sb.WriteString(str)
}

func (e *RawNamedExpression) Args() []any {
	return e.Params
}

func (e *RawNamedExpression) Clone() Expression {
	return &RawNamedExpression{
		forUpdate: e.forUpdate,
		Statement: e.Statement,
		Fields:    slices.Clone(e.Fields),
		Params:    slices.Clone(e.Params),
		not:       e.not,
		used:      e.used,
	}
}

func (e *RawNamedExpression) With(d driver.Driver, m attrs.Definer, quote string) Expression {
	if m == nil || e.used {
		return e
	}

	var nE = e.Clone().(*RawNamedExpression)
	nE.used = true

	for i, field := range nE.Fields {
		var current, _, f, _, aliases, isRelated, err = internal.WalkFields(m, field)
		if err != nil {
			panic(err)
		}

		if !e.forUpdate || isRelated || len(aliases) > 0 {
			var aliasStr string
			if len(aliases) > 0 {
				aliasStr = aliases[len(aliases)-1]
			} else {
				aliasStr = current.FieldDefs().TableName()
			}

			nE.Fields[i] = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				quote, aliasStr, quote,
				quote, f.ColumnName(), quote,
			)
			continue
		}

		nE.Fields[i] = fmt.Sprintf(
			"%s%s%s",
			quote, f.ColumnName(), quote,
		)
	}

	return nE
}
