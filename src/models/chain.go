package models

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-signals"
)

const (
	_MODELS_CHAIN_KEY = "models.embed.chain"
)

var (
	_BASE_MODEL_PTR = reflect.TypeOf(&Model{})
	_MODEL_IFACE    = reflect.TypeOf((*_ModelInterface)(nil)).Elem()
	//	_BINDER_VALUE   = reflect.ValueOf((*attrs.Binder)(nil)).Elem()
)

type private struct{}

type _ModelInterface interface {
	__Model() private
}

var _, _ = attrs.OnBeforeModelRegister.Listen(func(s signals.Signal[attrs.SignalModelMeta], meta attrs.SignalModelMeta) error {
	var (
		rTyp       = reflect.TypeOf(meta.Definer)
		modelChain = buildModelChain(rTyp)
	)

	//modelChain.initChain(reflect.ValueOf(
	//	meta.Definer,
	//))

	attrs.StoreOnMeta(
		meta.Definer,
		_MODELS_CHAIN_KEY,
		modelChain,
	)

	return nil
})

var _, _ = attrs.OnModelRegister.Listen(func(s signals.Signal[attrs.SignalModelMeta], meta attrs.SignalModelMeta) error {
	return nil
})

type BaseModelProxy struct {
	// the field on the most top-level object that contains the proxy
	rootField *reflect.StructField
	// the field that directly contains the proxy
	// this field can define tags to control the proxy behavior
	// from the source model
	directField *reflect.StructField

	// The name of the content type for the object
	// which embeds this proxy.
	cTypeFieldName string

	// The name of the field in the target model
	// which contains the object that this proxy
	// is pointing to.
	targetFieldName string

	// the previous model in the chain, if any
	prev *BaseModelInfo
	// the next model in the chain, if any
	next *BaseModelInfo
}

type BaseModelInfo struct {
	// the proxy for this model, if any
	proxy *BaseModelProxy

	// the reference to the base model field
	// from the root of the current chain part
	//
	// if this is a proxy model, this field
	// can also contain information on
	// how to control the proxy behavior
	base reflect.StructField

	//	// fields which should be initialized
	//	// when the model's fields are defined
	//	initFields []reflect.StructField
}

func (b *BaseModelInfo) initChain(obj reflect.Value) {
	var curr = b
	if obj.Kind() == reflect.Ptr {
		obj = obj.Elem()
	}

	for curr != nil {
		if curr.proxy == nil {
			break
		}

		var field = obj.FieldByIndex(curr.proxy.rootField.Index)
		field.Set(reflect.ValueOf(attrs.NewObject[attrs.Definer](
			curr.proxy.rootField.Type,
		)))

		obj = field.Elem()
		curr = curr.proxy.next
	}
}

func embedsModel(rTyp reflect.Type) bool {
	if rTyp.Kind() == reflect.Ptr {
		rTyp = rTyp.Elem()
	}
	if rTyp.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < rTyp.NumField(); i++ {
		var field = rTyp.Field(i)
		switch {
		case isModelField(field):
			return true
		case isProxyField(field):
			if embedsModel(field.Type.Elem()) {
				return true
			}
		case field.Type.Kind() == reflect.Struct && reflect.PointerTo(field.Type).Implements(_MODEL_IFACE):
			if embedsModel(field.Type) {
				return true
			}
		}
	}
	return false
}

//
//	func isBinderField(field reflect.StructField) bool {
//
//	}

func isProxyField(field reflect.StructField) bool {
	return field.Type.Kind() == reflect.Ptr &&
		field.Type.Elem().Kind() == reflect.Struct &&
		(field.Tag.Get("proxy") == "true" || field.Anonymous && embedsModel(field.Type))
}

func isModelField(field reflect.StructField) bool {
	return field.Type == _BASE_MODEL_PTR.Elem()
}

func buildModelChain(rTyp reflect.Type) *BaseModelInfo {
	assert.True(
		rTyp.Kind() == reflect.Ptr && rTyp.Elem().Kind() == reflect.Struct,
		"definer must be a pointer to a struct, got %s", rTyp.Kind(),
	)

	rTyp = rTyp.Elem()

	var base *BaseModelInfo
	for i := 0; i < rTyp.NumField(); i++ {
		var field = rTyp.Field(i)
		assert.False(
			field.Type == _BASE_MODEL_PTR,
			"definer %s cannot embed a pointer to Model",
			rTyp.Name(),
		)

		switch {
		case isProxyField(field):
			assert.False(
				base == nil,
				"definer %s must embed a model before any proxy fields (%s)",
				rTyp.Name(), field.Name,
			)
			assert.True(
				base.proxy == nil,
				"definer %s cannot embed multiple proxy fields (%s)",
				rTyp.Name(), field.Name,
			)

			var ctypeFieldName = field.Tag.Get("ctype")
			var targetFieldName = field.Tag.Get("target")
			if (ctypeFieldName == "" || targetFieldName == "") && !field.Type.Implements(reflect.TypeOf((*canTargetDefiner)(nil)).Elem()) {
				panic(fmt.Sprintf(
					"proxy field %d in model %s must have 'ctype' and 'target' tags defined, got %q and %q",
					i, rTyp.Name(), ctypeFieldName, targetFieldName,
				))
			}

			base.proxy = &BaseModelProxy{
				prev:            base,
				rootField:       &field,
				directField:     &field,
				cTypeFieldName:  ctypeFieldName,
				targetFieldName: targetFieldName,
				next:            buildModelChain(field.Type),
			}

		case isModelField(field):
			assert.True(
				base == nil,
				"definer %s cannot embed multiple base models",
				rTyp.Name(),
			)
			base = &BaseModelInfo{
				base: field,
			}

		case field.Type.Kind() == reflect.Struct && reflect.PointerTo(field.Type).Implements(_MODEL_IFACE) && base == nil:
			var current = field.Type
		structLoop:
			for current.Kind() == reflect.Struct {
			fieldsLoop:
				for j := 0; j < current.NumField(); j++ {
					var subField = current.Field(j)
					switch {
					case subField.Type == _BASE_MODEL_PTR:
						assert.Fail(
							"definer %s cannot embed a pointer to Model, embed the struct directly",
							rTyp.Name(),
						)
					case isModelField(subField):
						assert.True(
							base == nil,
							"definer %s cannot embed multiple base models (%s)",
							rTyp.Name(), subField.Name,
						)
						field.Index = append(field.Index, subField.Index...)
						base = &BaseModelInfo{
							base: field,
						}
						break structLoop
					case isProxyField(subField):
						assert.False(
							base == nil,
							"definer %s must embed a model before any proxy fields (%s)",
							rTyp.Name(), subField.Name,
						)
						assert.True(
							base.proxy == nil,
							"definer %s cannot embed multiple proxy fields (%s)",
							rTyp.Name(), subField.Name,
						)

						var ctypeFieldName = subField.Tag.Get("ctype")
						var targetFieldName = subField.Tag.Get("target")
						if (ctypeFieldName == "" || targetFieldName == "") && !subField.Type.Implements(reflect.TypeOf((*canTargetDefiner)(nil)).Elem()) {
							panic(fmt.Sprintf(
								"proxy field %d in model %s must have 'ctype' and 'target' tags defined, got %q and %q",
								i, rTyp.Name(), ctypeFieldName, targetFieldName,
							))
						}

						field.Index = append(field.Index, subField.Index...)
						base.proxy = &BaseModelProxy{
							rootField:       &field,
							directField:     &subField,
							prev:            base,
							cTypeFieldName:  ctypeFieldName,
							targetFieldName: targetFieldName,
							next:            buildModelChain(subField.Type),
						}
						break structLoop
					case subField.Type.Kind() == reflect.Struct && reflect.PointerTo(subField.Type).Implements(_MODEL_IFACE):
						field.Index = append(field.Index, subField.Index...)
						current = subField.Type
						continue structLoop
					default:
						continue fieldsLoop
					}
				}
			}

		default:
			continue
		}
	}
	return base
}

func getModelChain(def attrs.Definer) *BaseModelInfo {
	if !attrs.IsModelRegistered(def) {
		return buildModelChain(reflect.TypeOf(def))
	}
	var meta = attrs.GetModelMeta(def)
	var chainObj, ok = meta.Storage(_MODELS_CHAIN_KEY)
	if !ok {
		return nil
	}
	return chainObj.(*BaseModelInfo)
}
