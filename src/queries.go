package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/pkg/errors"
)

func GetObject[T attrs.Definer](identifier any) (T, error) {
	var obj = newDefiner[T]()
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
			ErrFieldNull,
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

	return d.(T), nil
}

func CT_GetObject[T attrs.Definer](identifier any) (interface{}, error) {
	var obj, err = GetObject[T](identifier)
	return obj, err
}
func CT_ListObjects[T attrs.Definer](amount, offset uint) ([]interface{}, error) {
	var results, err = ListObjects[T](uint64(offset), uint64(amount))
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}
func CT_ListObjectsByIDs[T attrs.Definer](i []interface{}) ([]interface{}, error) {
	var results, err = ListObjectsByIDs[T](0, 1000, i)
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}

func ListObjectsByIDs[T attrs.Definer, T2 any](offset, limit uint64, ids []T2) ([]T, error) {

	var (
		obj          = newDefiner[T]()
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
		results = append(results, obj.(T))
	}

	return results, nil
}

func ListObjects[T attrs.Definer](offset, limit uint64, ordering ...string) ([]T, error) {
	var obj = newDefiner[T]()
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
		results = append(results, obj.(T))
	}

	return results, nil
}

func CountObjects[T attrs.Definer](obj T) (int64, error) {
	return Objects(obj).Count().Exec()
}

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

func CreateObject[T attrs.Definer](obj T) error {
	var (
		definitions = obj.FieldDefs()
	)

	var d, err = Objects(obj).Create(obj).Exec()
	if err != nil {
		return err
	}

	var retDefs = d.FieldDefs()
	for _, field := range definitions.Fields() {
		var f, ok = retDefs.Field(field.Name())
		if !ok {
			return errors.Errorf("field %q not found in %T", field.Name(), obj)
		}

		var value = f.GetDefault()
		if value == nil && !field.AllowNull() {
			return errors.Wrapf(
				ErrFieldNull,
				"Field %q cannot be null",
				field.Name(),
			)
		}

		if err = field.SetValue(value, true); err != nil {
			return err
		}
	}

	return nil
}

func UpdateObject[T attrs.Definer](obj T) (int64, error) {
	var (
		definitions = obj.FieldDefs()
		primary     = definitions.Primary()
	)

	var primaryVal, err = primary.Value()
	if err != nil {
		return 0, err
	}

	return Objects(obj).
		Filter(primary.Name(), primaryVal).
		Update(obj).Exec()
}

func DeleteObject[T attrs.Definer](obj T) (int64, error) {

	var (
		definitions = obj.FieldDefs()
		primary     = definitions.Primary()
	)

	var primaryVal, err = primary.Value()
	if err != nil {
		return 0, err
	}

	return Objects(obj).
		Filter(primary.Name(), primaryVal).
		Delete().Exec()
}
