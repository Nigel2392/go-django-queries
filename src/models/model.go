package models

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/Nigel2392/go-signals"
)

var (
	// Internal interfaces that the model should implement
	_ _ModelInterface = &Model{}
	_ CanSaveObject   = &Model{}

	// Third party interfaces that the model should implement
	_ models.ContextSaver                  = &Model{}
	_ queries.CanSetup                     = &Model{}
	_ queries.DataModel                    = &Model{}
	_ queries.Annotator                    = &Model{}
	_ queries.ThroughModelSetter           = &Model{}
	_ attrs.CanSignalChanged               = &Model{}
	_ attrs.CanCreateObject[attrs.Definer] = &Model{}
)

type modelOptions struct {
	// base model information, used to  extract the model / proxy chain
	base   *BaseModelInfo
	object *reflect.Value
	defs   *attrs.ObjectDefinitions
	meta   attrs.ModelMeta
	state  *ModelState
	fromDB bool
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

	// changed is a signal which gets emitted when the
	// model is changed (e.g. fields are set, saved, etc.)
	//
	// it is a loose signal, not bound to any specific
	// signal pool. this means it is used only for this model.
	changed signals.Signal[ModelChangeSignal]

	// next model in the chain, used for embedding models
	// if not nil, this references a proxy model.
	proxy *proxyModel

	// data store for the model, used to store model data
	// like annotations, custom data, etc.
	data queries.ModelDataStore

	// ThroughModel is a model bound to the current
	// object, it will be set if the model is a
	// target of a ManyToMany or OneToMany relation
	// with a through model.
	ThroughModel attrs.Definer

	// annotations for the model, used to store
	// database annotation key value pairs
	Annotations map[string]any
}

// Setup sets up a [attrs.Definer] object so that it's model is properly initialized.
//
// This method is normally called automatically, but when manually defining a struct
// as a model, this method should be called to ensure the model is properly initialized.
//
// In short, this must be called if the model is not created using [attrs.NewObject].
func Setup[T attrs.Definer](def T) T {
	var model, err = ExtractModel(def)
	assert.True(
		err == nil,
		"failed to extract model from definer %T: %v", def, err,
	)
	assert.False(
		model == nil,
		"model is nil, cannot setup model for definer %T", def,
	)

	err = model.Setup(def)
	assert.True(
		err == nil,
		"failed to setup model %T: %v", def, err,
	)
	return def
}

func (m *Model) __Model() private { return private{} }

// checkValid checks if the model is valid and initialized.
func (m *Model) checkValid() {
	assert.False(m.internals == nil,
		fmt.Errorf("model internals are not initialized: %w", ErrModelInitialized),
	)
	assert.False(m.internals.base == nil,
		fmt.Errorf("model base information is not set: %w", ErrModelInitialized),
	)
	assert.False(m.internals.object == nil,
		fmt.Errorf("model object is not set: %w", ErrModelInitialized),
	)
}

func (m *Model) setupInitialState() {
	if m.internals.defs == nil {
		// if the model definitions are not set, we cannot setup the state
		// a nil state assumes that the model is always changed.
		return
	}

	m.internals.state = initState(m)
}

// onChange is a callback that is called when the model changes.
// It is used to handle changes in the model's fields to update the model's state
// when the signal is emitted.
func (m *Model) onChange(s signals.Signal[ModelChangeSignal], ms ModelChangeSignal) error {
	m.checkValid()

	if ms.Model != m {
		panic(fmt.Errorf(
			"model signal %T is not for model %T (%p != %p)",
			ms.Model, m, ms.Model, m,
		))
	}

	if m.internals.state == nil {
		m.setupInitialState()
	}

	// fmt.Printf(
	// "[onChange] Model %T received signal %s for field %s with flags %v (%v)\n",
	// m.internals.object.Interface(),
	// s.Name(), ms.Field.Name(), ms.Flags, ms.Field.GetValue(),
	// )

	switch {
	case ms.Flags.True(FlagModelReset), ms.Flags.True(FlagModelSetup):
		// set the model's initial state
		m.setupInitialState()
		// fmt.Printf(
		// 	"Proxy model %T has been reset or setup, initial state is now set\n",
		// 	m.internals.object.Interface(),
		// )

	case ms.Flags.True(FlagFieldChanged):
		m.internals.state.change(ms.Field.Name())

		// fmt.Printf(
		// 	"Model %T field %s changed to %v\n",
		// 	m.internals.object.Interface(),
		// 	ms.Field.Name(), ms.Field.GetValue(),
		// )

	case ms.Flags.True(FlagProxySetup), ms.Flags.True(FlagProxyChanged):
		var fieldName = proxyFieldName(ms.Model.internals.base.proxy.rootField.Name)
		m.internals.state.change(fieldName)

		// fmt.Printf(
		// 	"Model %T proxy field %s changed to %v\n",
		// 	m.internals.object.Interface(),
		// 	fieldName, ms.Model.internals.object.Interface(),
		// )
	default:
		// if the signal is not for a field change, we can skip it
		panic(fmt.Errorf(
			"model signal %T is not for a field change, flags: %v",
			ms.Model, ms.Flags,
		))
	}

	return nil
}

// SignalChanged sends a signal that the model has changed.
//
// This is used to allow the [attrs.Definitions] to callback to the model
// and notify it that the model has changed, so it can update its internal state
// and trigger any necessary updates.
func (m *Model) SignalChange(fa attrs.Field, value interface{}) {
	m.checkValid()

	//	fmt.Printf(
	//		"[SignalChange] Model %T field %s changed to %v\n",
	//		m.internals.object.Interface(),
	//		fa.Name(), value,
	//	)

	m.changed.Send(ModelChangeSignal{
		Model:  m,
		Field:  fa,
		Flags:  FlagFieldChanged,
		Object: m.internals.object.Interface().(attrs.Definer),
	})
}

// State returns the current state of the model.
//
// The state is initialized when the model is setup,
// and it contains the initial values of the model's fields
// as well as the changed fields.
func (m *Model) State() *ModelState {
	m.checkValid()
	if m.internals.state == nil {
		m.setupInitialState()
	}
	return m.internals.state
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
		m.changed = nil
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
		m.changed = nil
	}

	if m.changed == nil {
		m.changed = signals.New[ModelChangeSignal]("model.changed")
		m.changed.Listen(m.onChange)
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

	if rVal.IsNil() && changed && !nextNil {
		changed = true
		m.proxy.object = nil
		m.internals.defs = nil
		return changed, nil
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

		m.proxy.object.changed.Listen(func(s signals.Signal[ModelChangeSignal], ms ModelChangeSignal) error {
			//	fmt.Printf(
			//		"Proxy model %T changed\n",
			//		m.proxy.object.internals.object.Interface(),
			//	)
			m.changed.Send(ModelChangeSignal{
				Flags: FlagProxyChanged,
				Next:  &ms,
				Model: m,
			})
			return nil
		})
	}

	return changed, err
}

func (m *Model) proxyScope(qs *queries.QuerySet[attrs.Definer], internals *queries.QuerySetInternals) *queries.QuerySet[attrs.Definer] {
	var (
		newObj     = reflect.New(m.internals.object.Type().Elem()).Interface().(attrs.Definer)
		chain      = NewProxyChain(newObj, true)
		embedder   = chain
		proxy      = chain.Next()
		fieldNames = make([]any, 0)
		fieldChain = make([]string, 0)
	)

	// make sure to select all fields from the root model
	fieldNames = append(fieldNames, "*")

	for proxy != nil {
		var (
			sourceProxyField = embedder.ProxyField()
		)

		if sourceProxyField == nil {
			panic(fmt.Errorf(
				"proxy field is nil in model %T, a proxy field is required for proxy joins",
				embedder.object,
			))
		}

		// apppend the field name to the field chain (this does not include the astrix)
		fieldChain = append(fieldChain, sourceProxyField.Name())
		// if the field is a proxy field, we need to append the field name with an astrix to select all fields
		fieldNames = append(fieldNames, fmt.Sprintf("%s.*", strings.Join(fieldChain, ".")))
		// move down the chain
		embedder = proxy
		proxy = proxy.Next()

	}

	return qs.Select(fieldNames...)
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

	var _fields, err = attrs.UnpackFieldsFromArgs(def, flds...)
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

			if fromModelField == nil {
				panic(fmt.Errorf(
					"reverse relation %q in model %T does not have a field defined",
					key, def,
				))
			}

			var conf = &fields.FieldConfig{
				ScanTo:      def,
				ReverseName: key,
				ColumnName:  fromModelField.ColumnName(),
				Rel:         value,
			}

			switch typ {
			case attrs.RelOneToOne: // OneToOne
				if head.Value.Through() == nil {
					field = fields.NewOneToOneReverseField[attrs.Definer](def, key, conf)
				} else {
					field = fields.NewOneToOneReverseField[queries.Relation](def, key, conf)
				}
			case attrs.RelManyToOne: // ManyToOne, ForeignKey
				field = fields.NewForeignKeyField[attrs.Definer](def, key, conf)
			case attrs.RelOneToMany: // OneToMany, ForeignKeyReverse
				field = fields.NewForeignKeyReverseField[*queries.RelRevFK[attrs.Definer]](def, key, conf)
			case attrs.RelManyToMany: // ManyToMany
				field = fields.NewManyToManyField[*queries.RelM2M[attrs.Definer, attrs.Definer]](def, key, conf)
			default:
				panic("unknown relation type: " + typ.String())
			}

			if field != nil {
				_fields = append(_fields, field)
			}
		}

		if m.internals.base.proxy != nil {
			var (
				rootName         = m.internals.base.proxy.rootField.Name
				proxyDirectField = m.internals.base.proxy.directField
				proxyName        = proxyFieldName(rootName)

				// create a new plain proxy object to use as target in the relation
				rNewProxyObj = reflect.New(proxyDirectField.Type.Elem())
				newProxyObj  = rNewProxyObj.Interface().(attrs.Definer)
			)

			// add the proxy field to the model definitions
			_fields = append(_fields, newProxyField(
				m, def, rootName, proxyName,
				&ProxyFieldConfig{
					Proxy: newProxyObj,
				},
			))
		}

		m.internals.defs = attrs.Define(def, _fields...)
	}

	if tableName != "" && m.internals.defs.Table == "" {
		m.internals.defs.Table = tableName
	}

	return m.internals.defs
}

// GetQuerySet returns a new [queries.QuerySet] for the model.
//
// It automatically applies proxy joins if the model is a proxy model.
//
// The returned [queries.QuerySet] can be used to query the model's data
// and perform various operations like filtering, ordering, etc.
func (m *Model) GetQuerySet() *queries.QuerySet[attrs.Definer] {
	m.checkValid()

	var qs = queries.Objects(m.Object())

	if m.internals.base.proxy != nil {
		qs = qs.Scope(m.proxyScope)
	}

	return qs
}

// PK returns the primary key field of the model.
//
// If the model is not properly initialized it will panic.
//
// If the model does not have a primary key defined, it will return nil.
func (m *Model) PK() attrs.Field {
	m.checkValid()

	if m.internals.defs == nil {
		return nil
	}

	return m.internals.defs.Primary()
}

// RelatedField returns the related field by name.
//
// It checks if the model has definitions set up and if the field exists
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

// Object returns the object of the model.
//
// It checks if the model is properly initialized and if the object is set up,
// if the model is not properly set up, it will panic.
func (m *Model) Object() attrs.Definer {
	m.checkValid()
	return m.internals.object.Interface().(attrs.Definer)
}

// ModelMeta returns the model's metadata.
func (m *Model) ModelMeta() attrs.ModelMeta {
	m.checkValid()
	if m.internals.meta == nil {
		m.internals.meta = attrs.GetModelMeta(*m.internals.object)
	}
	return m.internals.meta
}

// Saved checks if the model is saved to the database.
// It checks if the model is properly initialized and if the model's definitions
// are set up. If the model is not initialized, it returns false.
// If the model is initialized, it checks if the model was loaded from the database
// or if the primary key field is set. If the primary key field is nil, it returns false.
// If the primary key field has a value, it returns true.
func (m *Model) Saved() bool {
	// if the model is not initialized, it is assumed
	// that it is not saved, so we return false
	if m.internals == nil ||
		m.internals.base == nil ||
		m.internals.object == nil {
		return false
	}

	// if the model was loaded from the database, it is saved
	if m.internals.fromDB {
		return true
	}

	// if the model has a nil primary key field,
	// we assume it is not saved.
	var pk = m.PK()
	if pk == nil {
		return false
	}

	var value, err = pk.Value()
	if err != nil {
		// if we cannot get the value of the primary key,
		// we assume it is not saved
		return false
	}

	return !attrs.IsZero(value)
}

// AfterQuery is called after a query is executed on the model.
//
// This is useful for setup after the model has been loaded from the database,
// such as setting the initial state of the model and marking it as loaded from the database.
func (m *Model) AfterQuery(_ *queries.GenericQuerySet) error {
	m.checkValid()
	m.setupInitialState()
	m.internals.fromDB = true
	return nil
}

// AfterSave is called after the model is saved to the database.
//
// This is useful for setup after the model has been saved to the database,
// such as setting the initial state of the model and marking it as loaded from the database.
func (m *Model) AfterSave(_ *queries.GenericQuerySet) error {
	m.checkValid()
	m.setupInitialState()
	m.internals.fromDB = true
	return nil
}

// If this model was the target end of a through relation,
// this method will set the through model for this model.
func (m *Model) SetThroughModel(throughModel attrs.Definer) {
	m.ThroughModel = throughModel
}

// Annotate adds annotations to the model.
// Annotations are key-value pairs that can be used to store additional
// information about the model, such as database annotations or custom data.
func (m *Model) Annotate(annotations map[string]any) {
	if m.Annotations == nil {
		m.Annotations = make(map[string]any)
	}

	maps.Copy(m.Annotations, annotations)
}

// DataStore returns the data store for the model.
//
// The data store is used to store model data like annotations, custom data, etc.
// If the data store is not initialized, it will be created.
func (m *Model) DataStore() queries.ModelDataStore {
	if m.data == nil {
		m.data = make(MapDataStore)
	}
	return m.data
}

// Save saves the model to the database.
//
// It checks if the model is properly initialized and if the model's definitions
// are set up. If the model is not initialized, it returns an error.
//
// If the model is initialized, it calls the SaveObject method on the model's
// object, passing the current context and a SaveConfig struct that contains
// the model's object, query set, fields to save, and a force flag.
//
// The object embedding the model can choose to implement the
// [canSaveObject] interface to provide a custom save implementation.
func (m *Model) Save(ctx context.Context) error {
	if m.internals != nil && m.internals.defs == nil && m.internals.object != nil {
		var obj = m.internals.object.Interface().(attrs.Definer)
		obj.FieldDefs()
	}

	if m.internals == nil || m.internals.object == nil {
		return fmt.Errorf(
			"cannot save object: %w: %w",
			query_errors.ErrNotImplemented, ErrModelInitialized,
		)
	}

	var this = m.internals.object.Interface().(attrs.Definer)
	return this.(CanSaveObject).SaveObject(ctx, SaveConfig{
		this: this,
	})
}

type SaveConfig struct {
	// this should not be nil, it is the object itself.
	//
	// If not provided, it will be set to the model's object inside of [Model.SaveObject].
	this attrs.Definer

	// A custom queryset to use for creating or updating the model.
	QuerySet *queries.QuerySet[attrs.Definer]

	// Fields to save, if empty, all fields will be saved.
	// If the model is not loaded from the database, all fields will be saved.
	Fields []string

	// Force indicates whether to force the save operation,
	// even if no fields have changed.
	Force bool
}

// SaveObject saves the model's object to the database.
//
// It checks if the model is properly initialized and if the model's definitions
// are set up. If the model is not initialized, it returns an error.
//
// If the model is initialized, it iterates over the model's fields and checks
// if any of the fields have changed. If any field has changed, it adds the field
// to the list of changed fields and prepares a queryset to save the model.
//
// A config struct [SaveConfig] is used to pass the model's object, queryset, fields to save,
// and a force flag to indicate whether to force the save operation.
func (m *Model) SaveObject(ctx context.Context, cnf SaveConfig) (err error) {
	if m.internals == nil || m.internals.defs == nil {
		return fmt.Errorf(
			"cannot save fields for %T: %w",
			m.internals.object.Interface(),
			ErrModelInitialized,
		)
	}

	// Setup the "this" object if not provided.
	if cnf.this == nil {
		cnf.this = m.internals.object.Interface().(attrs.Definer)
	}

	// check if anything has changed,
	if !m.internals.state.Changed(true) && m.internals.fromDB && !cnf.Force {
		// if nothing has changed, we can skip saving
		return nil
	}

	var fields = make(map[string]struct{}, len(cnf.Fields))
	for _, field := range cnf.Fields {
		fields[field] = struct{}{}
	}

	// if the model was not loaded from the database,
	// we automatically assume all changes are to be saved
	var anyChanges = !m.internals.fromDB
	var selectFields = make([]interface{}, 0)
	var updateFields = make([]attrs.Field, 0, m.internals.defs.ObjectFields.Len())
	for head := m.internals.defs.ObjectFields.Front(); head != nil; head = head.Next() {

		// if there was a list of fields provided and if
		// the field is not in the list of fields to save, we skip it

		//	fmt.Printf(
		//		"[SaveObject] Checking field %s in model %T, force: %v, fromDB: %v (%v) %v\n",
		//		head.Value.Name(), m.internals.object.Interface(),
		//		cnf.Force, m.internals.fromDB, head.Value.GetValue(), m.internals.state.HasChanged(head.Value.Name()),
		//	)

		var mustInclField bool
		if len(cnf.Fields) > 0 {
			if _, ok := fields[head.Value.Name()]; !ok && !cnf.Force && m.internals.fromDB {
				continue
			}
			mustInclField = true
		}

		var hasChanged = m.internals.state.HasChanged(head.Value.Name())
		if !hasChanged && !mustInclField && !cnf.Force && m.internals.fromDB {
			//	fmt.Printf(
			//		"[SaveObject] Field %s in model %T has not changed, skipping save: %v %v %v %v\n",
			//		head.Value.Name(), m.internals.object.Interface(),
			//		hasChanged, mustInclField, cnf.Force, m.internals.fromDB,
			//	)
			// if the field has not changed and none of the force flags are set,
			// we can skip saving this field
			continue
		}

		anyChanges = true
		updateFields = append(updateFields, head.Value)
	}

	// if no changes were made and the force flag is not set,
	// we can skip saving the model
	if (!anyChanges || len(updateFields) == 0) && !cnf.Force && len(cnf.Fields) == 0 && m.internals.fromDB {
		return nil
	}

	var transaction queries.Transaction
	if queries.CREATE_IMPLICIT_TRANSACTION {
		ctx, transaction, err = queries.StartTransaction(ctx)
		if err != nil {
			return fmt.Errorf(
				"failed to start transaction for model %T: %w",
				m.internals.object.Interface(), err,
			)
		}
	} else {
		transaction = queries.NullTransction()
	}
	defer transaction.Rollback()

	/*
		Save the model's proxy, if any.
	*/
	var proxy = m.proxy
	if proxy != nil && proxy.object != nil && proxy.object.internals.state.Changed(true) {
		err = proxy.object.Save(ctx)
		if err != nil {
			return fmt.Errorf(
				"failed to save proxy model %T: %w",
				proxy.object.internals.object.Interface(), err,
			)
		}
	}

	/*
		Save all model fields
	*/
	for _, field := range updateFields {
		anyChanges = true

		var err error
		switch fld := field.(type) {
		case models.Saver:
			panic(fmt.Errorf(
				"model %T field %s is a Saver, which is not supported in Save(), a ContextSaver is required to maintain transaction integrity",
				m.internals.object.Interface(), field.Name(),
			))
		case models.ContextSaver:
			err = fld.Save(ctx)
		case queries.SaveableField:
			err = fld.Save(ctx, cnf.this)
		}
		if err != nil {
			if !errors.Is(err, query_errors.ErrNotImplemented) {
				return fmt.Errorf(
					"failed to save field %q in model %T: %w",
					field.Name(), m.internals.object.Interface(), err,
				)
			}

			logger.Warnf(
				"field %q in model %T is not saveable, skipping: %v",
				field.Name(), m.internals.object.Interface(), err,
			)

			continue
		}

		// Add the field name to the list of changed fields.
		// This is used to determine which fields to save in the query set.
		selectFields = append(selectFields, field.Name())
	}

	/*
		Setup the query set to save the model.
	*/
	// Setup the query set if not provided.
	var querySet = cnf.QuerySet
	if querySet == nil {
		querySet = queries.
			Objects(cnf.this).
			Select(selectFields...).
			ExplicitSave()
	}

	// Add the context to the query set.
	querySet = querySet.
		WithContext(ctx)

	var updated int64
	var saved = m.Saved()
	if saved {
		updated, err = querySet.Update(cnf.this)
	} else {
		_, err = querySet.Create(cnf.this)
	}
	if err != nil {
		var s = "create"
		if saved {
			s = "update"
		}
		return fmt.Errorf(
			"failed to %s model %T: %w",
			s, m.internals.object.Interface(), err,
		)
	}

	if saved && updated == 0 {
		return fmt.Errorf(
			"model %T was not updated, no rows affected",
			m.internals.object.Interface(),
		)
	}

	// reset the state after saving
	m.internals.state.Reset()
	m.internals.fromDB = true

	return transaction.Commit()
}
