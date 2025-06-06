package expr

import (
	"fmt"
	"slices"
	"strings"
)

type function[LookupType comparable] struct {
	reg        *_lookups[any, LookupType]
	sql        func(col any, value []any) (sql string, args []any, err error)
	funcLookup LookupType
	field      string
	args       []any
	used       bool
	inner      []Expression
}

type Function = function[string]

func (e *function[T]) FieldName() string {
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

func (e *function[T]) SQL(sb *strings.Builder) []any {
	if e.sql == nil {
		panic(fmt.Errorf("SQL function %v not provided", e.funcLookup))
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
		slices.Clone(e.args),
	)

	if err != nil {
		panic(err)
	}

	sb.WriteString(sql)

	return append(args, params...)
}

func (e *function[T]) Clone() Expression {
	var inner = slices.Clone(e.inner)
	for i := range inner {
		inner[i] = inner[i].Clone()
	}

	return &function[T]{
		reg:        e.reg,
		sql:        e.sql,
		funcLookup: e.funcLookup,
		field:      e.field,
		args:       slices.Clone(e.args),
		used:       e.used,
		inner:      inner,
	}
}

func (e *function[T]) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || e.used {
		return e
	}

	var nE = e.Clone().(*function[T])
	nE.used = true

	if len(nE.inner) > 0 {
		for i, inner := range nE.inner {
			nE.inner[i] = inner.Resolve(inf)
		}
	}

	var sql, ok = e.reg.lookupFunc(
		inf.Driver, nE.funcLookup,
	)
	if !ok {
		panic(fmt.Errorf("could not resolve SQL function %v", nE.funcLookup))
	}

	nE.sql = func(col any, value []any) (string, []any, error) {
		return sql(inf.Driver, col, value)
	}

	if nE.field != "" {
		nE.field = ResolveExpressionField(inf, nE.field)
	}

	return nE
}

func newFunc[T comparable](registry *_lookups[any, T], funcLookup T, value []any, expr ...any) *function[T] {
	var inner = make([]Expression, 0, len(expr))
	for _, e := range expr {
		switch v := e.(type) {
		case Expression:
			inner = append(inner, v)
		case string:
			inner = append(inner, Field(v))
		default:
			panic("unsupported type")
		}
	}

	return &function[T]{
		reg:        registry,
		funcLookup: funcLookup,
		args:       value,
		inner:      inner,
	}
}

func FuncSum(expr ...any) *Function {
	return newFunc(funcLookups, "SUM", []any{}, expr...)
}

func FuncCount(expr ...any) *Function {
	return newFunc(funcLookups, "COUNT", []any{}, expr...)
}

func FuncAvg(expr ...any) *Function {
	return newFunc(funcLookups, "AVG", []any{}, expr...)
}

func FuncMax(expr ...any) *Function {
	return newFunc(funcLookups, "MAX", []any{}, expr...)
}

func FuncMin(expr ...any) *Function {
	return newFunc(funcLookups, "MIN", []any{}, expr...)
}

func FuncCoalesce(expr ...any) *Function {
	return newFunc(funcLookups, "COALESCE", []any{}, expr...)
}

func FuncConcat(expr ...any) *Function {
	return newFunc(funcLookups, "CONCAT", []any{}, expr...)
}

func FuncSubstr(expr any, start, length any) *Function {
	return newFunc(funcLookups, "SUBSTR", []any{start, length}, expr)
}

func FuncUpper(expr any) *Function {
	return newFunc(funcLookups, "UPPER", []any{}, expr)
}

func FuncLower(expr any) *Function {
	return newFunc(funcLookups, "LOWER", []any{}, expr)
}

func FuncLength(expr any) *Function {
	return newFunc(funcLookups, "LENGTH", []any{}, expr)
}

func FuncNow() *Function {
	return newFunc(funcLookups, "NOW", []any{}, nil)
}
