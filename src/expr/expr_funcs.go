package expr

import (
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Func struct {
	sql        func(col any, value []any) (sql string, args []any, err error)
	funcLookup string
	field      string
	args       []any
	used       bool
	forUpdate  bool
	inner      []Expression
}

func (e *Func) FieldName() string {
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

func (e *Func) SQL(sb *strings.Builder) []any {
	if e.sql == nil {
		panic(fmt.Errorf("SQL function %q not provided", e.funcLookup))
	}

	var innerBuf strings.Builder
	var args = make([]any, 0)
	for i, inner := range e.inner {
		if i > 0 {
			innerBuf.WriteString(", ")
		}
		args = append(args, inner.SQL(&innerBuf)...)
	}

	sql, params, err := e.sql(
		innerBuf.String(),
		append(e.args, args...),
	)

	if err != nil {
		panic(err)
	}

	sb.WriteString(sql)
	return params
}

func (e *Func) Clone() Expression {
	var inner = slices.Clone(e.inner)
	for i := range inner {
		inner[i] = inner[i].Clone()
	}

	return &Func{
		sql:        e.sql,
		funcLookup: e.funcLookup,
		field:      e.field,
		args:       slices.Clone(e.args),
		used:       e.used,
		forUpdate:  e.forUpdate,
		inner:      inner,
	}
}

func (e *Func) Resolve(d driver.Driver, m attrs.Definer, quote string) Expression {
	if m == nil || e.used {
		return e
	}

	var nE = e.Clone().(*Func)
	nE.used = true

	if len(nE.inner) > 0 {
		for i, inner := range nE.inner {
			nE.inner[i] = inner.Resolve(d, m, quote)
		}
	}

	var ok bool
	nE.sql, ok = funcLookups.lookupFunc(
		d, nE.funcLookup,
	)
	if !ok {
		panic(fmt.Errorf("could not resolve SQL function %s", nE.funcLookup))
	}

	if nE.field != "" {
		nE.field = ResolveExpressionField(m, nE.field, quote, e.forUpdate)
	}

	return nE
}

func newFunc(funcLookup string, value []any, expr ...any) *Func {
	var inner = make([]Expression, 0, len(expr))
	for _, e := range expr {
		switch v := e.(type) {
		case Expression:
			inner = append(inner, v)
		case string:
			if !strings.HasPrefix(v, "![") && !strings.HasSuffix(v, "]") {
				v = fmt.Sprintf("![%s]", v)
			}
			inner = append(inner, F(v))
		default:
			panic("unsupported type")
		}
	}

	return &Func{
		funcLookup: funcLookup,
		args:       value,
		inner:      inner,
	}
}

func FuncSum(expr ...any) *Func {
	return newFunc("SUM", []any{}, expr...)
}

func FuncCount(expr ...any) *Func {
	return newFunc("COUNT", []any{}, expr...)
}

func FuncAvg(expr ...any) *Func {
	return newFunc("AVG", []any{}, expr...)
}

func FuncMax(expr ...any) *Func {
	return newFunc("MAX", []any{}, expr...)
}

func FuncMin(expr ...any) *Func {
	return newFunc("MIN", []any{}, expr...)
}

func FuncCoalesce(expr ...any) *Func {
	return newFunc("COALESCE", []any{}, expr...)
}

func FuncConcat(expr ...any) *Func {
	return newFunc("CONCAT", []any{}, expr...)
}

func FuncSubstr(expr any, start, length any) *Func {
	return newFunc("SUBSTR", []any{start, length}, expr)
}

func FuncUpper(expr any) *Func {
	return newFunc("UPPER", []any{}, expr)
}

func FuncLength(expr any) *Func {
	return newFunc("LENGTH", []any{}, expr)
}

func FuncNow() *Func {
	return newFunc("NOW", []any{}, nil)
}
