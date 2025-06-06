package expr

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

// LogicalOp represents the logical operator to use in a query.
//
// It is used to compare two values in a logical expression.
// The logical operators are used in the WHERE clause of a SQL query,
// or inside of queryset join conditions.
type LogicalOp = String

const (
	EQ  LogicalOp = "="
	NE  LogicalOp = "!="
	GT  LogicalOp = ">"
	LT  LogicalOp = "<"
	GTE LogicalOp = ">="
	LTE LogicalOp = "<="

	ADD LogicalOp = "+"
	SUB LogicalOp = "-"
	MUL LogicalOp = "*"
	DIV LogicalOp = "/"
	MOD LogicalOp = "%"

	BITAND LogicalOp = "&"
	BITOR  LogicalOp = "|"
	BITXOR LogicalOp = "^"
	BITLSH LogicalOp = "<<"
	BITRSH LogicalOp = ">>"
	BITNOT LogicalOp = "~"
)

// ExprOp represents the expression operator to use in a query.
//
// It is used to combine multiple expressions in a logical expression.
type ExprOp string

const (
	OpAnd ExprOp = "AND"
	OpOr  ExprOp = "OR"
)

type LookupExpression = func(sb *strings.Builder) []any

type Lookup interface {
	// returns the drivers that support this lookup
	// if empty, the lookup is supported by all drivers
	Drivers() []driver.Driver

	// name of the lookup
	Name() string

	// number of arguments the lookup expects, or -1 for variable arguments
	Arity() (min, max int)

	// normalize the arguments for the lookup
	NormalizeArgs(inf *ExpressionInfo, value []any) ([]any, error)

	// Resolve resolves the lookup for the given field and value
	// and generates an expression for the lookup.
	Resolve(inf *ExpressionInfo, lhsResolved Expression, args []any) LookupExpression
}

type TableColumn struct {
	// The table or alias to use in the join condition
	// If this is set, the FieldColumn must be specified
	TableOrAlias string

	// The alias for the field in the join condition.
	FieldAlias string

	// RawSQL is the raw SQL to use in the join condition
	RawSQL string

	// The field or column to use in the join condition
	FieldColumn attrs.FieldDefinition

	// ForUpdate specifies if the field should be used in an UPDATE statement
	// This will automatically append "= ?" to the SQL statement
	ForUpdate bool

	// The value to use for the placeholder if the field column is not specified
	Value any
}

func (c *TableColumn) Validate() error {
	if c.TableOrAlias != "" && (c.ForUpdate || c.RawSQL != "") {
		return fmt.Errorf("cannot format column with (ForUpdate or RawSQL) and TableOrAlias: %v", c)
	}

	if c.RawSQL == "" && c.Value == nil && c.FieldColumn == nil && c.FieldAlias == "" {
		return fmt.Errorf("cannot format column with no value, raw SQL, field alias or field column: %v", c)
	}

	if c.ForUpdate && c.Value != nil {
		return fmt.Errorf("columns do not handle update values, ForUpdate and Value cannot be used together: %v", c)
	}

	if c.ForUpdate && c.RawSQL != "" {
		return fmt.Errorf("columns do support RawSQL and ForUpdate together: %v", c)
	}

	if c.FieldColumn != nil && c.RawSQL != "" {
		return fmt.Errorf("cannot format column with both FieldColumn and RawSQL: %v", c)
	}

	if c.FieldAlias != "" && c.ForUpdate {
		return fmt.Errorf("cannot format column with ForUpdate and FieldAlias: %v", c)
	}

	if c.FieldAlias != "" && c.Value != nil {
		return fmt.Errorf("cannot format column with FieldAlias and Value: %v", c)
	}

	return nil
}

type Expression interface {
	SQL(sb *strings.Builder) []any
	Clone() Expression
	Resolve(inf *ExpressionInfo) Expression
}

type LogicalExpression interface {
	Expression
	Scope(LogicalOp, Expression) LogicalExpression
	EQ(key interface{}, vals ...interface{}) LogicalExpression
	NE(key interface{}, vals ...interface{}) LogicalExpression
	GT(key interface{}, vals ...interface{}) LogicalExpression
	LT(key interface{}, vals ...interface{}) LogicalExpression
	GTE(key interface{}, vals ...interface{}) LogicalExpression
	LTE(key interface{}, vals ...interface{}) LogicalExpression
	ADD(key interface{}, vals ...interface{}) LogicalExpression
	SUB(key interface{}, vals ...interface{}) LogicalExpression
	MUL(key interface{}, vals ...interface{}) LogicalExpression
	DIV(key interface{}, vals ...interface{}) LogicalExpression
	MOD(key interface{}, vals ...interface{}) LogicalExpression
	BITAND(key interface{}, vals ...interface{}) LogicalExpression
	BITOR(key interface{}, vals ...interface{}) LogicalExpression
	BITXOR(key interface{}, vals ...interface{}) LogicalExpression
	BITLSH(key interface{}, vals ...interface{}) LogicalExpression
	BITRSH(key interface{}, vals ...interface{}) LogicalExpression
	BITNOT(key interface{}, vals ...interface{}) LogicalExpression
}

type ClauseExpression interface {
	Expression
	IsNot() bool
	Not(b bool) ClauseExpression
	And(...Expression) ClauseExpression
	Or(...Expression) ClauseExpression
}

type NamedExpression interface {
	Expression
	FieldName() string
}

var logicalOps = map[string]LogicalOp{
	// Equality comparison operators
	"=":  EQ,
	"!=": NE,
	">":  GT,
	"<":  LT,
	">=": GTE,
	"<=": LTE,

	// Arithmetic operators
	"+": ADD,
	"-": SUB,
	"*": MUL,
	"/": DIV,
	"%": MOD,

	// Bitwise operators
	"&":  BITAND,
	"|":  BITOR,
	"^":  BITXOR,
	"<<": BITLSH,
	">>": BITRSH,
	"~":  BITNOT,
}

func Op(op any) (LogicalOp, bool) {
	var rV = reflect.ValueOf(op)
	if rV.Kind() == reflect.String {
		var strOp = rV.String()
		op, ok := logicalOps[strOp]
		return op, ok
	}
	return "", false
}
