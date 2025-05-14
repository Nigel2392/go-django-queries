package expr

import (
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

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

func (e *ExprNode) Resolve(inf *ExpressionInfo) Expression {
	var nE = e.Clone().(*ExprNode)

	if inf.Model == nil {
		panic("model is nil")
	}

	if nE.used {
		return nE
	}

	nE.used = true
	nE.model = inf.Model

	var (
		col = ResolveExpressionField(
			inf, nE.field, false,
		)
		err error
	)
	// nE.args = ResolveExpressionArgs(d, m, nE.args, quote)
	nE.sql, nE.args, err = typeLookups.lookup(
		inf.Driver, col, nE.lookup, slices.Clone(nE.args),
	)

	if err != nil {
		panic(err)
	}

	return nE
}

func (e *ExprNode) SQL(sb *strings.Builder) []any {
	if e.not {
		sb.WriteString("NOT (")
		sb.WriteString(e.sql)
		sb.WriteString(")")
	} else {
		sb.WriteString(e.sql)
	}
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

func (g *ExprGroup) SQL(sb *strings.Builder) []any {
	if g.not {
		sb.WriteString("NOT ")
	}
	sb.WriteString("(")
	var args = make([]any, 0)
	for i, child := range g.children {
		if i > 0 {
			sb.WriteString(" ")
			sb.WriteString(string(g.op))
			sb.WriteString(" ")
		}

		args = append(args, child.SQL(sb)...)
	}
	sb.WriteString(")")
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

func (g *ExprGroup) Resolve(inf *ExpressionInfo) Expression {
	var gClone = g.Clone().(*ExprGroup)
	for i, e := range gClone.children {
		gClone.children[i] = e.Resolve(inf)
	}
	return gClone
}
