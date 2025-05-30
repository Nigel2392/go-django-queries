package expr

import "strings"

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
