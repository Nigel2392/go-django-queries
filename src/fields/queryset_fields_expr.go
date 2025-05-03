package fields

import (
	"database/sql/driver"
	"strings"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var _ queries.VirtualField = (*ExpressionField[any])(nil)

type ExpressionField[T any] struct {
	*DataModelField[T]

	// expr is the expression used to calculate the field's value
	expr expr.Expression
}

func NewVirtualField[T any](forModel attrs.Definer, dst any, name string, expr expr.Expression) *ExpressionField[T] {
	var f = &ExpressionField[T]{
		DataModelField: NewDataModelField[T](forModel, dst, name),
		expr:           expr,
	}
	return f
}

func (f *ExpressionField[T]) Alias() string {
	return f.DataModelField.Name()
}

func (f *ExpressionField[T]) SQL(d driver.Driver, m attrs.Definer, quote string) (string, []any) {
	if f.expr == nil {
		return "", nil
	}
	var expr = f.expr.With(d, m, quote)
	var sb strings.Builder
	expr.SQL(&sb)
	return sb.String(), expr.Args()
}
