package expr

import (
	"database/sql/driver"
	"fmt"

	"github.com/Nigel2392/go-django-queries/src/alias"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ExpressionLookupInfo struct {
	// PrepForLikeQuery is a function that prepares the value for a LIKE query.
	//
	// It takes any value and returns a string that is properly formatted and
	// escaped for use in a LIKE query.
	PrepForLikeQuery func(any) string

	// FormatLookupCol is a function that formats the left-hand side and right-hand side of
	// a lookup operation in the query.
	//
	// It takes the operator and the left-hand side value and returns a formatted string.
	// This is used to format the left-hand side of an operator in the query for iexact, icontains, etc.
	//
	// The default compiler has a format function for the following operators:
	//
	// - iexact
	// - icontains
	// - istartswith
	// - iendswith
	FormatLookupCol func(string, string) string

	// LogicalOpRHS is a map of logical operators to functions that format the right-hand side of the operator.
	//
	// It takes the logical operator and the right-hand side value and returns a formatted string.
	//
	// The defualt compiler has logical operators for:
	//
	// - EQ
	// - NE
	// - GT
	// - LT
	// - GTE
	// - LTE
	// - ADD
	// - SUB
	// - MUL
	// - DIV
	// - MOD
	// - BITAND
	// - BITOR
	// - BITXOR
	// - BITLSH
	// - BITRSH
	// - BITNOT
	LogicalOpRHS map[LogicalOp]func(rhs string, value []any) (string, []any)

	// Operators is a map of lookup operations to format strings.
	//
	// It is used to format the operators in the query.
	//
	// Use ExpressionInfo.FormatOp(...) to format the operator.
	//
	// The default compiler has operators for:
	//
	// - iexact
	// - contains
	// - icontains
	// - regex
	// - iregex
	// - startswith
	// - endswith
	// - istartswith
	// - iendswith
	OperatorsRHS map[string]string

	// PatternOps is a map of pattern operators to format strings.
	//
	// It is used to format operators when the operator is used as
	// an expression in a pattern match, such as 'contains' or 'icontains'.
	//
	// Use ExpressionInfo.PatternOp(...) to format the pattern operator.
	//
	// The default compiler supports pattern operators for:
	//
	// - contains
	// - icontains
	// - startswith
	// - endswith
	// - istartswith
	// - iendswith
	PatternOpsRHS map[string]string
}

type ExpressionInfo struct {
	// Driver is the driver used to execute the query.
	Driver driver.Driver

	// Model is the base model of the queryset.
	Model attrs.Definer

	// The AliasGen is used to generate aliases for the fields in the query.
	AliasGen *alias.Generator

	// Placeholder is the placeholder to use in the query.
	//
	// It is used to format the placeholders in the query.
	Placeholder string

	// FormatField is a function that formats the field for the SQL query.
	//
	// It takes a TableColumn and returns the formatted field as a string
	// and a slice of possible args that can be used in the query.
	FormatField func(*TableColumn) (string, []any)

	// Quote is a function that quotes the given string for use in a SQL query.
	Quote func(string) string

	// Lookups provides information about how to format the lookups
	// used in the query.
	Lookups ExpressionLookupInfo

	// ForUpdate specifies if the expression is used in an UPDATE statement
	// or UPDATE- like statement.
	//
	// This will automatically append "= ?" to the SQL TableColumn statement
	ForUpdate bool
}

func (inf *ExpressionLookupInfo) FormatLogicalOpRHS(op LogicalOp, rhs string, values ...any) (string, []any) {
	if inf.LogicalOpRHS == nil {
		panic("ExpressionInfo.LogicalOpRHS is nil, cannot format logical operator")
	}
	if format, ok := inf.LogicalOpRHS[op]; ok {
		return format(rhs, values)
	}
	panic(fmt.Errorf("unknown logical operator %s: compiler does not support operator", op))
}

func (inf *ExpressionLookupInfo) FormatOpRHS(op string, fmtArgs ...any) string {
	if inf.OperatorsRHS == nil {
		panic("ExpressionInfo.Operators is nil, cannot format operator")
	}
	if format, ok := inf.OperatorsRHS[op]; ok {
		return fmt.Sprintf(format, fmtArgs...)
	}
	panic(fmt.Errorf("unknown operator %s: compiler does not support operator", op))
}

func (inf *ExpressionLookupInfo) PatternOpRHS(op string, fmtArgs ...any) string {
	if inf.PatternOpsRHS == nil {
		panic("ExpressionInfo.PatternOps is nil, cannot format pattern operator")
	}
	if format, ok := inf.PatternOpsRHS[op]; ok {
		return fmt.Sprintf(format, fmtArgs...)
	}
	panic(fmt.Errorf("unknown pattern operator %s: compiler does not support operator", op))
}
