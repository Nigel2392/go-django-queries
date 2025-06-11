package models

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ProxyChain struct {
	root               *ProxyChain
	prev               *ProxyChain
	next               *ProxyChain
	rootEmbeddingField *reflect.StructField
	embeddingField     *reflect.StructField
	model              *Model
	fieldDefs          attrs.Definitions
	object             attrs.Definer
	targetCtypeField   attrs.FieldDefinition
	targetPrimaryField attrs.FieldDefinition
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

func (p *ProxyChain) TargetContentTypeField() attrs.FieldDefinition {
	return p.targetCtypeField
}

func (p *ProxyChain) TargetPrimaryField() attrs.FieldDefinition {
	return p.targetPrimaryField
}

func proxyFieldName(name string) string {
	// return fmt.Sprintf("__%s__proxy", name)
	return "__PROXY"
}

func (p *ProxyChain) ProxyField() attrs.FieldDefinition {
	if p.embeddingField == nil {
		panic(fmt.Sprintf(
			"embedding field is nil in model %T, this should not happen",
			p.object,
		))
	}

	if p.model.proxy == nil {
		panic(fmt.Errorf("Proxy is nil for %T", p.model.internals.object.Interface())) //lint:ignore ST1005 this is a type
	}

	if p.next == nil {
		return nil
	}

	var fieldName = proxyFieldName(p.next.rootEmbeddingField.Name)
	var fieldDef, ok = p.fieldDefs.Field(fieldName)
	if !ok {
		panic(fmt.Sprintf(
			"embedding field %q not found in field definitions of model %T",
			fieldName, p.object,
		))
	}

	return fieldDef
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
			model:     rootModel,
			object:    obj,
			fieldDefs: obj.FieldDefs(),
			rootEmbeddingField: copyStructField(
				&base.base,
			),
			embeddingField: copyStructField(
				&base.base,
			),
		}
	}

	var root = &ProxyChain{
		model:     rootModel,
		object:    obj,
		fieldDefs: obj.FieldDefs(),
		rootEmbeddingField: copyStructField(
			&base.base,
		),
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

		var (
			targetCtypeField   attrs.FieldDefinition
			targetPrimaryField attrs.FieldDefinition
		)

		if targetDefiner, ok := obj.(CanTargetDefiner); ok {
			targetCtypeField = targetDefiner.TargetContentTypeField()
			targetPrimaryField = targetDefiner.TargetPrimaryField()
		} else {
			targetCtypeField, ok = chain.fieldDefs.Field(currTyp.cTypeFieldName)
			if !ok {
				panic(fmt.Sprintf(
					"proxy field %d in model %T does not have a field with ctype name %q",
					i, rootModel, currTyp.cTypeFieldName,
				))
			}

			targetPrimaryField, ok = chain.fieldDefs.Field(currTyp.targetFieldName)
			if !ok {
				panic(fmt.Sprintf(
					"proxy field %d in model %T does not have a field with target name %q",
					i, rootModel, currTyp.targetFieldName,
				))
			}
		}

		chain.next = &ProxyChain{
			prev:               chain,
			root:               root,
			model:              model,
			object:             obj,
			fieldDefs:          fieldDefs,
			targetCtypeField:   targetCtypeField,
			targetPrimaryField: targetPrimaryField,
			rootEmbeddingField: copyStructField(
				currTyp.rootField,
			),
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
