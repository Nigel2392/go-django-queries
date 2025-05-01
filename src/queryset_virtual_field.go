package queries

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

var _ VirtualField = &ExpressionField[int]{}

type VirtualField interface {
	attrs.Field
	Alias() string
	SQL(d driver.Driver, m attrs.Definer, quote string) (string, []any)
}

type DataModel interface {
	Has(key string) bool
	Get(key string) (any, bool)
	Set(key string, value any) error
}

type BaseModel struct {
	data map[string]interface{}
}

func (m *BaseModel) Has(key string) bool {
	return m.data != nil && m.data[key] != nil
}

func (m *BaseModel) Get(key string) (any, bool) {
	if m.data == nil {
		return nil, false
	}
	var val, ok = m.data[key]
	return val, ok
}

func (m *BaseModel) Set(key string, value any) error {
	if m.data == nil {
		m.data = make(map[string]interface{})
	}
	m.data[key] = value
	return nil
}

func NewVirtualField[T any](forModel attrs.Definer, dataModel DataModel, name string, expr Expression) *ExpressionField[T] {

	if forModel == nil || dataModel == nil {
		panic("NewVirtualField: model is nil")
	}
	if name == "" {
		panic("NewVirtualField: name is empty")
	}
	if expr == nil {
		panic("NewVirtualField: expression is nil")
	}

	var f = &ExpressionField[T]{
		model:     forModel,
		dataModel: dataModel,
		name:      name,
		expr:      expr,
	}

	return f
}

type ExpressionField[T any] struct {
	// model is the model that this field belongs to
	model attrs.Definer

	// dataModel is the model that contains the data for this field
	//
	// it should be embedded in the attrs.Definer type which this virtual field is for
	dataModel DataModel

	// name is the name of the field's map key in the model
	// it is also the alias used in the query
	name string

	// expr is the expression used to calculate the field's value
	expr Expression
}

func (f *ExpressionField[T]) Name() string {
	return f.name
}

func (f *ExpressionField[T]) Alias() string {
	return f.name
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

// no real column, special case for virtual fields
func (e *ExpressionField[T]) ColumnName() string {
	return ""
}

func (e *ExpressionField[T]) Tag(string) string {
	return ""
}

func (e *ExpressionField[T]) Type() reflect.Type {
	var rT = reflect.TypeOf(*new(T))
	if rT.Kind() == reflect.Ptr {
		return rT.Elem()
	}
	return rT
}

func (e *ExpressionField[T]) Attrs() map[string]any {
	return nil
}

func (e *ExpressionField[T]) IsPrimary() bool {
	return false
}

func (e *ExpressionField[T]) AllowNull() bool {
	return true
}

func (e *ExpressionField[T]) AllowBlank() bool {
	return true
}

func (e *ExpressionField[T]) AllowEdit() bool {
	return false
}

func (e *ExpressionField[T]) GetValue() interface{} {
	if e.dataModel == nil {
		panic("model is nil")
	}

	var val, ok = e.dataModel.Get(e.name)
	if !ok || val == nil {
		return *new(T)
	}

	valTyped, ok := val.(T)
	if !ok {
		return *new(T)
	}

	return valTyped
}

func (e *ExpressionField[T]) SetValue(v interface{}, _ bool) error {
	if e.dataModel == nil {
		panic("model is nil")
	}

	var (
		rV = reflect.ValueOf(v)
		rT = reflect.TypeOf(v)

		resT = reflect.TypeOf(*new(T))
	)

	if rT != resT {
		if rT.ConvertibleTo(resT) {
			rV = rV.Convert(resT)
		} else if rV.IsValid() && rT.Kind() == reflect.Ptr && (rT.Elem() == resT || rT.Elem().ConvertibleTo(resT)) {
			rV = rV.Elem()
			if rT.Elem() != resT {
				rV = rV.Convert(resT)
			}
		} else {
			return fmt.Errorf("value %v is not of type %T", v, *new(T))
		}
	}

	v = rV.Interface()

	if _, ok := v.(T); ok {
		e.dataModel.Set(e.name, v)
		return nil
	}

	return fmt.Errorf("value %v is not of type %T", v, *new(T))
}
func (e *ExpressionField[T]) Value() (driver.Value, error) {
	var val = e.GetValue()
	if val == nil {
		return *new(T), nil
	}

	return val, nil
}

func (e *ExpressionField[T]) Scan(src interface{}) error {
	return e.SetValue(src, false)
}

func (e *ExpressionField[T]) GetDefault() interface{} {
	return nil
}

func (e *ExpressionField[T]) Instance() attrs.Definer {
	return e.model
}

func (e *ExpressionField[T]) Rel() attrs.Definer {
	return nil
}

func (e *ExpressionField[T]) ForeignKey() attrs.Definer {
	return nil
}

func (e *ExpressionField[T]) ManyToMany() attrs.Relation {
	return nil
}

func (e *ExpressionField[T]) OneToOne() attrs.Relation {
	return nil
}

func (e *ExpressionField[T]) FormField() fields.Field {
	return nil
}

func (e *ExpressionField[T]) Validate() error {
	return nil
}

func (e *ExpressionField[T]) Label() string {
	return e.name
}

func (e *ExpressionField[T]) ToString() string {
	return fmt.Sprint(e.GetValue())
}

func (e *ExpressionField[T]) HelpText() string {
	return ""
}

type queryField[T any] struct {
	name  string
	expr  Expression
	value T
}

func QuerySum(name, field string) attrs.Field {
	return newQueryField[float64](name, &RawExpr{
		Statement: "SUM(%s)",
		Fields:    []string{field},
	})
}

func QueryCount(name, field string) attrs.Field {
	return newQueryField[int64](name, &RawExpr{
		Statement: "COUNT(%s)",
		Fields:    []string{field},
	})
}

func newQueryField[T any](name string, expr Expression) *queryField[T] {
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
func (q *queryField[T]) Attrs() map[string]any { return nil }
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
func (q *queryField[T]) Rel() attrs.Definer           { return nil }
func (q *queryField[T]) ForeignKey() attrs.Definer    { return nil }
func (q *queryField[T]) ManyToMany() attrs.Relation   { return nil }
func (q *queryField[T]) OneToOne() attrs.Relation     { return nil }
func (q *queryField[T]) FormField() fields.Field      { return nil }
func (q *queryField[T]) Validate() error              { return nil }
func (q *queryField[T]) Label() string                { return q.name }
func (q *queryField[T]) ToString() string             { return fmt.Sprint(q.value) }
func (q *queryField[T]) HelpText() string             { return "" }
