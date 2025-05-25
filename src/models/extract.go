package models

import (
	"reflect"

	"github.com/Nigel2392/go-django/src/core/errs"
)

const (
	ErrObjectPtr       errs.Error = "object must be a pointer to a struct"
	ErrObjectInvalid   errs.Error = "object is invalid or nil, the object must be a valid pointer to a struct"
	ErrModelEmbedded   errs.Error = "object does not have an embedded Model field, the object must embed the Model struct"
	ErrModelAdressable errs.Error = "the Model is not addressable, the object must embed the Model struct"
	ErrModelType       errs.Error = "object has a non- Model type embedded as the Model field, the object must embed the models.Model struct"
)

func ExtractModel(def any) (*Model, error) {
	var (
		rVal = reflect.ValueOf(def)
		rTyp = rVal.Type()
	)

	// attrs.Definer must always be a pointer to a struct
	if rTyp.Kind() != reflect.Ptr {
		return nil, ErrObjectPtr
	}

	if !rVal.IsValid() || rVal.IsNil() {
		return nil, ErrObjectInvalid
	}

	var modelField, ok = rTyp.Elem().FieldByName("Model")
	if !ok {
		return nil, ErrModelEmbedded
	}

	// check if the model field is embedded
	if !modelField.Anonymous {
		return nil, ErrModelEmbedded
	}

	// retrieve the model field by its index chain
	var modelValue = rVal.Elem().FieldByIndex(modelField.Index)
	if modelValue.Kind() != reflect.Struct {
		return nil, ErrModelEmbedded
	}

	// check if the model is addressable
	if !modelValue.CanAddr() {
		return nil, ErrModelAdressable
	}

	// return the model POINTER.
	var modelPtr = modelValue.Addr().Interface()
	m, ok := modelPtr.(*Model)
	if !ok {
		return nil, ErrModelType
	}

	if m.internals.selfField == nil {
		m.internals.selfField = &modelField
	}

	if m.internals.selfValue == nil {
		m.internals.selfValue = &modelValue
	}

	return m, nil
}
