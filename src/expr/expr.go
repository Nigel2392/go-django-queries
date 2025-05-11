package expr

import (
	"database/sql/driver"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Expression interface {
	SQL(sb *strings.Builder) []any
	Clone() Expression
	Resolve(d driver.Driver, model attrs.Definer, quote string) Expression
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
