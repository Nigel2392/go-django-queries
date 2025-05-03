package queries

import (
	"context"
	"fmt"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/Nigel2392/go-signals"
	"github.com/pkg/errors"
)

// CT_GetObject retrieves an object from the database by its identifier.
//
// This is a function with the CT_ prefix to indicate that it is a function to be used in a `contenttypes.ContentTypeDefinition` context.
func CT_GetObject[T attrs.Definer](identifier any) (interface{}, error) {
	var obj, err = GetObject[T](identifier)
	return obj, err
}

// CT_ListObjects lists objects from the database.
//
// This is a function with the CT_ prefix to indicate that it is a function to be used in a `contenttypes.ContentTypeDefinition` context.
func CT_ListObjects[T attrs.Definer](amount, offset uint) ([]interface{}, error) {
	var results, err = ListObjects[T](uint64(offset), uint64(amount))
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}

// CT_ListObjectsByIDs lists objects from the database by their IDs.
//
// This is a function with the CT_ prefix to indicate that it is a function to be used in a `contenttypes.ContentTypeDefinition` context.
func CT_ListObjectsByIDs[T attrs.Definer](i []interface{}) ([]interface{}, error) {
	var results, err = ListObjectsByIDs[T](0, 1000, i)
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}

// ListObjectsByIDs lists objects from the database by their IDs.
//
// It takes an offset, limit, and a slice of IDs as parameters and returns a slice of objects of type T.
func ListObjectsByIDs[T attrs.Definer, T2 any](offset, limit uint64, ids []T2) ([]T, error) {

	var (
		obj          = internal.NewDefiner[T]()
		definitions  = obj.FieldDefs()
		primaryField = definitions.Primary()
	)

	var d, err = Objects(obj).
		Filter(
			fmt.Sprintf("%s__in", primaryField.Name()),
			attrs.InterfaceList(ids)...,
		).
		Limit(int(limit)).
		Offset(int(offset)).
		All().Exec()

	if err != nil {
		return nil, err
	}

	var results = make([]T, 0, len(ids))
	for _, obj := range d {
		results = append(results, obj.Object.(T))
	}

	return results, nil
}

// ListObjects lists objects from the database.
//
// It takes an offset and a limit as parameters and returns a slice of objects of type T.
func ListObjects[T attrs.Definer](offset, limit uint64, ordering ...string) ([]T, error) {
	var obj = internal.NewDefiner[T]()
	var d, err = Objects(obj).
		OrderBy(ordering...).
		Limit(int(limit)).
		Offset(int(offset)).
		All().Exec()

	if err != nil {
		return nil, err
	}

	var results = make([]T, 0, len(d))
	for _, obj := range d {
		results = append(results, obj.Object.(T))
	}

	return results, nil
}

// GetObject retrieves an object from the database by its identifier.
//
// It takes an identifier as a parameter and returns the object of type T.
//
// The identifier can be any type, but it is expected to be the primary key of the object.
func GetObject[T attrs.Definer](identifier any) (T, error) {
	var obj = internal.NewDefiner[T]()
	var (
		defs         = obj.FieldDefs()
		primaryField = defs.Primary()
	)

	if err := primaryField.SetValue(identifier, true); err != nil {
		return obj, err
	}

	primaryValue, err := primaryField.Value()
	if err != nil {
		return obj, err
	}

	if fields.IsZero(primaryValue) {
		return obj, errors.Wrapf(
			query_errors.ErrFieldNull,
			"Primary field %q cannot be null",
			primaryField.Name(),
		)
	}

	d, err := Objects(obj).
		Filter(
			fmt.Sprintf("%s__exact", primaryField.Name()),
			primaryValue,
		).
		Get().Exec()

	if err != nil {
		return obj, err
	}

	return d.Object.(T), nil
}

// CountObjects counts the number of objects in the database.
func CountObjects[T attrs.Definer](obj T) (int64, error) {
	return Objects(obj).Count().Exec()
}

// SaveObject saves an object to the database.
//
// It checks if the primary key is set. If it is not set, it creates a new object. If it is set, it updates the existing object.
//
// It sends a pre-save signal before saving and a post-save signal after saving.
//
// If the object implements the models.Saver interface, it will call the Save method instead of executing a query.
func SaveObject[T attrs.Definer](obj T) error {
	var fieldDefs = obj.FieldDefs()
	var primaryField = fieldDefs.Primary()
	var primaryValue, err = primaryField.Value()
	if err != nil {
		return err
	}
	if fields.IsZero(primaryValue) {
		return CreateObject(obj)
	}
	_, err = UpdateObject(obj)
	return err
}

func sendSignal(s signals.Signal[SignalSave], obj attrs.Definer, q QueryCompiler) error {
	return s.Send(SignalSave{
		Instance: obj,
		Using:    q,
	})
}

// CreateObject creates a new object in the database and sets its default values.
//
// It sends a pre-save signal before saving and a post-save signal after saving.
//
// If the object implements the models.Saver interface, it will call the Save method instead of executing a query.
func CreateObject[T attrs.Definer](obj T) error {
	var (
		definitions = obj.FieldDefs()
	)

	var (
		qs       = Objects(obj).ExplicitSave().Create(obj)
		compiler = qs.Compiler()
	)

	// Send pre model save signal
	if err := sendSignal(SignalPreModelSave, obj, compiler); err != nil {
		return err
	}

	var (
		d       attrs.Definer
		retDefs attrs.Definitions
		err     error
	)

	if saver, ok := any(obj).(models.Saver); ok {
		err = saver.Save(context.Background())
		if err != nil {
			return err
		}
		goto postSaveSignal
	} else {
		d, err = qs.Exec()
		if err != nil {
			return err
		}
	}

	retDefs = d.FieldDefs()
	for _, field := range definitions.Fields() {
		var f, ok = retDefs.Field(field.Name())
		if !ok {
			return errors.Errorf("field %q not found in %T", field.Name(), obj)
		}

		var value = f.GetValue()
		if value == nil && !field.AllowNull() {
			return errors.Wrapf(
				query_errors.ErrFieldNull,
				"Field %q cannot be null",
				field.Name(),
			)
		}

		if err = field.SetValue(value, true); err != nil {
			return errors.Wrapf(
				err, "failed to set value %v to field %q",
				value, field.Name(),
			)
		}
	}

postSaveSignal:
	// Send post model save signal
	return sendSignal(SignalPostModelSave, obj, compiler)
}

// UpdateObject updates an existing object in the database.
//
// It sends a pre-save signal before saving and a post-save signal after saving.
//
// If the object implements the models.Saver interface, it will call the Save method instead of executing a query.
func UpdateObject[T attrs.Definer](obj T) (int64, error) {
	var (
		definitions = obj.FieldDefs()
		primary     = definitions.Primary()
	)

	var primaryVal, err = primary.Value()
	if err != nil {
		return 0, err
	}

	var (
		qs = Objects(obj).
			Filter(primary.Name(), primaryVal).
			Update(obj)
		compiler = qs.Compiler()
	)

	// Send pre model save signal
	if err := sendSignal(SignalPreModelSave, obj, compiler); err != nil {
		return 0, err
	}

	var d int64
	if saver, ok := any(obj).(models.Saver); ok {
		err = saver.Save(context.Background())
		d = 1
	} else {
		d, err = qs.Exec()
	}
	if err != nil {
		return 0, err
	}

	if err := sendSignal(SignalPostModelSave, obj, compiler); err != nil {
		return 0, err
	}

	return d, nil
}

// DeleteObject deletes an object from the database.
//
// It sends a pre-delete signal before deleting and a post-delete signal after deleting.
//
// If the object implements the models.Deleter interface, it will call the Delete method instead of executing a query.
func DeleteObject[T attrs.Definer](obj T) (int64, error) {
	var (
		definitions = obj.FieldDefs()
		primary     = definitions.Primary()
	)

	var primaryVal, err = primary.Value()
	if err != nil {
		return 0, err
	}

	if err := SignalPreModelDelete.Send(obj); err != nil {
		return 0, err
	}

	var d int64
	if deleter, ok := any(obj).(models.Deleter); ok {
		err = deleter.Delete(context.Background())
		d = 1
	} else {
		d, err = Objects(obj).
			Filter(primary.Name(), primaryVal).
			Delete().Exec()
	}
	if err != nil {
		return 0, err
	}

	if err := SignalPostModelDelete.Send(obj); err != nil {
		return d, err
	}

	return d, nil
}
