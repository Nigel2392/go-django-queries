package models

import (
	"reflect"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/models"
)

var _ queries.DataModel = &Model{}

type modelOptions struct {
	selfField *reflect.StructField
	selfValue *reflect.Value
	_meta     attrs.ModelMeta
	_defs     *attrs.ObjectDefinitions
}

type Model struct {
	internals modelOptions
	data      queries.ModelDataStore
}

func (m *Model) Define(def attrs.Definer, f ...attrs.Field) *attrs.ObjectDefinitions {
	attrs.RegisterModel(def)

	if m.internals._meta == nil {
		m.internals._meta = attrs.GetModelMeta(def)
	}

	var signalInfo = ModelSignalInfo{
		Data: make(map[string]any),
	}

	// validate if it is the same object
	// if not, clear the _defs so any old fields pointing to the old
	// object will be cleared
	if m.internals._defs != nil && m.internals._defs.Object != def {
		signalInfo.Flags.set(FlagModelReset)
		signalInfo.Data["old"] = m.internals._defs.Object
		signalInfo.Data["new"] = def
		m.internals._defs = nil
	}

	if m.internals.selfField == nil {
		var model = reflect.TypeOf(def)
		if model.Kind() == reflect.Ptr {
			model = model.Elem()
		}

		var self, ok = model.FieldByName("Model")
		if !ok || !self.Anonymous {
			panic("object does not have an embedded Model field, the object must embed the Model struct")
		}
		m.internals.selfField = &self

		if m.internals.selfField.Type.Kind() == reflect.Ptr {
			panic("object must not embed a pointer to Model, it must embed the Model struct")
		}

		var rVal = reflect.ValueOf(def)
		var selfValue = rVal.Elem().FieldByIndex(m.internals.selfField.Index)
		if !selfValue.CanAddr() {
			panic("the Model is not addressable, the object must embed the Model struct")
		}
		m.internals.selfValue = &selfValue
	}

	var tableName = m.internals.selfField.Tag.Get("table")
	if m.internals._defs == nil {
		// var reverseRelations = make([]attrs.Field, 0)
		for head := m.internals._meta.ReverseMap().Front(); head != nil; head = head.Next() {
			var (
				field attrs.Field

				key            = head.Key
				value          = head.Value
				typ            = value.Type()
				from           = value.From()
				fromModelField = from.Field()
			)

			switch typ {
			case attrs.RelOneToOne: // OneToOne
				field = fields.NewRelatedField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToOne: // ManyToOne, ForeignKey
				field = fields.NewRelatedField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelOneToMany: // OneToMany, ForeignKeyReverse
				field = fields.NewForeignKeyReverseField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToMany: // ManyToMany
				field = fields.NewManyToManyField[attrs.Definer](def, m, key, key, fromModelField.ColumnName(), value)
			}

			if field != nil {
				f = append(f, field)
			}
		}

		SIGNAL_MODEL_SETUP.Send(ModelSignal{
			SignalInfo: signalInfo,
			Model:      m,
			Object:     def,
		})

		m.internals._defs = attrs.Define(def, f...)
	}

	if tableName != "" && m.internals._defs.Table == "" {
		m.internals._defs.Table = tableName
	}

	return m.internals._defs
}

//
//func (m *Model) String() string {
//	if m.internals._defs == nil {
//		return fmt.Sprintf("{%T}", m)
//	}
//	var rTyp = reflect.TypeOf(m.internals._defs.Object)
//	if rTyp.Kind() == reflect.Ptr {
//		rTyp = rTyp.Elem()
//	}
//	var prim = m.internals._defs.Primary()
//	var primaryVal, _ = prim.Value()
//	return fmt.Sprintf("%s(%v)%v", rTyp.Name(), primaryVal, m.data)
//}

func (m *Model) ModelMeta() attrs.ModelMeta {
	if m.internals._meta == nil {
		m.internals._meta = attrs.GetModelMeta(m.internals._defs.Object)
	}
	return m.internals._meta
}

func (m *Model) RelatedField(name string) (attrs.Field, bool) {
	if m.internals._defs == nil {
		return nil, false
	}
	if m.internals._meta == nil {
		m.internals._meta = attrs.GetModelMeta(m.internals._defs.Object)
	}
	var (
		_, ok1 = m.internals._meta.Forward(name)
		_, ok2 = m.internals._meta.Reverse(name)
	)
	if ok1 || ok2 {
		return m.internals._defs.Field(name)
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
	if m.internals._defs == nil {
		return nil
	}

	for _, field := range m.internals._defs.Fields() {
		if saver, ok := field.(models.Saver); ok {
			if err := saver.Save(); err != nil {
				return err
			}
		}
	}

	return nil
}
