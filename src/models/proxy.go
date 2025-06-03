package models

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type EmbedModelOptions struct {
	// AutoInit indicates whether the pointer to the model should be automatically initialized
	// If AutoInit is true, the model will be initialized to a new instance
	// If it is false, the model will be left as nil and no fields will be embedded.
	AutoInit bool

	// EmbedFields specifies which fields of the model should be embedded
	// If EmbedFields is nil, all fields of the model will be embedded.
	EmbedFields []any
}

func EmbedProxyModel(nameOrScan any, options ...EmbedModelOptions) func(d attrs.Definer) []attrs.Field {
	var opts EmbedModelOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return func(d attrs.Definer) []attrs.Field {
		var (
			rTyp = reflect.TypeOf(d).Elem()
			rVal = reflect.ValueOf(d).Elem()
		)

		var fieldval reflect.Value
		switch v := nameOrScan.(type) {
		case string:
			var fieldTyp, ok = rTyp.FieldByName(v)
			assert.True(ok, "field %q not found in %T", v, d)

			fieldval = rVal.FieldByIndex(fieldTyp.Index)
			assert.True(
				fieldval.Kind() == reflect.Ptr && fieldval.Type().Elem().Kind() == reflect.Struct,
				"field %q in %T must be a pointer to a struct, got %s", v, d, fieldval.Kind(),
			)
			assert.True(
				fieldval.CanSet(),
				"field %q in %T must be settable, got %s", v, d, fieldval.Kind(),
			)
		case attrs.Definer:
			fieldval = reflect.ValueOf(v)
			nameOrScan = reflect.TypeOf(v).Elem().Name()
		default:
			assert.Fail("nameOrScan must be a string or attrs.Definer, got %T", v)
		}

		if fieldval.IsNil() {
			if opts.AutoInit {
				var newVal = reflect.New(fieldval.Type().Elem())
				fieldval.Set(newVal)
			} else {
				return []attrs.Field{} // no fields to embed
			}
		}

		definer, ok := fieldval.Interface().(attrs.Definer)
		assert.True(ok, "field %q in %T must implement attrs.Definer, got %T", nameOrScan, d, fieldval.Interface())

		if len(opts.EmbedFields) > 0 {
			var fields, err = attrs.UnpackFieldsFromArgs(definer, opts.EmbedFields...)
			assert.True(err == nil, "failed to unpack fields: %v", err)
			return fields
		}

		return definer.FieldDefs().Fields()
	}
}

type ProxyChain struct {
	root           *ProxyChain
	next           *ProxyChain
	embeddingField *reflect.StructField
	model          *Model
	object         attrs.Definer
}

func (p *ProxyChain) Root() *ProxyChain {
	return p.root
}

func (p *ProxyChain) Next() *ProxyChain {
	return p.next
}

func (p *ProxyChain) Model() *Model {
	return p.model
}

func (p *ProxyChain) Object() attrs.Definer {
	return p.object
}

func (p *ProxyChain) EmbeddingField() *reflect.StructField {
	return copyStructField(p.embeddingField)
}

func copyStructField(field *reflect.StructField) *reflect.StructField {
	if field == nil {
		return nil
	}
	return &reflect.StructField{
		Name:      field.Name,
		PkgPath:   field.PkgPath,
		Type:      field.Type,
		Tag:       field.Tag,
		Offset:    field.Offset,
		Index:     slices.Clone(field.Index),
		Anonymous: field.Anonymous,
	}
}

func NewProxyChain(obj attrs.Definer, createNew bool) *ProxyChain {
	var rootObject = reflect.ValueOf(obj)
	if rootObject.Kind() != reflect.Ptr || !rootObject.IsValid() {
		panic(fmt.Sprintf(
			"object %T must be a pointer to a struct, got %s",
			obj, rootObject.Kind(),
		))
	}

	if createNew {
		rootObject = reflect.New(rootObject.Type().Elem())
		obj = rootObject.Interface().(attrs.Definer)
	}

	var (
		base       = getModelChain(obj)
		currTyp    = base.proxy
		currObj    = rootObject.Elem()
		modelValue = currObj.FieldByIndex(
			base.base.Index,
		)
		rootModel = modelValue.Addr().Interface().(*Model)
	)

	if err := rootModel.Setup(obj); err != nil {
		panic(fmt.Sprintf(
			"failed to setup model %T from object %T: %v",
			rootModel, obj, err,
		))
	}

	if currTyp == nil {
		return &ProxyChain{
			model:  rootModel,
			object: obj,
			embeddingField: copyStructField(
				&base.base,
			),
		}
	}

	var root = &ProxyChain{
		model:  rootModel,
		object: obj,
		embeddingField: copyStructField(
			&base.base,
		),
	}
	var chain = root
	var i = 0
	for currTyp != nil {
		if currTyp.rootField == nil {
			panic(fmt.Sprintf(
				"proxy field %d in model %T is nil, this should not happen",
				i, rootModel,
			))
		}

		var field = currObj.FieldByIndex(currTyp.rootField.Index)
		if !field.IsValid() {
			panic(fmt.Sprintf(
				"proxy field %d in model %T is not valid, this should not happen",
				i, rootModel,
			))
		}

		if field.IsNil() {
			// var newV = reflect.ValueOf(attrs.NewObject[attrs.Definer](currTyp.rootField.Type))
			// field.Set(newV)
			var newV = reflect.New(currTyp.rootField.Type.Elem())
			field.Set(newV)
		}

		var objVal = field
		if field.Kind() == reflect.Ptr {
			field = field.Elem()
		}

		var modelVal = field.FieldByIndex(currTyp.next.base.Index)
		var model = modelVal.Addr().Interface().(*Model)
		var obj = objVal.Interface().(attrs.Definer)

		if err := model.Setup(obj); err != nil {
			panic(fmt.Sprintf(
				"failed to setup model %T from proxy field %d in model %T: %v",
				obj, i, rootModel, err,
			))
		}

		var fieldDefs = obj.FieldDefs()
		if reflect.TypeOf(fieldDefs.Instance()) != objVal.Type() {
			panic(fmt.Sprintf(
				"proxy field %d in model %T has a different FieldDefs() instance type than the object %T",
				i, rootModel, obj,
			))
		}

		chain.next = &ProxyChain{
			root:   root,
			model:  model,
			object: obj,
			embeddingField: copyStructField(
				currTyp.directField,
			),
		}

		if currTyp.next == nil {
			break
		}

		currObj = field
		currTyp = currTyp.next.proxy
		chain = chain.next
		i++
	}

	return root
}
