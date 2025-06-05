package expr

import (
	"fmt"
	"strings"
)

// StringExpr is a string type which implements the Expression interface.
// It is used to represent a string value in SQL queries.
//
// It can be used like so, and supports no arguments:
//
//	StringExpr("a = b")
type StringExpr string

func (e StringExpr) SQL(sb *strings.Builder) []any {
	sb.WriteString(string(e))
	return []any{}
}

func (e StringExpr) Clone() Expression {
	return StringExpr([]byte(e))
}

// Resolve resolves the expression by returning itself - this is a no-op for StringExpr.
func (e StringExpr) Resolve(inf *ExpressionInfo) Expression {
	return e
}

type value struct {
	v      any
	used   bool
	unsafe bool
}

func Value(v any, unsafe ...bool) Expression {
	var s bool
	if len(unsafe) > 0 && unsafe[0] {
		s = true
	}
	return &value{v: v, unsafe: s}
}

func (e *value) SQL(sb *strings.Builder) []any {
	if e.unsafe {
		sb.WriteString(fmt.Sprintf("%v", e.v))
		return []any{}
	}
	sb.WriteString("?")
	return []any{e.v}
}

func (e *value) Clone() Expression {
	return &value{v: e.v, used: e.used, unsafe: e.unsafe}
}

func (e *value) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || e.used {
		return e
	}

	var nE = e.Clone().(*value)
	nE.used = true

	if !nE.unsafe {
		return nE
	}

	switch v := any(nE.v).(type) {
	case string:
		nE.v = any(inf.Quote(v))
	case []byte:
		panic("cannot use []byte as a value in an expression, use a string instead")
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		nE.v = any(fmt.Sprintf("%d", v))
	case float32, float64:
		nE.v = any(fmt.Sprintf("%f", v))
	case bool:
		if v {
			nE.v = any("1")
		} else {
			nE.v = any("0")
		}
	case nil:
		nE.v = any("NULL")
	default:
		panic(fmt.Errorf("unsupported value type %T in expression", v))
	}

	return nE
}
