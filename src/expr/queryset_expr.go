package expr

import (
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
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
	With(d driver.Driver, model attrs.Definer, quote string) Expression
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
type RawExpr struct {
	Statement string
	Fields    []string
	Params    []any
	not       bool
	used      bool
}

func (e *RawExpr) SQL(sb *strings.Builder) {
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

func (e *RawExpr) Args() []any {
	return e.Params
}

func (e *RawExpr) Not(not bool) Expression {
	e.not = not
	return e
}

func (e *RawExpr) IsNot() bool {
	return e.not
}

func (e *RawExpr) And(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpAnd}
}

func (e *RawExpr) Or(exprs ...Expression) Expression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpOr}
}

func (e *RawExpr) Clone() Expression {
	return &RawExpr{
		Statement: e.Statement,
		Fields:    e.Fields,
		Params:    e.Params,
		not:       e.not,
		used:      e.used,
	}
}

func (e *RawExpr) With(d driver.Driver, m attrs.Definer, quote string) Expression {
	if m == nil || e.used {
		return e
	}

	var nE = e.Clone().(*RawExpr)
	nE.used = true

	for i, field := range nE.Fields {
		var current, _, f, _, aliases, _, err = internal.WalkFields(m, field)
		if err != nil {
			panic(err)
		}

		var alias string
		if len(aliases) > 0 {
			alias = aliases[len(aliases)-1]
		} else {
			alias = current.FieldDefs().TableName()
		}

		nE.Fields[i] = fmt.Sprintf(
			"%s%s%s.%s%s%s",
			quote, alias, quote,
			quote, f.ColumnName(), quote,
		)
	}
	return nE
}
