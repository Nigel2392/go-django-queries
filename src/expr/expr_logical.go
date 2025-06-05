package expr

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
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

func And(exprs ...Expression) *ExprGroup {
	return &ExprGroup{children: exprs, op: OpAnd}
}

func Or(exprs ...Expression) *ExprGroup {
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
			inf, nE.field,
		)
		err error
	)
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

func (e *ExprNode) Not(not bool) ClauseExpression {
	e.not = not
	return e
}

func (e *ExprNode) IsNot() bool {
	return e.not
}

func (e *ExprNode) And(exprs ...Expression) ClauseExpression {
	return &ExprGroup{children: append([]Expression{e}, exprs...), op: OpAnd}
}

func (e *ExprNode) Or(exprs ...Expression) ClauseExpression {
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
	op       ExprOp
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
			if g.op != "" {
				sb.WriteString(" ")
				sb.WriteString(string(g.op))
				sb.WriteString(" ")
			}
		}

		args = append(args, child.SQL(sb)...)
	}
	sb.WriteString(")")
	return args
}

func (g *ExprGroup) Not(not bool) ClauseExpression {
	g.not = not
	return g
}

func (g *ExprGroup) IsNot() bool {
	return g.not
}

func (g *ExprGroup) And(exprs ...Expression) ClauseExpression {
	return &ExprGroup{children: append([]Expression{g}, exprs...), op: OpAnd}
}

func (g *ExprGroup) Or(exprs ...Expression) ClauseExpression {
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

type logicalChainExpr struct {
	parentheses bool // whether the expression should be wrapped in parentheses
	fieldName   string
	used        bool
	forUpdate   bool
	inner       []Expression
}

func L(expr ...any) LogicalExpression {
	if len(expr) == 0 {
		panic(fmt.Errorf("logicalChainExpr requires at least one inner expression"))
	}

	var fieldName string
	var inner = make([]Expression, 0, len(expr))
	for i, e := range expr {
		if n, ok := e.(NamedExpression); ok && (i == 0 || i > 0 && fieldName == "") {
			fieldName = n.FieldName()
		}
		if opStr, ok := e.(string); ok {
			op, ok := logicalOps[opStr]
			if ok {
				inner = append(inner, StringExpr(op))
				continue
			}
			if i == 0 && fieldName == "" {
				fieldName = opStr
				continue
			}
		}

		inner = append(
			inner,
			expressionFromInterface[Expression](e)...,
		)
	}

	return &logicalChainExpr{
		parentheses: false,
		fieldName:   fieldName,
		used:        false,
		forUpdate:   false,
		inner:       inner,
	}
}

func (l *logicalChainExpr) Scope(fn func() LogicalExpression) LogicalExpression {
	return &logicalChainExpr{
		fieldName: l.fieldName,
		used:      l.used,
		forUpdate: l.forUpdate,
		inner: append(slices.Clone(l.inner), &ExprGroup{
			children: []Expression{fn()},
			op:       "",
		}),
	}
}

func (l *logicalChainExpr) FieldName() string {
	if len(l.inner) == 0 {
		return ""
	}
	if l.fieldName != "" {
		return l.fieldName
	}
	for _, expr := range l.inner {
		if namer, ok := expr.(NamedExpression); ok {
			var name = namer.FieldName()
			if name != "" {
				return name
			}
		}
	}
	return ""
}

func (l *logicalChainExpr) SQL(sb *strings.Builder) []any {
	if len(l.inner) == 0 {
		panic(fmt.Errorf("SQL logicalChainExpr has no inner expressions"))
	}
	var args = make([]any, 0)
	if l.fieldName != "" {
		sb.WriteString(l.fieldName)
	}
	if l.forUpdate && l.fieldName != "" {
		sb.WriteString(" = ")
	}
	for _, inner := range l.inner {
		args = append(args, inner.SQL(sb)...)
	}
	return args
}

func (l *logicalChainExpr) Clone() Expression {
	var inner = slices.Clone(l.inner)
	for i := range inner {
		inner[i] = inner[i].Clone()
	}
	return &logicalChainExpr{
		fieldName: l.fieldName,
		used:      l.used,
		forUpdate: l.forUpdate,
		inner:     inner,
	}
}

func (l *logicalChainExpr) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || l.used {
		return l
	}
	var nE = l.Clone().(*logicalChainExpr)
	nE.used = true
	nE.forUpdate = inf.ForUpdate

	if nE.fieldName != "" {
		nE.fieldName = ResolveExpressionField(inf, nE.fieldName)
	}

	if len(nE.inner) > 0 {
		for i, inner := range nE.inner {
			nE.inner[i] = inner.Resolve(inf)
		}
	}

	return nE
}

func (l *logicalChainExpr) chain(op LogicalOp, key interface{}, vals ...interface{}) LogicalExpression {
	var (
		copyExprs = slices.Clone(l.inner)
	)
	copyExprs = append(copyExprs, StringExpr(op))

	if key != nil {
		copyExprs = append(
			copyExprs,
			expressionFromInterface[Expression](key)...,
		)
	}

	for _, val := range vals {
		copyExprs = append(
			copyExprs,
			expressionFromInterface[Expression](val)...,
		)
	}

	return &logicalChainExpr{
		fieldName: l.fieldName,
		used:      l.used,
		forUpdate: l.forUpdate,
		inner:     copyExprs,
	}
}

func (l *logicalChainExpr) EQ(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpEQ, key, vals...)
}
func (l *logicalChainExpr) NE(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpNE, key, vals...)
}
func (l *logicalChainExpr) GT(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpGT, key, vals...)
}
func (l *logicalChainExpr) LT(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpLT, key, vals...)
}
func (l *logicalChainExpr) GTE(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpGTE, key, vals...)
}
func (l *logicalChainExpr) LTE(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpLTE, key, vals...)
}
func (l *logicalChainExpr) ADD(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpADD, key, vals...)
}
func (l *logicalChainExpr) SUB(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpSUB, key, vals...)
}
func (l *logicalChainExpr) MUL(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpMUL, key, vals...)
}
func (l *logicalChainExpr) DIV(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpDIV, key, vals...)
}
func (l *logicalChainExpr) MOD(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpMOD, key, vals...)
}
func (l *logicalChainExpr) BITAND(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITAND, key, vals...)
}
func (l *logicalChainExpr) BITOR(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITOR, key, vals...)
}
func (l *logicalChainExpr) BITXOR(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITXOR, key, vals...)
}
func (l *logicalChainExpr) BITLSH(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITLSH, key, vals...)
}
func (l *logicalChainExpr) BITRSH(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITRSH, key, vals...)
}
func (l *logicalChainExpr) BITNOT(key interface{}, vals ...interface{}) LogicalExpression {
	return l.chain(LogicalOpBITNOT, key, vals...)
}
