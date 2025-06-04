package queries

import (
	"fmt"
	"strings"

	_ "unsafe"

	"github.com/Nigel2392/go-django-queries/src/expr"
)

//go:linkname newFunc github.com/Nigel2392/go-django-queries/src/expr.newFunc
func newFunc(funcLookup string, value []any, expr ...any) *expr.Function

var _ expr.Expression = (*subqueryExpr)(nil)

type subqueryExpr struct {
	field expr.Expression
	q     QueryInfo
	op    string
	not   bool
	used  bool
}

func (s *subqueryExpr) SQL(sb *strings.Builder) []any {
	var written bool
	var args = make([]any, 0)
	if s.field != nil {
		args = append(args, s.field.SQL(sb)...)
		written = true
	}

	if s.not {
		if written {
			sb.WriteString(" ")
		}

		sb.WriteString("NOT ")
		written = true
	}

	if s.op != "" {
		if written {
			sb.WriteString(" ")
		}

		sb.WriteString(s.op)
		written = true
	}

	var sql = s.q.SQL()
	if sql != "" {
		if written {
			sb.WriteString(" ")
		}

		sb.WriteString("(")
		sb.WriteString(sql)
		sb.WriteString(")")
	}

	args = append(args, s.q.Args()...)
	return args
}

func (s *subqueryExpr) Clone() expr.Expression {
	return &subqueryExpr{
		q:     s.q,
		not:   s.not,
		used:  s.used,
		field: s.field,
		op:    s.op,
	}
}

func (s *subqueryExpr) Resolve(inf *expr.ExpressionInfo) expr.Expression {
	if inf.Model == nil || s.used {
		return s
	}

	var nE = s.Clone().(*subqueryExpr)
	nE.used = true

	if nE.field != nil {
		nE.field = nE.field.Resolve(inf)
	}

	return nE
}

//__expr_exists
//__expr_not_exists
//__expr_in
//__expr_not_in

func Subquery(qs *GenericQuerySet) expr.Expression {
	q := qs.queryAll()
	return &subqueryExpr{
		q: q,
	}
}

func SubqueryCount(qs *GenericQuerySet) *subqueryExpr {
	q := qs.queryCount()
	return &subqueryExpr{
		q:    q,
		not:  false,
		used: false,
		op:   "COUNT",
	}
}

func SubqueryExists(qs *GenericQuerySet) expr.Expression {
	q := qs.queryAll()
	return &subqueryExpr{
		q:    q,
		not:  false,
		used: false,
		op:   "EXISTS",
	}
}

func SubqueryIn(field any, qs *GenericQuerySet) expr.Expression {
	q := qs.queryAll()
	var f expr.NamedExpression
	switch v := field.(type) {
	case expr.NamedExpression:
		f = v
	case string:
		f = expr.F(fmt.Sprintf("![%s]", v))
	default:
		panic(fmt.Errorf("invalid type %T", v))
	}
	return &subqueryExpr{
		q:     q,
		not:   false,
		used:  false,
		op:    "IN",
		field: f,
	}
}
