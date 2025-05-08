package models

import (
	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var _ queries.DataModel = &Model{}

type mapDataStore map[string]interface{}

func (m mapDataStore) set(key string, value any) error {
	m[key] = value
	return nil
}

func (m mapDataStore) get(key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	return nil, false
}

func (m mapDataStore) delete(key string) error {
	delete(m, key)
	return nil
}

type datastore interface {
	set(key string, value any) error
	get(key string) (any, bool)
	delete(key string) error
}

type Model struct {
	data  datastore
	_meta attrs.ModelMeta
	_defs *attrs.ObjectDefinitions
}

func (m *Model) Define(def attrs.Definer, f ...attrs.Field) *attrs.ObjectDefinitions {
	attrs.RegisterModel(def)

	if m._meta == nil {
		m._meta = attrs.GetModelMeta(def)
	}

	if m._defs == nil {
		// var reverseRelations = make([]attrs.Field, 0)
		for head := m._meta.ReverseMap().Front(); head != nil; head = head.Next() {
			var key = head.Key
			var value = head.Value
			var typ = value.Type()

			var (
				field attrs.Field
			)

			var (
				from           = value.From()
				fromModelField = from.Field()
			)

			switch typ {
			case attrs.RelOneToOne: // OneToOne
				field = fields.NewOneToOneReverseField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToOne: // ForeignKey, ForeignKey
				field = fields.NewForeignKeyField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelOneToMany: // ForeignKeyReverse, ForeignKeyReverse
				field = fields.NewForeignKeyReverseField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToMany: // ManyToMany
				field = fields.NewManyToManyReverseField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			}

			if field != nil {
				f = append(f, field)
			}
		}

		m._defs = attrs.Define(def, f...)
	}
	return m._defs
}

func (m *Model) ModelMeta() attrs.ModelMeta {
	if m._meta == nil {
		m._meta = attrs.GetModelMeta(m._defs.Object)
	}
	return m._meta
}

func (m *Model) RelatedField(name string) (attrs.Field, bool) {
	if m._defs == nil {
		return nil, false
	}
	var (
		_, ok1 = m._meta.Forward(name)
		_, ok2 = m._meta.Reverse(name)
	)
	if ok1 || ok2 {
		return m._defs.Field(name)
	}
	return nil, false
}

func (m *Model) HasQueryValue(key string) bool {
	if m.data == nil {
		return false
	}
	_, ok := m.data.get(key)
	return ok
}

func (m *Model) GetQueryValue(key string) (any, bool) {
	if m.data == nil {
		return nil, false
	}
	var val, ok = m.data.get(key)
	return val, ok
}

func (m *Model) SetQueryValue(key string, value any) error {
	if m.data == nil {
		m.data = make(mapDataStore)
	}
	m.data.set(key, value)
	return nil
}

func (m *Model) SaveFields() error {
	if m._defs == nil {
		return nil
	}
	for _, field := range m._defs.Fields() {
		if saver, ok := field.(interface{ Save() error }); ok {
			if err := saver.Save(); err != nil {
				return err
			}
		}
	}
	return nil
}
