package models

import (
	"context"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/elliotchance/orderedmap/v2"
)

type NewFieldFunc func(queries.ModelDataStore, attrs.Definer, attrs.Definitions) attrs.Field

type fakeModel struct {
	cnf    *FakeModelObject
	fields *orderedmap.OrderedMap[string, attrs.Field]
	defs   *attrs.ObjectDefinitions
}

type FakeModelObject struct {
	TableName  string
	Data       queries.ModelDataStore
	FieldFuncs []NewFieldFunc
}

func FakeModel(model *FakeModelObject) *fakeModel {
	if model == nil {
		panic("FakeModelObject cannot be nil")
	}

	if model.Data == nil {
		model.Data = make(MapDataStore)
	}

	if len(model.FieldFuncs) == 0 {
		panic("No field functions provided for FakeModel")
	}

	var m = &fakeModel{
		cnf: &FakeModelObject{
			TableName:  model.TableName,
			Data:       model.Data,
			FieldFuncs: model.FieldFuncs,
		},
		fields: orderedmap.NewOrderedMap[string, attrs.Field](),
	}

	if len(model.FieldFuncs) > 0 {
		for _, f := range model.FieldFuncs {
			var field = f(model.Data, m, nil)
			if field == nil {
				continue
			}
			m.fields.Set(field.Name(), field)
		}
	}

	return m
}

func (m *fakeModel) FieldDefs() attrs.Definitions {
	if m.defs != nil {
		return m.defs
	}

	if m.fields == nil || (m.fields.Len() == 0 && len(m.cnf.FieldFuncs) > 0) {
		m.fields = orderedmap.NewOrderedMap[string, attrs.Field]()
		for _, f := range m.cnf.FieldFuncs {
			var field = f(m.cnf.Data, m, nil)
			if field == nil {
				continue
			}
			m.fields.Set(field.Name(), field)
		}
	}

	if m.fields != nil && m.fields.Len() > 0 {
		var values = make([]attrs.Field, 0, m.fields.Len())
		for head := m.fields.Front(); head != nil; head = head.Next() {
			values = append(values, head.Value)
		}

		m.defs = attrs.Define(m, values...).WithTableName(
			m.cnf.TableName,
		)
	}

	return m.defs
}

func (m *fakeModel) InitNew() attrs.Definer {
	if m == nil {
		panic("fakeModel cannot be nil when initializing a new instance")
	}
	if m.cnf == nil {
		panic("FakeModelObject cannot be nil when initializing a new instance")
	}
	if m.cnf.Data == nil {
		m.cnf.Data = make(MapDataStore)
	}
	var newModel = &fakeModel{
		cnf: &FakeModelObject{
			TableName:  m.cnf.TableName,
			Data:       m.cnf.Data,
			FieldFuncs: m.cnf.FieldFuncs,
		},
		fields: orderedmap.NewOrderedMap[string, attrs.Field](),
		defs:   nil,
	}
	return newModel
}

func (m *fakeModel) Clone() attrs.Definer {
	var clone = &fakeModel{
		cnf: &FakeModelObject{
			TableName:  m.cnf.TableName,
			Data:       m.cnf.Data,
			FieldFuncs: m.cnf.FieldFuncs,
		},
		fields: orderedmap.NewOrderedMap[string, attrs.Field](),
	}

	for _, f := range m.cnf.FieldFuncs {
		var field = f(m.cnf.Data, clone, m.defs)
		clone.fields.Set(field.Name(), field)
	}

	return clone
}

func (m *fakeModel) Save(ctx context.Context) error {
	var primary = m.FieldDefs().Primary()
	if primary == nil {
		return query_errors.ErrNilPointer
	}

	var val = primary.GetValue()
	if val == nil || fields.IsZero(val) {
		return m.create(ctx)
	}
	return m.update(ctx)
}

func (m *fakeModel) create(_ context.Context) error {
	var fakeFromDb, err = queries.Objects[attrs.Definer](m).Create(m)
	if err != nil {
		return err
	}
	if fakeFromDb == nil {
		return query_errors.ErrNilPointer
	}
	var fm, ok = fakeFromDb.(*fakeModel)
	if !ok {
		return query_errors.ErrTypeMismatch
	}
	*m = *fm // Copy the fields from the created model
	return nil
}

func (m *fakeModel) update(_ context.Context) error {
	var primary = m.FieldDefs().Primary()
	if primary == nil {
		return query_errors.ErrNilPointer
	}

	var val = primary.GetValue()
	if val == nil || fields.IsZero(val) {
		return query_errors.ErrFieldNull
	}

	var ct, err = queries.Objects[attrs.Definer](m).
		Filter(primary.Name(), val).
		Update(m)
	if err != nil {
		return err
	}
	if ct == 0 {
		return query_errors.ErrNoRows
	}
	return nil
}
