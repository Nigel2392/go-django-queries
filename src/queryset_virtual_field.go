package queries

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
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
	HasQueryValue(key string) bool
	GetQueryValue(key string) (any, bool)
	SetQueryValue(key string, value any) error
}

type BaseModel struct {
	data  map[string]interface{}
	_defs attrs.Definitions
}

func (m *BaseModel) Define(def attrs.Definer, definitions attrs.Definitions) attrs.Definitions {
	if m._defs == nil {
		if definitions == nil {
			definitions = def.FieldDefs()
		}
		m._defs = definitions
	}
	return m._defs
}

func (m *BaseModel) HasQueryValue(key string) bool {
	return m.data != nil && m.data[key] != nil
}

func (m *BaseModel) GetQueryValue(key string) (any, bool) {
	if m.data == nil {
		return nil, false
	}
	var val, ok = m.data[key]
	return val, ok
}

func (m *BaseModel) SetQueryValue(key string, value any) error {
	if m.data == nil {
		m.data = make(map[string]interface{})
	}
	m.data[key] = value
	return nil
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

	// resultType is the type of the result of the expression
	resultType reflect.Type
}

func NewVirtualField[T any](forModel attrs.Definer, dst any, name string, expr Expression) *ExpressionField[T] {

	if forModel == nil || dst == nil {
		panic("NewVirtualField: model is nil")
	}
	if name == "" {
		panic("NewVirtualField: name is empty")
	}
	if expr == nil {
		panic("NewVirtualField: expression is nil")
	}

	var (
		dataModel  DataModel
		resultType = reflect.TypeOf(*new(T))
	)

	if m, ok := dst.(DataModel); ok {
		dataModel = m
		goto retField
	}

retField:
	var f = &ExpressionField[T]{
		model:      forModel,
		dataModel:  dataModel,
		resultType: resultType,
		name:       name,
		expr:       expr,
	}

	return f
}

func (f *ExpressionField[T]) getQueryValue() (any, bool) {
	return f.dataModel.GetQueryValue(f.name)
}

func (f *ExpressionField[T]) setQueryValue(v any) error {
	return f.dataModel.SetQueryValue(f.name, v)
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

	var val, ok = e.getQueryValue()
	if !ok || val == nil {
		return *new(T)
	}

	valTyped, ok := val.(T)
	if !ok {
		return *new(T)
	}

	return valTyped
}

func castToNumber[T any](s string) (any, error) {
	var n, err = attrs.CastToNumber[T](s)
	return n, err
}

var reflect_convert = map[reflect.Kind]func(string) (any, error){
	reflect.Int:     castToNumber[int],
	reflect.Int8:    castToNumber[int8],
	reflect.Int16:   castToNumber[int16],
	reflect.Int32:   castToNumber[int32],
	reflect.Int64:   castToNumber[int64],
	reflect.Uint:    castToNumber[uint],
	reflect.Uint8:   castToNumber[uint8],
	reflect.Uint16:  castToNumber[uint16],
	reflect.Uint32:  castToNumber[uint32],
	reflect.Uint64:  castToNumber[uint64],
	reflect.Float32: castToNumber[float32],
	reflect.Float64: castToNumber[float64],
	reflect.String: func(s string) (any, error) {
		return s, nil
	},
	reflect.Bool: func(s string) (any, error) {
		var b, err = strconv.ParseBool(s)
		return b, err
	},
}

func (e *ExpressionField[T]) SetValue(v interface{}, _ bool) error {
	if e.dataModel == nil {
		panic("model is nil")
	}

	var (
		rV = reflect.ValueOf(v)
		rT = reflect.TypeOf(v)
	)

	if rT != e.resultType {

		if rT.ConvertibleTo(e.resultType) {
			rV = rV.Convert(e.resultType)
		} else if rV.IsValid() && rT.Kind() == reflect.Ptr && (rT.Elem() == e.resultType || rT.Elem().ConvertibleTo(e.resultType)) {
			rV = rV.Elem()
			if rT.Elem() != e.resultType {
				rV = rV.Convert(e.resultType)
			}
		}

		if rT.Kind() == reflect.String && rT.Kind() != e.resultType.Kind() {

			if f, ok := reflect_convert[e.resultType.Kind()]; ok {
				var val, err = f(rV.String())
				if err != nil {
					return fmt.Errorf("cannot convert %v to %T: %w", v, *new(T), err)
				}
				rV = reflect.ValueOf(val)
			} else {
				return fmt.Errorf("cannot convert %v to %T", v, *new(T))
			}

		}
	}

	v = rV.Interface()

	if _, ok := v.(T); ok {
		e.setQueryValue(v)
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
