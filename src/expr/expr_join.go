package expr

import (
	"fmt"
	"slices"
	"strings"
)

type multipleExpr struct {
	field     string
	used      bool
	forUpdate bool
	inner     []Expression
}

func (e *multipleExpr) FieldName() string {
	if e.field != "" {
		return e.field
	}

	for _, expr := range e.inner {
		if namer, ok := expr.(NamedExpression); ok {
			var name = namer.FieldName()
			if name != "" {
				return name
			}
		}
	}

	return ""
}

func (e *multipleExpr) SQL(sb *strings.Builder) []any {
	if len(e.inner) == 0 {
		panic(fmt.Errorf("SQL multipleExpr has no inner expressions"))
	}

	if e.field != "" {
		sb.WriteString(e.field)
	}

	if e.forUpdate {
		sb.WriteString(" = ")
	}

	var args = make([]any, 0)
	for _, inner := range e.inner {
		args = append(args, inner.SQL(sb)...)
	}

	return args
}

func (e *multipleExpr) Clone() Expression {
	var inner = slices.Clone(e.inner)
	for i := range inner {
		inner[i] = inner[i].Clone()
	}

	return &multipleExpr{
		field:     e.field,
		used:      e.used,
		forUpdate: e.forUpdate,
		inner:     inner,
	}
}

func (e *multipleExpr) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || e.used {
		return e
	}

	var nE = e.Clone().(*multipleExpr)

	nE.used = true
	nE.forUpdate = inf.ForUpdate
	nE.field = ResolveExpressionField(inf, nE.field)

	if nE.field == "" {
		panic(fmt.Errorf("multipleExpr requires a field name"))
	}

	if len(nE.inner) > 0 {
		for i, inner := range nE.inner {
			nE.inner[i] = inner.Resolve(inf)
		}
	}

	return nE
}

func Multiple(expr ...any) NamedExpression {
	var inner = make([]Expression, 0, len(expr))
	var fieldName string
	for i, e := range expr {

		if n, ok := e.(NamedExpression); ok && (i == 0 || i > 0 && fieldName == "") {
			fieldName = n.FieldName()
		}

		if s, ok := e.(string); ok && (i == 0 || i > 0 && fieldName == "") {
			fieldName = s
			continue
		}

		switch v := e.(type) {
		case Expression:
			inner = append(inner, v)
		case string:
			inner = append(inner, StringExpr(v))
		default:
			panic("unsupported type")
		}
	}

	if len(inner) == 0 {
		panic(fmt.Errorf("multipleExpr requires at least one inner expression"))
	}

	return &multipleExpr{
		field: fieldName,
		inner: inner,
	}
}
