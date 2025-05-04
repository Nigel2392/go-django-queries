package models

import (
	"github.com/Nigel2392/go-django-queries/internal"
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
	_meta queries.ModelMeta
	_defs *attrs.ObjectDefinitions
}

func (m *Model) Define(def attrs.Definer, f ...attrs.Field) *attrs.ObjectDefinitions {
	queries.RegisterModel(def)
	if m._meta == nil {
		m._meta = queries.GetModelMeta(def)
	}
	if m._defs == nil {
		// var reverseRelations = make([]attrs.Field, 0)
		for key, value := range m._meta.IterReverse() {
			var (
				typ   = value.Type()
				chain = value.Chain()
			)

			var (
				field         attrs.Field
				relModelField = chain.Field()
			)

			if relModelField == nil {
				continue
			}

			key = internal.GetReverseAlias(relModelField, key)
			switch typ {
			case queries.RelationTypeOneToOne: // OneToOne
				field = fields.NewRelatedField[attrs.Definer](def, m, key, relModelField.ColumnName(), value)
			case queries.RelationTypeManyToMany: // ManyToMany
				field = fields.NewRelatedField[[]attrs.Definer](def, m, key, relModelField.ColumnName(), value)
			case queries.RelationTypeOneToMany: // OneToMany, ForeignKey
				field = fields.NewRelatedField[attrs.Definer](def, m, key, relModelField.ColumnName(), value)
			case queries.RelationTypeManyToOne: // ManyToOne, ForeignKeyReverse
				field = fields.NewRelatedField[attrs.Definer](def, m, key, relModelField.ColumnName(), value)
			}

			if field != nil {
				f = append(f, field)
			}
		}

		m._defs = attrs.Define(def, f...)
	}
	return m._defs
}

func (m *Model) ModelMeta() queries.ModelMeta {
	if m._meta == nil {
		m._meta = queries.GetModelMeta(m._defs.Object)
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
