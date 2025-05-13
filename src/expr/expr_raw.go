package expr

import (
	"database/sql/driver"
	"fmt"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

// RawExpr is a function expression for SQL queries.
// It is used to represent a function call in SQL queries.
//
// It can be used like so:
//
//		RawExpr{
//			// Represent the SQL function call, with each %s being replaced by the corresponding field in fields.
//			Statement:    `SUBSTR(TRIM(%s, " "), 0, 2) = ?``,
//	     	// The fields to be used in the SQL function call. Each field will be replaced by the corresponding value in args.
//			Fields: []string{"myField"},
//			// The arguments to be used in the SQL function call. Each argument will be replaced by the corresponding value in args.
//			Params:   []any{"ab"},
//		}
type RawExpr = RawNamedExpression

func Raw(statement string, value ...any) Expression {
	var str, fields, values = ParseExprStatement(statement, value)
	return &RawExpr{
		Statement: str,
		Fields:    fields,
		Params:    values,
	}
}

type RawNamedExpression struct {
	Statement string
	Params    []any
	Fields    []string
	Field     string
	forUpdate bool
	not       bool
	used      bool
}

// U creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// It is meant to be used for create & update statements, where there should be no alias used for the field name.
//
// The first field in the statement is used as the field name for the expression, and the rest of the fields are used as placeholders for the values.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	 # sets the field name to the first field found in the statement, I.E. ![Field1]:
//
//		expr := U("![Field1] = ![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//		fmt.Println(expr.SQL()) // prints: "field1 = age + ? + height + ?"
//		fmt.Println(expr.Args()) // prints: [3, 4]
//
//	 # sets the field name to the first field found in the statement, I.E. ![Field1]:
//
//		expr := U("![Field1] = ? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//		fmt.Println(expr.SQL()) // prints: "field1 = ? + ? + height + ? * ?"
//		fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func U(statement string, value ...any) NamedExpression {

	statement, fields, values := ParseExprStatement(statement, value)

	var field string
	if len(fields) > 0 {
		field = fields[0]
	} else {
		panic("no field found in statement")
	}

	return &RawNamedExpression{
		forUpdate: true,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     field,
	}
}

// F creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// The first field in the statement is used as the field name for the expression, and the rest of the fields are used as placeholders for the values.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	 # sets the field name to the first field found in the statement, I.E. ![Age]:
//
//		expr := F("![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//		fmt.Println(expr.SQL()) // prints: "table.age + ? + table.height + ?"
//		fmt.Println(expr.Args()) // prints: [3, 4]

//	 # sets the field name to the first field found in the statement, I.E. ![Height]:
//
//		expr := F("? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//		fmt.Println(expr.SQL()) // prints: "? + ? + table.height + ? * ?"
//		fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func F(statement string, value ...any) NamedExpression {
	statement, fields, values := ParseExprStatement(statement, value)

	var fieldName string
	if len(fields) > 0 {
		fieldName = fields[0]
	} else {
		panic("no field found in statement")
	}

	return &RawNamedExpression{
		forUpdate: false,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     fieldName,
	}
}

// NamedF creates a new RawNamedExpression with the given statement and values.
// It parses the statement to extract the fields and values, and returns a pointer to the new RawNamedExpression.
//
// The statement should contain placeholders for the fields and values, which will be replaced with the actual values.
//
// The placeholders for fields should be in the format ![FieldName], and the placeholders for values should be in the format ?[Index],
// or the values should use the regular SQL placeholder directly (database driver dependent).
//
// Example usage:
//
//	expr := NamedF("Field1", "![Age] + ?[1] + ![Height] + ?[2] * ?[1]", 3, 4)
//	fmt.Println(expr.SQL()) // prints: "table.age + ? + table.height + ?"
//	fmt.Println(expr.Args()) // prints: [3, 4]
//
//	expr := NamedF("Field1", "? + ? + ![Height] + ? * ?", 4, 5, 6, 7)
//	fmt.Println(expr.SQL()) // prints: "? + ? + table.height + ? * ?"
//	fmt.Println(expr.Args()) // prints: [4, 5, 6, 7]
func NamedF(fieldName, stmt string, value ...any) NamedExpression {
	statement, fields, values := ParseExprStatement(stmt, value)
	return &RawNamedExpression{
		forUpdate: false,
		Statement: statement,
		Params:    values,
		Fields:    fields,
		Field:     fieldName,
	}
}

func (e *RawNamedExpression) FieldName() string {
	return e.Field
}

func (e *RawNamedExpression) SQL(sb *strings.Builder) []any {
	if len(e.Fields) == 0 {
		sb.WriteString(e.Statement)
		return e.Params
	}

	var fields = make([]any, len(e.Fields))
	for i, field := range e.Fields {
		fields[i] = field
	}

	var str = fmt.Sprintf(e.Statement, fields...)
	sb.WriteString(str)
	return e.Params
}

func (e *RawNamedExpression) Clone() Expression {
	return &RawNamedExpression{
		forUpdate: e.forUpdate,
		Statement: e.Statement,
		Fields:    slices.Clone(e.Fields),
		Params:    slices.Clone(e.Params),
		not:       e.not,
		used:      e.used,
	}
}

func (e *RawNamedExpression) Resolve(d driver.Driver, m attrs.Definer, quote string) Expression {
	if m == nil || e.used {
		return e
	}

	var nE = e.Clone().(*RawNamedExpression)
	nE.used = true

	//if len(nE.Params) > 0 {
	//	nE.Params = ResolveExpressionArgs(d, m, nE.Params, quote)
	//}

	for i, field := range nE.Fields {
		nE.Fields[i] = ResolveExpressionField(m, field, quote, e.forUpdate)
	}

	return nE
}
