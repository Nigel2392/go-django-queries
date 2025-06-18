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
type String string

func (e String) String() string                         { return string(e) }
func (e String) SQL(sb *strings.Builder) []any          { sb.WriteString(string(e)); return []any{} }
func (e String) Clone() Expression                      { return String([]byte(e)) }
func (e String) Resolve(inf *ExpressionInfo) Expression { return e }

// field is a string type which implements the Expression interface.
// It is used to represent a field in SQL queries.
// It can be used like so:
//
//	Field("MyModel.MyField")
type field struct {
	fieldName string
	field     *ResolvedField
	used      bool
}

func Field(fld string) NamedExpression {
	return &field{fieldName: fld}
}

func (e *field) FieldName() string {
	return e.fieldName
}

func (e *field) SQL(sb *strings.Builder) []any {
	sb.WriteString(e.field.SQLText)
	return []any{}
}

func (e *field) Clone() Expression {
	return &field{fieldName: e.fieldName, field: e.field, used: e.used}
}

func (e *field) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || e.used {
		return e
	}

	var nE = e.Clone().(*field)
	nE.used = true
	nE.field = inf.ResolveExpressionField(nE.fieldName)
	return nE
}

// Value is a type that implements the Expression interface.
// It is used to represent a value in SQL queries, allowing for both safe and unsafe usage.
// It can be used like so:
//
//	Value("some value") // safe usage
//	Value("some value", true) // unsafe usage, will not use placeholders
//
// The unsafe usage allows for direct insertion of values into the SQL query, which can be dangerous if not used carefully.
type value struct {
	v           any
	used        bool
	unsafe      bool
	placeholder string // Placeholder for the value, if needed
}

func Value(v any, unsafe ...bool) Expression {
	if expr, ok := v.(Expression); ok {
		return expr
	}

	var s bool
	if len(unsafe) > 0 && unsafe[0] {
		s = true
	}
	return &value{v: normalizeDefinerArg(v), unsafe: s}
}

func (e *value) SQL(sb *strings.Builder) []any {
	if e.unsafe {
		sb.WriteString(fmt.Sprintf("%v", e.v))
		return []any{}
	}
	sb.WriteString(e.placeholder)
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
	nE.placeholder = inf.Placeholder

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

type namedExpression struct {
	field     *ResolvedField
	fieldName string
	forUpdate bool
	used      bool
	Expression
}

// As creates a NamedExpression with a specified field name and an expression.
//
// It is used to give a name to an expression, which can be useful for annotations,
// or for updating fields in a model using [Expression]s.
func As(name string, expr Expression) NamedExpression {
	if name == "" {
		panic("field name cannot be empty")
	}
	if expr == nil {
		panic("expression cannot be nil")
	}
	return &namedExpression{
		fieldName:  name,
		Expression: expr,
	}
}

func (n *namedExpression) FieldName() string {
	return n.fieldName
}

func (n *namedExpression) Clone() Expression {
	return &namedExpression{
		used:       n.used,
		field:      n.field,
		fieldName:  n.fieldName,
		forUpdate:  n.forUpdate,
		Expression: n.Expression.Clone(),
	}
}

func (n *namedExpression) Resolve(inf *ExpressionInfo) Expression {
	if inf.Model == nil || n.used {
		return n
	}

	var nE = n.Clone().(*namedExpression)
	nE.used = true
	nE.forUpdate = inf.ForUpdate

	if nE.fieldName != "" && nE.forUpdate {
		nE.field = inf.ResolveExpressionField(nE.fieldName)
	}

	nE.Expression = nE.Expression.Resolve(inf)
	return nE
}

func (n *namedExpression) SQL(sb *strings.Builder) []any {
	if !n.forUpdate {
		return n.Expression.SQL(sb)
	}

	sb.WriteString(n.field.SQLText)
	sb.WriteString(" = ")
	return n.Expression.SQL(sb)
}
