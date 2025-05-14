package expr

import (
	"database/sql/driver"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/alias"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ExpressionInfo struct {
	Driver   driver.Driver
	Model    attrs.Definer
	AliasGen *alias.Generator
	Quote    string
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
