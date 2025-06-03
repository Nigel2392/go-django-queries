package models

import (
	"fmt"
	"reflect"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/models"
)

var _ queries.DataModel = &Model{}

type modelOptions struct {
	// base model information, used to extract the model / proxy chain
	base *BaseModelInfo

	meta   attrs.ModelMeta
	object *reflect.Value
	defs   *attrs.ObjectDefinitions
}

type proxyModel struct {
	proxy  *BaseModelProxy
	object *Model
}

type Model struct {
	internals *modelOptions

	// next model in the chain, used for embedding models
	// if not nil, this references a proxy model.
	proxy *proxyModel

	// data store for the model, used to store model data
	// like annotations, custom data, etc.
	data queries.ModelDataStore
}

// Setup initializes a model's field definitions and binds
// the model's values to the model.
//
// The model has to be saved to the database before it can be used,
// otherwise it will panic.
func Setup[T attrs.Definer](model T) T {
	var m, err = ExtractModel(model)
	if err != nil {
		panic("failed to extract model: " + err.Error())
	}

	if m.internals == nil {
		if err := m.Setup(model); err != nil {
			panic("failed to setup model: " + err.Error())
		}
	}

	if m.internals.object == nil {
		if err := m.Setup(model); err != nil {
			panic("failed to setup model: " + err.Error())
		}
	}

	return model
}

func Define[T1 attrs.Definer](def T1, f ...any) *attrs.ObjectDefinitions {
	var baseModelInfo = getModelChain(def)
	var model, err = extractFromInfo(baseModelInfo, def)
	if err != nil {
		panic("failed to extract model: " + err.Error())
	}
	return model.Define(def, f...)
}

func (m *Model) __Model() {}

func (m *Model) Setup(def attrs.Definer) error {
	if def == nil {
		return ErrObjectInvalid
	}

	var base = getModelChain(def)
	if base == nil {
		return fmt.Errorf(
			"object %T does not have an embedded Model field: %w",
			def, ErrModelEmbedded,
		)
	}

	var defValue = reflect.ValueOf(def)
	var defElem = defValue.Elem()
	var self = defElem.FieldByIndex(base.base.Index)
	if self.Addr().Pointer() != reflect.ValueOf(m).Pointer() {
		return fmt.Errorf(
			"object %T is not the same as the model %T, expected %d, got %d",
			def, m, self.Addr().Pointer(), reflect.ValueOf(m).Pointer(),
		)
	}

	if base.proxy != nil {
		var (
			rVal         = defElem.FieldByIndex(base.proxy.rootField.Index)
			initialIsNil = rVal.IsNil()
			nextNil      = (m.proxy == nil || m.proxy.object == nil)
			nilDiff      = !initialIsNil && nextNil
			ptrDiff      = (!nextNil && m.proxy.object.internals.object.Pointer() != rVal.Pointer())
			reset        = nilDiff || ptrDiff
		)

		m.proxy = &proxyModel{
			proxy: base.proxy,
		}

		if initialIsNil && base.proxy.directField.Tag.Get("auto") == "true" {
			rVal.Set(reflect.New(base.proxy.rootField.Type.Elem()))
			reset = true
		}

		var err error
		if !rVal.IsNil() && reset {
			var proxy = rVal.Interface().(attrs.Definer)
			m.proxy.object, err = extractFromInfo(
				base.proxy.next, proxy,
			)
			if err != nil {
				return fmt.Errorf(
					"failed to extract embedded model from proxy field %s: %w",
					base.proxy.rootField.Name, err,
				)
			}

			if m.proxy.object == nil {
				return fmt.Errorf(
					"failed to extract embedded model from proxy field %s: %w",
					base.proxy.rootField.Name, ErrObjectInvalid,
				)
			}

			err = m.proxy.object.Setup(proxy)
			if err != nil {
				return fmt.Errorf(
					"failed to setup embedded model from proxy field %s: %w",
					base.proxy.rootField.Name, err,
				)
			}
		}

		if reset && m.internals != nil {
			m.internals.object = nil
			m.internals.defs = nil
		}
	}

	if m.internals != nil && m.internals.object != nil && m.internals.object.Pointer() == defValue.Pointer() {
		return nil
	}

	m.internals = &modelOptions{
		defs:   nil,
		object: &defValue,
		base:   base,
		meta:   attrs.GetModelMeta(def),
	}

	return nil
}

func (m *Model) Define(def attrs.Definer, flds ...any) *attrs.ObjectDefinitions {
	if err := m.Setup(def); err != nil {
		panic("failed to setup model: " + err.Error())
	}

	var f, err = attrs.UnpackFieldsFromArgs(def, flds...)
	if err != nil {
		panic("failed to unpack fields: " + err.Error())
	}

	if m.internals == nil {
		panic("model internals are not initialized, call Setup() first")
	}

	var signalInfo = ModelSignalInfo{
		Data: make(map[string]any),
	}

	// validate if it is the same object
	// if not, clear the defs so any old fields pointing to the old
	// object will be cleared
	if m.internals.defs != nil && m.internals.defs.Object != def {
		signalInfo.Flags.set(FlagModelReset)
		signalInfo.Data["old"] = m.internals.defs.Object
		signalInfo.Data["new"] = def
		m.internals.defs = nil
	}

	if m.internals.base == nil {
		m.internals.base = getModelChain(def)
		if m.internals.base == nil {
			panic("object does not have an embedded Model field, the object must embed the Model struct")
		}

		if m.internals.base.base.Type.Kind() == reflect.Ptr {
			panic("object must not embed a pointer to Model, it must embed the Model struct")
		}
	}

	var tableName = m.internals.base.base.Tag.Get("table")
	if m.internals.defs == nil {
		// var reverseRelations = make([]attrs.Field, 0)
		for head := m.internals.meta.ReverseMap().Front(); head != nil; head = head.Next() {
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
				if head.Value.Through() == nil {
					field = fields.NewOneToOneReverseField[attrs.Definer](def, def, key, key, fromModelField.ColumnName(), value)
				} else {
					field = fields.NewOneToOneReverseField[queries.Relation](def, def, key, key, fromModelField.ColumnName(), value)
				}
			case attrs.RelManyToOne: // ManyToOne, ForeignKey
				field = fields.NewForeignKeyField[attrs.Definer](def, def, key, key, fromModelField.ColumnName(), value)
			case attrs.RelOneToMany: // OneToMany, ForeignKeyReverse
				field = fields.NewForeignKeyReverseField[*queries.RelRevFK[attrs.Definer]](def, def, key, key, fromModelField.ColumnName(), value)
			case attrs.RelManyToMany: // ManyToMany
				field = fields.NewManyToManyField[*queries.RelM2M[attrs.Definer, attrs.Definer]](def, def, key, key, fromModelField.ColumnName(), value)
			default:
				panic("unknown relation type: " + typ.String())
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

		m.internals.defs = attrs.Define(def, f...)
	}

	if tableName != "" && m.internals.defs.Table == "" {
		m.internals.defs.Table = tableName
	}

	return m.internals.defs
}

type ProxyChain struct {
	Next        *ProxyChain
	Model       *Model
	Object      attrs.Definer
	Definitions attrs.Definitions
}

func (UNUSED *Model) ProxyChain() *ProxyChain {
	var (
		currTyp = UNUSED.internals.base.proxy
		currObj = reflect.New(UNUSED.internals.object.Type().Elem())
	)
	currObj.Elem().Set(UNUSED.internals.object.Elem())

	var _static = reflect.ValueOf(UNUSED)
	var self = currObj.Elem().FieldByIndex(
		UNUSED.internals.base.base.Index,
	)
	self = self.Addr()
	assert.False(
		currObj.Pointer() == UNUSED.internals.object.Pointer(),
		"model (%x) is equal to the current object %T's model, this should not happen (%x == %x)",
		currObj.Pointer(), UNUSED.internals.object.Elem().Type().String(),
		UNUSED.internals.object.Pointer(), currObj.Pointer(),
	)
	assert.False(
		self.Pointer() == _static.Pointer(),
		"model (%x) is equal to the current object %T's model, this should not happen (%x == %x)",
		self.Pointer(), UNUSED.internals.object.Elem().Type().String(), _static.Pointer(), self.Pointer(),
	)

	var USED = self.Interface().(*Model)
	USED.internals = nil
	USED.data = nil
	USED.proxy = nil

	var object = currObj.Interface().(attrs.Definer)
	if err := USED.Setup(object); err != nil {
		panic(fmt.Sprintf(
			"failed to setup model %T from object %T: %v",
			object, UNUSED.internals.object.Elem().Type(), err,
		))
	}

	fmt.Printf("Setting up model %T from object %T %T %T %x %x %v\n", USED, object, currObj.Interface(), UNUSED.internals.object.Interface(), UNUSED.internals.object.Pointer(), USED.internals.object.Pointer(), UNUSED.internals.object.Pointer() == currObj.Pointer())
	var root = &ProxyChain{
		Model:       USED,
		Object:      object,
		Definitions: object.FieldDefs(),
	}

	var chain = root
	var i = 0
	currObj = currObj.Elem()
	for currTyp != nil {
		if currTyp.rootField == nil {
			panic(fmt.Sprintf(
				"proxy field %d in model %T is nil, this should not happen",
				i, USED,
			))
		}

		var field = currObj.FieldByIndex(currTyp.rootField.Index)
		if !field.IsValid() {
			panic(fmt.Sprintf(
				"proxy field %d in model %T is not valid, this should not happen",
				i, USED,
			))
		}

		var fieldCopy = reflect.New(currTyp.rootField.Type.Elem())
		if !field.IsNil() {
			fieldCopy.Elem().Set(field.Elem())
		}

		var objVal = fieldCopy
		if fieldCopy.Kind() == reflect.Ptr {
			fieldCopy = fieldCopy.Elem()
		}

		var modelVal = fieldCopy.FieldByIndex(currTyp.next.base.Index)
		var model = modelVal.Addr().Interface().(*Model)
		model.internals = nil
		model.data = nil
		model.proxy = nil

		var obj = objVal.Interface().(attrs.Definer)
		var fieldDefs = obj.FieldDefs()
		if reflect.TypeOf(fieldDefs.Instance()) != objVal.Type() {
			panic(fmt.Sprintf(
				"proxy field %d in model %T has a different FieldDefs() instance type than the object %T",
				i, USED, obj,
			))
		}

		if err := model.Setup(obj); err != nil {
			panic(fmt.Sprintf(
				"failed to setup model %T from proxy field %d in model %T: %v",
				obj, i, USED, err,
			))
		}

		chain.Next = &ProxyChain{
			Model:       model,
			Object:      obj,
			Definitions: fieldDefs,
		}

		if currTyp.next == nil {
			break
		}

		currObj = fieldCopy
		currTyp = currTyp.next.proxy
		chain = chain.Next
		i++
	}

	return root
}

func (m *Model) Object() attrs.Definer {
	if m.internals == nil || m.internals.object == nil {
		return nil
	}
	return m.internals.object.Interface().(attrs.Definer)
}

func (m *Model) ModelMeta() attrs.ModelMeta {
	if m.internals.meta == nil {
		m.internals.meta = attrs.GetModelMeta(m.internals.defs.Object)
	}
	return m.internals.meta
}

func (m *Model) RelatedField(name string) (attrs.Field, bool) {
	if m.internals.defs == nil {
		return nil, false
	}
	if m.internals.meta == nil {
		m.internals.meta = attrs.GetModelMeta(m.internals.defs.Object)
	}
	var (
		_, ok1 = m.internals.meta.Forward(name)
		_, ok2 = m.internals.meta.Reverse(name)
	)
	if ok1 || ok2 {
		return m.internals.defs.Field(name)
	}
	return nil, false
}

func (m *Model) ModelDataStore() queries.ModelDataStore {
	if m.data == nil {
		m.data = make(MapDataStore)
	}
	return m.data
}

func (m *Model) SaveFields() error {
	if m.internals.defs == nil {
		return nil
	}

	for _, field := range m.internals.defs.Fields() {
		if saver, ok := field.(models.Saver); ok {
			if err := saver.Save(); err != nil {
				return err
			}
		}
	}

	return nil
}
