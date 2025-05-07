package queries

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

var _ VirtualField = &queryField[any]{}

type queryField[T any] struct {
	name  string
	expr  expr.Expression
	value T
}

func newQueryField[T any](name string, expr expr.Expression) *queryField[T] {
	return &queryField[T]{name: name, expr: expr}
}

// VirtualField

func (q *queryField[T]) Alias() string { return q.name }
func (q *queryField[T]) SQL(d driver.Driver, m attrs.Definer, quote string) (string, []any) {
	var sqlBuilder = &strings.Builder{}
	var expr = q.expr.With(d, m, quote)
	expr.SQL(sqlBuilder)
	return sqlBuilder.String(), expr.Args()
}

// attrs.Field minimal impl
func (q *queryField[T]) Name() string          { return q.name }
func (q *queryField[T]) ColumnName() string    { return "" }
func (q *queryField[T]) Tag(string) string     { return "" }
func (q *queryField[T]) Type() reflect.Type    { return reflect.TypeOf(*new(T)) }
func (q *queryField[T]) Attrs() map[string]any { return map[string]any{} }
func (q *queryField[T]) IsPrimary() bool       { return false }
func (q *queryField[T]) AllowNull() bool       { return true }
func (q *queryField[T]) AllowBlank() bool      { return true }
func (q *queryField[T]) AllowEdit() bool       { return false }
func (q *queryField[T]) GetValue() any         { return q.value }
func (q *queryField[T]) SetValue(v any, _ bool) error {
	val, ok := v.(T)
	if !ok {
		return fmt.Errorf("type mismatch on queryField[%T]: %v", *new(T), v)
	}
	q.value = val
	return nil
}
func (q *queryField[T]) Value() (driver.Value, error) { return q.value, nil }
func (q *queryField[T]) Scan(v any) error             { return q.SetValue(v, false) }
func (q *queryField[T]) GetDefault() any              { return nil }
func (q *queryField[T]) Instance() attrs.Definer      { return nil }
func (q *queryField[T]) Rel() attrs.Relation          { return nil }
func (q *queryField[T]) FormField() fields.Field      { return nil }
func (q *queryField[T]) Validate() error              { return nil }
func (q *queryField[T]) Label() string                { return q.name }
func (q *queryField[T]) ToString() string             { return fmt.Sprint(q.value) }
func (q *queryField[T]) HelpText() string             { return "" }
