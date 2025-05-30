package fields

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

var _ attrs.Field = &DataModelField[any]{}

type DataModelField[T any] struct {
	// model is the model that this field belongs to
	Model attrs.Definer

	// dataModel is the model that contains the data for this field
	//
	// it should be embedded in the attrs.Definer type which this virtual field is for
	DataModel any

	// name is the name of the field's map key in the model
	// it is also the alias used in the query
	name string

	// resultType is the type of the result of the expression
	resultType reflect.Type

	// fieldRef is the back reference in case this field is embedded in another
	// field type
	fieldRef attrs.Field
}

func indirect(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return indirect(t.Elem())
	}
	return t
}

func NewDataModelField[T any](forModel attrs.Definer, dst any, name string, ref ...attrs.Field) *DataModelField[T] {
	if forModel == nil || dst == nil {
		panic("NewDataModelField: model is nil")
	}

	if name == "" {
		panic("NewDataModelField: name is empty")
	}

	var (
		Type = reflect.TypeOf((*T)(nil)).Elem()

		dstT = reflect.TypeOf(dst)
		dstV = reflect.ValueOf(dst)
	)

	if _, ok := dst.(queries.ModelDataStore); ok {
		goto addField
	}

	if dstT.Kind() == reflect.Pointer && dstT.Elem().Kind() == reflect.Struct {
		var field, ok = dstT.Elem().FieldByName(name)
		if ok {
			if !field.Type.AssignableTo(Type) && !field.Type.ConvertibleTo(Type) {
				panic(fmt.Errorf("NewDataModelField: %s != %s (%T.%s)", indirect(Type).Name(), indirect(field.Type).Name(), forModel, name))
			}

			var fieldVal = dstV.Elem().FieldByName(name)
			if !fieldVal.IsValid() {
				if !fieldVal.CanSet() {
					panic(fmt.Errorf("NewDataModelField: field %T.%s is not settable", forModel, name))
				}
				if fieldVal.CanAddr() {
					fieldVal.Addr().Set(reflect.New(Type.Elem()))
				} else {
					fieldVal.Set(reflect.New(Type.Elem()).Elem())
				}
			}

			if fieldVal.CanAddr() {
				dst = fieldVal.Addr().Interface()
			} else {
				dst = fieldVal.Interface()
			}

			goto addField
		}
	}

	if _, ok := dst.(queries.DataModel); !ok {

		if dstT.Kind() != reflect.Ptr {
			panic("NewDataModelField: resultType is not a pointer")
		}

		if !dstV.IsValid() {
			panic(fmt.Errorf("NewDataModelField: resultType is nil: %T", dst))
		}

		if Type.Kind() != reflect.Interface {
			if dstT.Elem() != Type {
				panic(fmt.Errorf("NewDataModelField: resultType \"%s\" is not a pointer to T: %T (%T.%s)", indirect(Type).Name(), dst, forModel, name))
			}
		} else {
			if !dstT.Elem().Implements(Type) {
				panic(fmt.Errorf("NewDataModelField: resultType \"%s\" does not implement T: %T (%T.%s)", indirect(Type).Name(), dst, forModel, name))
			}
		}

		if dstV.Elem().Kind() == reflect.Ptr && dstV.Elem().IsNil() {
			dstV.Elem().Set(reflect.New(Type.Elem()))
		}
	}

addField:
	var fRef attrs.Field
	if len(ref) > 0 {
		fRef = ref[0]
	}

	if len(ref) > 0 && fRef == nil {
		panic(fmt.Errorf("NewDataModelField: fieldRef is nil for %T.%s", forModel, name))
	}

	var f = &DataModelField[T]{
		Model:      forModel,
		DataModel:  dst,
		resultType: Type,
		name:       name,
		fieldRef:   fRef,
	}

	return f
}

func (f *DataModelField[T]) getQueryValue() (any, bool) {
	switch m := f.DataModel.(type) {
	case queries.ModelDataStore:
		return m.GetValue(f.name)
	case queries.DataModel:
		return m.ModelDataStore().GetValue(f.name)
	}

	// rVal is always a pointer.
	// If T is a pointer, it is a pointer to a pointer.
	var rVal = reflect.ValueOf(f.DataModel)
	if !rVal.IsValid() {
		return nil, false
	}

	rVal = rVal.Elem()

	if !rVal.IsValid() || !rVal.CanInterface() {
		return nil, false
	}

	return rVal.Interface(), true
}

func (f *DataModelField[T]) setQueryValue(v any) error {
	switch m := f.DataModel.(type) {
	case queries.ModelDataStore:
		return m.SetValue(f.name, v)
	case queries.DataModel:
		return m.ModelDataStore().SetValue(f.name, v)
	}

	var rVal = reflect.ValueOf(f.DataModel)
	if !rVal.IsValid() {
		return fmt.Errorf("setQueryValue: dst value is nil")
	}

	if !rVal.Elem().CanSet() {
		return fmt.Errorf("setQueryValue: dst value is not settable")
	}

	var vVal = reflect.ValueOf(v)
	rVal.Elem().Set(vVal)
	return nil
}

func (f *DataModelField[T]) Name() string {
	return f.name
}

// no real column, special case for virtual fields
func (e *DataModelField[T]) ColumnName() string {
	return ""
}

func (e *DataModelField[T]) Tag(string) string {
	return ""
}

func (e *DataModelField[T]) Type() reflect.Type {
	if e.resultType == nil {
		panic("resultType is nil")
	}

	return e.resultType
}

func (e *DataModelField[T]) Attrs() map[string]any {
	return map[string]any{}
}

func (e *DataModelField[T]) IsPrimary() bool {
	return false
}

func (e *DataModelField[T]) AllowNull() bool {
	return true
}

func (e *DataModelField[T]) AllowBlank() bool {
	return true
}

func (e *DataModelField[T]) AllowEdit() bool {
	return false
}

func (e *DataModelField[T]) AnnotateValue(v any) error {
	return e.SetValue(v, false)
}

func (e *DataModelField[T]) GetValue() interface{} {
	if e.DataModel == nil {
		panic("model is nil")
	}

	var val, _ = e.getQueryValue()
	if e.resultType.Kind() == reflect.Pointer && (e.resultType.Comparable() && any(val) == any(*new(T)) || val == nil) {
		val = reflect.New(e.resultType.Elem()).Interface()
		assert.Err(e.setQueryValue(val))
	}

	if val != nil {
		switch {
		case e.fieldRef != nil:
			assert.Err(attrs.BindValueToModel(
				e.Model, e.fieldRef, val,
			))
		default:
			assert.Err(attrs.BindValueToModel(
				e.Model, e, val,
			))
		}
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

var baseReflectKinds = (func() []reflect.Kind {
	var kinds = make([]reflect.Kind, 0, len(reflect_convert))
	for k := range reflect_convert {
		kinds = append(kinds, k)
	}
	return kinds
})()

func (e *DataModelField[T]) SetValue(v interface{}, _ bool) error {
	if e.DataModel == nil {
		panic("model is nil")
	}

	var (
		rV = reflect.ValueOf(v)
		rT = reflect.TypeOf(v)
	)

	if !rV.IsValid() || rT == nil {
		rV = reflect.New(e.resultType).Elem()
		rT = rV.Type()
	}

	if e.fieldRef != nil {
		assert.Err(attrs.BindValueToModel(
			e.Model, e.fieldRef, rV,
		))
	} else {
		assert.Err(attrs.BindValueToModel(
			e.Model, e, rV,
		))
	}

	if rT != e.resultType {

		if rT.ConvertibleTo(e.resultType) {
			rV = rV.Convert(e.resultType)
		} else if rV.IsValid() && rT.Kind() == reflect.Ptr && (rT.Elem() == e.resultType || rT.Elem().ConvertibleTo(e.resultType)) {
			rV = rV.Elem()
			if rT.Elem() != e.resultType {
				rV = rV.Convert(e.resultType)
			}
		}

		if slices.Contains(baseReflectKinds, rT.Kind()) {

			if f, ok := reflect_convert[e.resultType.Kind()]; ok {
				var val, err = f(rV.String())
				if err != nil {
					return fmt.Errorf("cannot convert %v to %T: %w", v, *new(T), err)
				}

				rV = reflect.ValueOf(val)

				if rV.Type() != e.resultType {
					rV = rV.Convert(e.resultType)
				}
			} else {
				return fmt.Errorf("cannot convert %v to %T", v, *new(T))
			}

		}
	}

	v = rV.Interface()
	if v == nil {
		e.setQueryValue(
			reflect.New(e.resultType).Interface(),
		)
		return nil
	}

	if _, ok := v.(T); ok {
		e.setQueryValue(v)
		return nil
	}

	var typName string = e.resultType.Name()
	if typName == "" {
		typName = fmt.Sprintf("%T", *(new(T)))
	} else {
		typName = e.resultType.Name()
	}

	return fmt.Errorf("value %v (%T) is not of type %s", v, v, typName)
}

func (e *DataModelField[T]) Value() (driver.Value, error) {
	var val = e.GetValue()
	if val == nil {
		return *new(T), nil
	}

	return val, nil
}

func (e *DataModelField[T]) Scan(src interface{}) error {
	return e.SetValue(src, false)
}

func (e *DataModelField[T]) GetDefault() interface{} {
	return nil
}

func (e *DataModelField[T]) Instance() attrs.Definer {
	if e.Model == nil {
		panic("model is nil")
	}
	if def, ok := e.Model.(attrs.Definer); ok {
		return def
	}
	panic(fmt.Errorf("model %T does not implement attrs.Definer", e.Model))
}

func (e *DataModelField[T]) Rel() attrs.Relation {
	return nil
}

func (e *DataModelField[T]) FormField() fields.Field {
	return nil
}

func (e *DataModelField[T]) Validate() error {
	return nil
}

func (e *DataModelField[T]) Label() string {
	return e.name
}

func (e *DataModelField[T]) ToString() string {
	return fmt.Sprint(e.GetValue())
}

func (e *DataModelField[T]) HelpText() string {
	return ""
}
