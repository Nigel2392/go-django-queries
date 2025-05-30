package expr

import (
	"fmt"
	"slices"
	"strings"
)

type chainExpr struct {
	field     string
	used      bool
	forUpdate bool
	inner     []Expression
}

func (e *chainExpr) FieldName() string {
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

func (e *chainExpr) SQL(sb *strings.Builder) []any {
	if len(e.inner) == 0 {
		panic(fmt.Errorf("SQL chainExpr has no inner expressions"))
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

func (e *chainExpr) Clone() Expression {
	var inner = slices.Clone(e.inner)
	for i := range inner {
		inner[i] = inner[i].Clone()
	}

	return &chainExpr{
		field:     e.field,
		used:      e.used,
		forUpdate: e.forUpdate,
		inner:     inner,
	}
}

func (e *chainExpr) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || e.used {
		return e
	}

	var nE = e.Clone().(*chainExpr)

	nE.used = true
	nE.forUpdate = inf.ForUpdate
	nE.field = ResolveExpressionField(inf, nE.field)

	if nE.field == "" {
		panic(fmt.Errorf("chainExpr requires a field name"))
	}

	if len(nE.inner) > 0 {
		for i, inner := range nE.inner {
			nE.inner[i] = inner.Resolve(inf)
		}
	}

	return nE
}

func Chain(expr ...any) NamedExpression {
	var inner = make([]Expression, 0, len(expr))
	var fieldName string

exprLoop:
	for i, e := range expr {

		if n, ok := e.(NamedExpression); ok && (i == 0 || i > 0 && fieldName == "") {
			fieldName = n.FieldName()
		}

		if opStr, ok := e.(string); ok {
			op, ok := logicalOps[opStr]
			if ok {
				inner = append(inner, StringExpr(op))
				continue exprLoop
			}

			if i == 0 && fieldName == "" {
				fieldName = opStr
				continue exprLoop
			}
		}

		switch v := e.(type) {
		case Expression:
			inner = append(inner, v)
		case LogicalOp:
			inner = append(inner, StringExpr(v))
		case string:
			if !strings.HasPrefix(v, "![") && !strings.HasSuffix(v, "]") {
				v = fmt.Sprintf("![%s]", v)
			}
			inner = append(inner, F(v))
		default:
			panic("unsupported type")
		}
	}

	if len(inner) == 0 {
		panic(fmt.Errorf("chainExpr requires at least one inner expression"))
	}

	return &chainExpr{
		field: fieldName,
		inner: inner,
	}
}
