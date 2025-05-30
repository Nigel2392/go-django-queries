package expr

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/alias"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

// LogicalOp represents the logical operator to use in a query.
//
// It is used to compare two values in a logical expression.
// The logical operators are used in the WHERE clause of a SQL query,
// or inside of queryset join conditions.
type LogicalOp string

const (
	LogicalOpEQ  LogicalOp = "="
	LogicalOpNE  LogicalOp = "!="
	LogicalOpGT  LogicalOp = ">"
	LogicalOpLT  LogicalOp = "<"
	LogicalOpGTE LogicalOp = ">="
	LogicalOpLTE LogicalOp = "<="

	LogicalOpADD LogicalOp = "+"
	LogicalOpSUB LogicalOp = "-"
	LogicalOpMUL LogicalOp = "*"
	LogicalOpDIV LogicalOp = "/"
	LogicalOpMOD LogicalOp = "%"

	LogicalOpBITAND LogicalOp = "&"
	LogicalOpBITOR  LogicalOp = "|"
	LogicalOpBITXOR LogicalOp = "^"
	LogicalOpBITLSH LogicalOp = "<<"
	LogicalOpBITRSH LogicalOp = ">>"
	LogicalOpBITNOT LogicalOp = "~"
)

// ExprOp represents the expression operator to use in a query.
//
// It is used to combine multiple expressions in a logical expression.
type ExprOp string

const (
	OpAnd ExprOp = "AND"
	OpOr  ExprOp = "OR"
)

type ExpressionInfo struct {
	Driver      driver.Driver
	Model       attrs.Definer
	AliasGen    *alias.Generator
	FormatField func(*TableColumn) (string, []any)

	// ForUpdate specifies if the expression is used in an UPDATE statement
	// or UPDATE- like statement.
	//
	// This will automatically append "= ?" to the SQL TableColumn statement
	ForUpdate bool
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
	IsNot() bool
	Not(b bool) LogicalExpression
	And(...Expression) LogicalExpression
	Or(...Expression) LogicalExpression
}

type NamedExpression interface {
	Expression
	FieldName() string
}

var logicalOps = map[string]LogicalOp{
	// Equality comparison operators
	"=":  LogicalOpEQ,
	"!=": LogicalOpNE,
	">":  LogicalOpGT,
	"<":  LogicalOpLT,
	">=": LogicalOpGTE,
	"<=": LogicalOpLTE,

	// Arithmetic operators
	"+": LogicalOpADD,
	"-": LogicalOpSUB,
	"*": LogicalOpMUL,
	"/": LogicalOpDIV,
	"%": LogicalOpMOD,

	// Bitwise operators
	"&":  LogicalOpBITAND,
	"|":  LogicalOpBITOR,
	"^":  LogicalOpBITXOR,
	"<<": LogicalOpBITLSH,
	">>": LogicalOpBITRSH,
	"~":  LogicalOpBITNOT,
}
