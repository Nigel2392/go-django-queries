package models

import (
	"fmt"
	"maps"
	"reflect"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/models"
)

var (
	_ queries.DataModel                    = &Model{}
	_ queries.Annotator                    = &Model{}
	_ queries.ThroughModelSetter           = &Model{}
	_ attrs.CanCreateObject[attrs.Definer] = &Model{}
)

type modelOptions struct {
	// base model information, used to extract the model / proxy chain
	base   *BaseModelInfo
	object *reflect.Value
	defs   *attrs.ObjectDefinitions
	meta   attrs.ModelMeta
}

type proxyModel struct {
	proxy  *BaseModelProxy
	object *Model
}

type Model struct {
	// internals of the model, used to store
	// the model's base information, object, definitions, etc.
	// this is set to nil if the model is not setup yet
	internals *modelOptions

	// next model in the chain, used for embedding models
	// if not nil, this references a proxy model.
	proxy *proxyModel

	// ThroughModel is a model bound to the current
	// object, it will be set if the model is a
	// target of a ManyToMany or OneToMany relation
	// with a through model.
	ThroughModel attrs.Definer

	// annotations for the model, used to store
	// database annotation key value pairs
	Annotations map[string]any

	// data store for the model, used to store model data
	// like annotations, custom data, etc.
	data queries.ModelDataStore
}

func (m *Model) __Model() private { return private{} }

// checkValid checks if the model is valid and initialized.
func (m *Model) checkValid() {
	assert.False(m.internals == nil,
		"model internals are not initialized, model is improperly initialized",
	)
	assert.False(m.internals.base == nil,
		"model base information is not set, model is improperly initialized",
	)
	assert.False(m.internals.object == nil,
		"model object is not set, model is improperly initialized",
	)
}

// CreateObject creates a new object of the model type
// and sets it up with the model's definitions.
//
// It returns nil if the object is not valid or if the model
// is not registered with the model system.
//
// This automatically sets up the model's fields
// and handles the proxy model if it exists.
//
// This method is automatically called by the
// [attrs.NewObject] function when a new object is created.
func (m *Model) CreateObject(object attrs.Definer) attrs.Definer {
	if !attrs.IsModelRegistered(object) {
		return nil
	}

	var obj = reflect.ValueOf(object)
	if !obj.IsValid() || obj.IsNil() {
		return nil
	}

	var base = getModelChain(object)
	if base == nil {
		return nil
	}

	var newObj = reflect.New(obj.Type().Elem())
	var modelVal = newObj.Elem().FieldByIndex(
		base.base.Index,
	)

	var model = modelVal.Addr().Interface().(*Model)
	if err := model.Setup(newObj.Interface().(attrs.Definer)); err != nil {
		return nil
	}

	return newObj.Interface().(attrs.Definer)
}

func (m *Model) Setup(def attrs.Definer) error {
	if def == nil {
		return ErrObjectInvalid
	}

	// Retrieve the pre-compiled model chain
	var base = getModelChain(def)
	if base == nil {
		return fmt.Errorf(
			"object %T does not have an embedded Model field: %w",
			def, ErrModelEmbedded,
		)
	}

	// check if the object's model field is
	// points to the current model
	var defValue = reflect.ValueOf(def)
	var defElem = defValue.Elem()
	var self = defElem.FieldByIndex(base.base.Index)
	if self.Addr().Pointer() != reflect.ValueOf(m).Pointer() {
		return fmt.Errorf(
			"object %T is not the same as the model %T, expected %d, got %d",
			def, m, self.Addr().Pointer(), reflect.ValueOf(m).Pointer(),
		)
	}

	var sig = ModelSignal{
		SignalInfo: ModelSignalInfo{
			Data: make(map[string]any),
		},
		Model:  m,
		Object: def,
	}

	// Handle the model's proxy object if it exists.
	var wasChanged, err = m.setupProxy(
		base,
		defValue,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to setup proxy for model %T: %w",
			def, err,
		)
	}

	// if the proxy was changed it needs
	// to be reset, we need to clear the internals
	// as some fields may be pointing to the old object
	if wasChanged && m.internals != nil {
		sig.SignalInfo.Flags.set(FlagProxySetup)
		m.internals.object = nil
		m.internals.defs = nil
	}

	// validate if it is the same object
	// if not, clear the defs so any old fields pointing to the old
	// object will be cleared
	if (m.internals != nil && m.internals.defs != nil) && (m.internals.object != nil && m.internals.object.Pointer() != defValue.Pointer()) {
		sig.SignalInfo.Flags.set(FlagModelReset)
		sig.SignalInfo.Data["old"] = m.internals.defs.Object
		sig.SignalInfo.Data["new"] = def
		m.internals.defs = nil
		m.internals.object = nil
	}

	// if the model is not setup, we need to initialize it
	if m.internals == nil || m.internals.object == nil {
		sig.SignalInfo.Flags.set(FlagModelSetup)
		m.internals = &modelOptions{
			object: &defValue,
			base:   base,
		}
	}

	// send the model setup signal
	if !sig.SignalInfo.Flags.True(ModelSignalFlagNone) {
		if err := SIGNAL_MODEL_SETUP.Send(sig); err != nil {
			return fmt.Errorf(
				"failed to emit model setup signal for %T: %w",
				def, err,
			)
		}
	}

	return nil
}

// setupProxy sets up the proxy for the model if it exists.
// It checks if the proxy field is set, and if so, it extracts the
// embedded model from the proxy field and calls Setup on it with the
// provided definer proxy object.
func (m *Model) setupProxy(base *BaseModelInfo, parent reflect.Value) (changed bool, err error) {
	if base.proxy == nil {
		return false, nil
	}

	if parent.Kind() == reflect.Ptr {
		parent = parent.Elem()
	}

	var (
		rVal         = parent.FieldByIndex(base.proxy.rootField.Index)
		initialIsNil = rVal.IsNil()
		nextNil      = (m.proxy == nil || m.proxy.object == nil)
		nilDiff      = !initialIsNil && nextNil
		ptrDiff      = (!nextNil && m.proxy.object.internals.object.Pointer() != rVal.Pointer())
	)

	m.proxy = &proxyModel{
		proxy: base.proxy,
	}

	// determine if the proxy needs to be reset
	changed = nilDiff || ptrDiff

	// if the proxy is nil, we need to create a new one when specified
	if initialIsNil && (base.proxy.directField.Tag.Get("auto") == "true" || base.proxy.rootField.Tag.Get("auto") == "true") {
		var newObj = attrs.NewObject[attrs.Definer](base.proxy.rootField.Type)
		rVal.Set(reflect.ValueOf(newObj))
		changed = true
	}

	// if there is a difference in the pointer or one of
	// the pointers is nil, we need to reset the proxy
	if !rVal.IsNil() && changed {
		changed = true
		var proxy = rVal.Interface().(attrs.Definer)
		var modelValue = rVal.Elem().FieldByIndex(
			base.proxy.next.base.Index,
		)
		var modelPtr = modelValue.Addr().Interface()
		m.proxy.object = modelPtr.(*Model)

		// proxy object must not be nil
		if m.proxy.object == nil {
			return false, fmt.Errorf(
				"failed to extract embedded model from proxy field %s: %w",
				base.proxy.rootField.Name, ErrObjectInvalid,
			)
		}

		// setup the proxy object
		err = m.proxy.object.Setup(proxy)
		if err != nil {
			return false, fmt.Errorf(
				"failed to setup embedded model from proxy field %s: %w",
				base.proxy.rootField.Name, err,
			)
		}
	}

	return changed, err
}

// Define defines the fields of the model based on the provided definer
//
// Normally this would be done with [attrs.Define], the current model method
// is a convenience method which also handles the setup of the model
// as well as reverse relation setup.
func (m *Model) Define(def attrs.Definer, flds ...any) *attrs.ObjectDefinitions {
	if err := m.Setup(def); err != nil {
		panic("failed to setup model: " + err.Error())
	}

	var f, err = attrs.UnpackFieldsFromArgs(def, flds...)
	assert.True(
		err == nil,
		"failed to unpack fields from args: %v", err,
	)

	m.checkValid()
	var tableName = m.internals.base.base.Tag.Get("table")
	if m.internals.defs == nil {
		var meta = attrs.GetModelMeta(def)
		for head := meta.ReverseMap().Front(); head != nil; head = head.Next() {
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

		//	for i, field := range f {
		//		if field.IsPrimary() {
		//			f[i] = &primaryField{
		//				Field: field,
		//				model: m,
		//			}
		//			break
		//		}
		//	}

		m.internals.defs = attrs.Define(def, f...)
	}

	if tableName != "" && m.internals.defs.Table == "" {
		m.internals.defs.Table = tableName
	}

	return m.internals.defs
}

func (m *Model) initDefaults(defs *attrs.ObjectDefinitions) *attrs.ObjectDefinitions {
	var primary = m.internals.defs.Primary()
	if primary == nil {
		return m.internals.defs
	}

	var val, err = primary.Value()
	if err != nil {
		panic(fmt.Errorf(
			"failed to get primary key value for model %T: %w",
			m.internals.object.Interface(), err,
		))
	}

	if val == nil || attrs.IsZero(val) {
		return m.internals.defs
	}

	for head := m.internals.defs.ObjectFields.Front(); head != nil; head = head.Next() {
		var field = head.Value
		if field.IsPrimary() {
			continue
		}

		// This is only for related fields
		if field.Rel() == nil {
			continue
		}

		var (
			shouldSet bool
			value     = field.GetValue()
			dftValue  = field.GetDefault()
			valZero   = attrs.IsZero(value)
			dftZero   = attrs.IsZero(dftValue)
		)

		if valZero && !dftZero {
			shouldSet = true
			value = dftValue
		}

		if !valZero && !dftZero {
			assert.Err(attrs.BindValueToModel(
				defs.Object, field, value,
			))
		}

		if shouldSet {
			// If the field has a default value, set it.
			// This is useful for fields that are not set yet.
			field.SetValue(value, true)
		}
	}

	return m.internals.defs
}

func (m *Model) Object() attrs.Definer {
	m.checkValid()
	return m.internals.object.Interface().(attrs.Definer)
}

func (m *Model) ModelMeta() attrs.ModelMeta {
	m.checkValid()
	if m.internals.meta == nil {
		m.internals.meta = attrs.GetModelMeta(*m.internals.object)
	}
	return m.internals.meta
}

func (m *Model) RelatedField(name string) (attrs.Field, bool) {
	if m.internals.defs == nil {
		return nil, false
	}
	var meta = m.ModelMeta()
	var (
		_, ok1 = meta.Forward(name)
		_, ok2 = meta.Reverse(name)
	)
	if ok1 || ok2 {
		return m.internals.defs.Field(name)
	}
	return nil, false
}

func (m *Model) Annotate(annotations map[string]any) {
	if m.Annotations == nil {
		m.Annotations = make(map[string]any)
	}

	maps.Copy(m.Annotations, annotations)
}

func (m *Model) SetThroughModel(throughModel attrs.Definer) {
	m.ThroughModel = throughModel
}

func (m *Model) DataStore() queries.ModelDataStore {
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
