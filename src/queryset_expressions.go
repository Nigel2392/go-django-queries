package queries

import (
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Expression interface {
	SQL(sb *strings.Builder)
	Args() []any
	IsNot() bool
	Not(b bool) Expression
	And(...Expression) Expression
	Or(...Expression) Expression
	Clone() Expression
	With(model attrs.Definer, quote string) Expression
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

func express(key interface{}, vals ...interface{}) []Expression {
	switch v := key.(type) {
	case string:
		if len(vals) == 0 {
			panic(fmt.Errorf("no values provided for key %q", v))
		}
		return []Expression{Q(v, vals...)}
	case Expression:
		var expr = make([]Expression, 0, len(vals)+1)
		expr = append(expr, v)
		for _, val := range vals {
			var v, ok = val.(Expression)
			if !ok {
				panic(fmt.Errorf("value %v is not an Expression", val))
			}
			expr = append(expr, v)
		}
		return expr
	case map[string]interface{}:
		var expr = make([]Expression, 0, len(v))
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

func walkFields(m attrs.Definer, column string) (definer attrs.Definer, parent attrs.Definer, f attrs.Field, chain []string, isRelated bool, err error) {
	var parts = strings.Split(column, ".")
	var current = m
	var field attrs.Field
	var chainParts = make([]string, 0, len(parts))
	for i, part := range parts {
		var defs = current.FieldDefs()
		var f, ok = defs.Field(part)
		if !ok {
			return nil, nil, nil, nil, false, fmt.Errorf("field %q not found in %T", part, current)
		}

		chainParts = append(chainParts, part)
		field = f

		var rel = f.Rel()
		if i == len(parts)-1 && rel == nil {
			break
		}

		parent = current
		current = rel
		if current == nil {
			return nil, nil, nil, nil, false, fmt.Errorf("field %q has no related model in %T", part, f)
		}

		isRelated = true
	}

	return current, parent, field, chainParts, isRelated, nil
}

type ExprNode struct {
	sql    string
	args   []any
	not    bool
	model  attrs.Definer
	column string
	lookup string
	used   bool
}

func Expr(field string, operation string, value ...any) *ExprNode {
	sqlCond, err := sqlCondition(field, operation)
	if err != nil {
		panic(err)
	}
	return &ExprNode{
		sql:    sqlCond,
		args:   normalizeArgs(operation, value),
		column: field,
		lookup: operation,
	}
}

func (e *ExprNode) With(m attrs.Definer, quote string) Expression {
	var nE = e.Clone().(*ExprNode)

	if m == nil || e.used {
		return e
	}

	nE.used = true

	nE.model = m

	var current, _, field, _, _, err = walkFields(m, nE.column)
	if err != nil {
		panic(err)
	}

	var defs = current.FieldDefs()
	col := fmt.Sprintf(
		"%s%s%s.%s%s%s",
		quote, defs.TableName(), quote,
		quote, field.ColumnName(), quote,
	)

	switch nE.lookup {
	case "exact":
		nE.sql = fmt.Sprintf("%s = ?", col)
	case "icontains", "istartswith", "iendswith":
		nE.sql = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", col)
	case "contains", "startswith", "endswith":
		nE.sql = fmt.Sprintf("%s LIKE ?", col)
	case "gt":
		nE.sql = fmt.Sprintf("%s > ?", col)
	case "gte":
		nE.sql = fmt.Sprintf("%s >= ?", col)
	case "lt":
		nE.sql = fmt.Sprintf("%s < ?", col)
	case "lte":
		nE.sql = fmt.Sprintf("%s <= ?", col)
	default:
		panic(fmt.Errorf("unsupported lookup: %s", e.lookup))
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

func (e *ExprNode) Not(not bool) Expression {
	e.not = not
	return e
}

func (e *ExprNode) IsNot() bool {
	return e.not
}

func (e *ExprNode) And(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpAnd}
}

func (e *ExprNode) Or(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpOr}
}

func (e *ExprNode) Clone() Expression {
	return &ExprNode{
		sql:    e.sql,
		args:   e.args,
		not:    e.not,
		column: e.column,
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

func (g *ExprGroup) Not(not bool) Expression {
	g.not = not
	return g
}

func (g *ExprGroup) IsNot() bool {
	return g.not
}

func (g *ExprGroup) And(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{g}, exprs...), op: OpAnd}
}

func (g *ExprGroup) Or(exprs ...Expression) Expression {
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

func (g *ExprGroup) With(m attrs.Definer, quote string) Expression {
	for _, e := range g.children {
		e.With(m, quote)
	}
	return g
}

// FuncExpr is a function expression for SQL queries.
// It is used to represent a function call in SQL queries.
//
// It can be used like so:
//
//		FuncExpr{
//			// Represent the SQL function call, with each %s being replaced by the corresponding field in fields.
//			sql:    `SUBSTR(TRIM(%s, " "), 0, 2) = ?``,
//	     	// The fields to be used in the SQL function call. Each field will be replaced by the corresponding value in args.
//			fields: []string{"myField"},
//			// The arguments to be used in the SQL function call. Each argument will be replaced by the corresponding value in args.
//			args:   []any{"ab"},
//		}
type FuncExpr struct {
	Statement string
	Fields    []string
	Params    []any
	not       bool
	used      bool
}

func (e *FuncExpr) SQL(sb *strings.Builder) {
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

func (e *FuncExpr) Args() []any {
	return e.Params
}

func (e *FuncExpr) Not(not bool) Expression {
	e.not = not
	return e
}

func (e *FuncExpr) IsNot() bool {
	return e.not
}

func (e *FuncExpr) And(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpAnd}
}

func (e *FuncExpr) Or(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpOr}
}

func (e *FuncExpr) Clone() Expression {
	return &FuncExpr{
		Statement: e.Statement,
		Fields:    e.Fields,
		Params:    e.Params,
		not:       e.not,
		used:      e.used,
	}
}

func (e *FuncExpr) With(m attrs.Definer, quote string) Expression {
	if m == nil || e.used {
		return e
	}

	e.used = true

	for i, field := range e.Fields {
		var current, _, f, _, _, err = walkFields(m, field)
		if err != nil {
			panic(err)
		}
		var defs = current.FieldDefs()
		e.Fields[i] = fmt.Sprintf(
			"%s%s%s.%s%s%s",
			quote, defs.TableName(), quote,
			quote, f.ColumnName(), quote,
		)
	}
	return e
}
