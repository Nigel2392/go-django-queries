package models

import (
	"fmt"
	"reflect"
	"strings"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var _ queries.DataModel = &Model{}

type mapDataStore map[string]interface{}

func (m mapDataStore) String() string {
	var sb strings.Builder
	sb.WriteString("[")
	var i = 0
	for k, v := range m {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%q: %v", k, v)
		i++
	}
	sb.WriteString("]")
	return sb.String()
}

func (m mapDataStore) HasValue(key string) bool {
	_, ok := m[key]
	return ok
}

func (m mapDataStore) SetValue(key string, value any) error {
	m[key] = value
	return nil
}

func (m mapDataStore) GetValue(key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	return nil, false
}

func (m mapDataStore) DeleteValue(key string) error {
	delete(m, key)
	return nil
}

type Model struct {
	data  queries.ModelDataStore
	_meta attrs.ModelMeta
	_defs *attrs.ObjectDefinitions
}

func (m *Model) Define(def attrs.Definer, f ...attrs.Field) *attrs.ObjectDefinitions {
	attrs.RegisterModel(def)

	if m._meta == nil {
		m._meta = attrs.GetModelMeta(def)
	}

	var model = reflect.TypeOf(def)
	if model.Kind() == reflect.Ptr {
		model = model.Elem()
	}

	var self, ok = model.FieldByName("Model")
	if !ok {
		panic("model does not have a Model field, did you forget to embed the Model struct?")
	}

	var tableName = self.Tag.Get("table")
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
				field = fields.NewRelatedField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToOne: // ForeignKey, ForeignKey
				field = fields.NewRelatedField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelOneToMany: // ForeignKeyReverse, ForeignKeyReverse
				field = fields.NewForeignKeyReverseField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToMany: // ManyToMany
				field = fields.NewManyToManyField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			}

			if field != nil {
				f = append(f, field)
			}
		}

		m._defs = attrs.Define(def, f...)
	}

	if tableName != "" && m._defs.Table == "" {
		m._defs.Table = tableName
	}

	return m._defs
}

//
//func (m *Model) String() string {
//	var rTyp = reflect.TypeOf(m._defs.Object)
//	if rTyp.Kind() == reflect.Ptr {
//		rTyp = rTyp.Elem()
//	}
//	var prim = m._defs.Primary()
//	var primaryVal, _ = prim.Value()
//	return fmt.Sprintf("%s(%v)%v", rTyp.Name(), primaryVal, m.data)
//}

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

func (m *Model) ModelDataStore() queries.ModelDataStore {
	if m.data == nil {
		m.data = make(mapDataStore)
	}
	return m.data
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
