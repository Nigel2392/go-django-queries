package queries

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/elliotchance/orderedmap/v2"
)

type objectRelation struct {
	relTyp  attrs.RelationType
	objects *orderedmap.OrderedMap[any, *object]
}

type object struct {
	// through is a possible through model for the relation
	through attrs.Definer

	// the primary key of the object
	pk any

	// the field defs of the object
	fieldDefs attrs.Definitions

	// The object itself, which is a Definer
	obj attrs.Definer

	// the direct relations of the object (multiple)
	relations map[string]*objectRelation
}

type rootObject struct {
	object      *object
	annotations map[string]any // Annotations for the root object
}

type rows[T attrs.Definer] struct {
	objects *orderedmap.OrderedMap[any, *rootObject]
}

func newRows[T attrs.Definer]() *rows[T] {
	return &rows[T]{
		objects: orderedmap.NewOrderedMap[any, *rootObject](),
	}
}

func (r *rows[T]) addRoot(pk any, obj attrs.Definer, annotations map[string]any) *rootObject {
	if pk == nil {
		panic("cannot add root object with nil primary key")
	}

	if root, ok := r.objects.Get(pk); ok {
		return root
	}

	var defs attrs.Definitions
	if obj != nil {
		defs = obj.FieldDefs()
	}

	var root = &rootObject{
		object: &object{
			pk:        pk,
			obj:       obj,
			fieldDefs: defs,
			relations: make(map[string]*objectRelation),
		},
		annotations: annotations,
	}

	r.objects.Set(pk, root)
	return root
}

func (r *rows[T]) addRelationChain(chain []chainPart) {

	var root = chain[0]
	var obj, ok = r.objects.Get(root.pk)
	if !ok {
		panic(fmt.Sprintf("root object with primary key %v not found in rows, root needs to be added with rows.addRoot", root.pk))
	}
	var current = obj.object
	var idx = 1
	for idx < len(chain) {
		var part = chain[idx]

		// If the primary key is zero and the relation is not a ManyToOne or OneToOne,
		// we can stop traversing the chain, as there is no data for this relation
		//
		// This is to exclude empty rows in the result set when querying multiple- valued relations.
		//
		// ManyToOne and OneToOne relations are special cases where the primary key can be zero.
		//
		// This also means that any deeper relations cannot be traversed, I.E. we break the loop.
		if fields.IsZero(part.pk) && !(part.relTyp == attrs.RelManyToOne || part.relTyp == attrs.RelOneToOne) {
			break
		}

		var next, ok = current.relations[part.chain]
		if !ok {
			next = &objectRelation{
				relTyp:  part.relTyp,
				objects: orderedmap.NewOrderedMap[any, *object](),
			}
			current.relations[part.chain] = next
		}

		child, ok := next.objects.Get(part.pk)
		if !ok {

			var through attrs.Definer
			if part.through != nil {
				// If there is a through object, we need to set it
				through = part.through
			}

			child = &object{
				pk:        part.pk,
				fieldDefs: part.object.FieldDefs(),
				obj:       part.object,
				relations: make(map[string]*objectRelation),
				through:   through,
			}

			next.objects.Set(part.pk, child)
		}

		current = child
		idx++
	}
}

func (r *rows[T]) compile() []*Row[T] {
	var addRelations func(*object)
	addRelations = func(obj *object) {

		if obj.pk == nil {
			panic(fmt.Sprintf("object %T has no primary key, cannot deduplicate relations", obj.obj))
		}

		for relName, rel := range obj.relations {
			if rel.objects.Len() == 0 {
				continue
			}

			var relatedObjects = make([]Relation, 0, rel.objects.Len())
			for relHead := rel.objects.Front(); relHead != nil; relHead = relHead.Next() {
				var relatedObj = relHead.Value
				if relatedObj == nil {
					continue
				}

				addRelations(relatedObj)

				var throughObj attrs.Definer
				if relatedObj.through != nil {
					// If there is a through object, we need to add it to the related objects
					throughObj = relatedObj.through
				}

				relatedObjects = append(relatedObjects, &baseRelation{
					object:  relatedObj.obj,
					through: throughObj,
				})
			}

			if len(relatedObjects) > 0 {
				setRelatedObjects(relName, rel.relTyp, obj.obj, relatedObjects)
			}
		}
	}

	var root = make([]*Row[T], 0, r.objects.Len())
	for head := r.objects.Front(); head != nil; head = head.Next() {
		var obj = head.Value
		if obj == nil {
			continue
		}

		addRelations(obj.object)

		var definer = obj.object.obj
		if definer == nil {
			continue
		}

		root = append(root, &Row[T]{
			Object:      definer.(T),
			Annotations: obj.annotations,
		})
	}
	return root
}

func newSettableRelation[T any](typ reflect.Type) T {
	var setterTyp = typ
	if setterTyp.Kind() == reflect.Ptr {
		setterTyp = setterTyp.Elem()
	}

	if setterTyp.Kind() == reflect.Slice {
		var n = reflect.MakeSlice(setterTyp, 0, 0)
		var sliceVal = n.Interface()
		if n.Type().Implements(reflect.TypeOf((*T)(nil)).Elem()) {
			return sliceVal.(T)
		}
		return n.Addr().Interface().(T)
	}

	var newVal = reflect.New(setterTyp)
	if newVal.Type().Implements(reflect.TypeOf((*T)(nil)).Elem()) {
		return newVal.Interface().(T)
	}

	return newVal.Addr().Interface().(T)
}

func setRelatedObjects(relName string, relTyp attrs.RelationType, obj attrs.Definer, relatedObjects []Relation) {

	var fieldDefs = obj.FieldDefs()
	var field, ok = fieldDefs.Field(relName)
	if !ok {
		panic(fmt.Sprintf("relation %s not found in field defs of %T", relName, obj))
	}

	switch relTyp {
	case attrs.RelManyToOne:
		// handle foreign keys
		//
		// no through model is expected
		if len(relatedObjects) > 1 {
			panic(fmt.Sprintf("expected at most one related object for %s, got %d", relName, len(relatedObjects)))
		}

		var relatedObject attrs.Definer
		if len(relatedObjects) > 0 {
			relatedObject = relatedObjects[0].Model()
		}

		field.SetValue(relatedObject, true)

	case attrs.RelOneToMany:
		// handle reverse foreign keys
		//
		// a through model is not expected
		var related = make([]attrs.Definer, len(relatedObjects))
		for i, relatedObj := range relatedObjects {
			related[i] = relatedObj.Model()
		}

		if dm, ok := obj.(DataModel); ok {
			dm.ModelDataStore().SetValue(relName, related)
		}

		var typ = field.Type()
		switch {
		case typ.Kind() == reflect.Slice:
			// If the field is a slice, we can set the related objects directly
			var slice = reflect.MakeSlice(typ, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				slice.Index(i).Set(reflect.ValueOf(relatedObj.Model()))
			}
			field.SetValue(slice.Interface(), true)

		default:
			panic(fmt.Sprintf("expected field %s to be a slice, got %s", relName, typ))
		}

	case attrs.RelOneToOne:
		// handle one-to-one relations
		//
		// a through model COULD BE expected, but it is not required
		if len(relatedObjects) > 1 {
			panic(fmt.Sprintf("expected at most one related object for %s, got %d", relName, len(relatedObjects)))
		}

		var relatedObject Relation
		if len(relatedObjects) > 0 {
			relatedObject = relatedObjects[0]
		}

		switch {
		case field.Type().Implements(reflect.TypeOf((*SettableThroughRelation)(nil)).Elem()):
			// If the field is a SettableThroughRelation, we can set the related object directly
			var value = field.GetValue()
			if value == nil {
				value = newSettableRelation[SettableThroughRelation](field.Type())
				field.SetValue(value, true)
			}
			var rel = value.(SettableThroughRelation)
			rel.SetValue(relatedObject.Model(), relatedObject.Through())

		case field.Type().Implements(reflect.TypeOf((*Relation)(nil)).Elem()):
			// If the field is a Relation, we can set the related object directly
			field.SetValue(relatedObject, true)

		case field.Type().Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
			// If the field is a Definer, we can set the related object directly
			field.SetValue(relatedObject.Model(), true)

		default:
			panic(fmt.Sprintf("expected field %s to be a Relation or Definer, got %s", relName, field.Type()))

		}

	case attrs.RelManyToMany:
		// handle many-to-many relations
		//
		// a through model is expected
		if dm, ok := obj.(DataModel); ok {
			dm.ModelDataStore().SetValue(relName, relatedObjects)
		}

		var typ = field.Type()
		switch {
		case typ.Implements(reflect.TypeOf((*SettableMultiThroughRelation)(nil)).Elem()):
			// If the field is a SettableMultiRelation, we can set the related objects directly
			var value = field.GetValue()
			if value == nil {
				value = newSettableRelation[SettableMultiThroughRelation](typ)
				field.SetValue(value, true)
			}
			var rel = value.(SettableMultiThroughRelation)
			// Set the related objects
			rel.SetValues(relatedObjects)

		case typ.Kind() == reflect.Slice && typ.Elem().Implements(reflect.TypeOf((*Relation)(nil)).Elem()):
			// If the field is a slice, we can set the related objects directly
			var slice = reflect.MakeSlice(typ, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				slice.Index(i).Set(reflect.ValueOf(relatedObj))
			}
			fieldDefs.Set(relName, slice.Interface())

		case typ.Kind() == reflect.Slice && typ.Elem().Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
			// If the field is a slice of Definer, we can set the related objects directly
			var slice = reflect.MakeSlice(typ, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				var relatedDefiner = relatedObj.Model()
				slice.Index(i).Set(reflect.ValueOf(relatedDefiner))
			}
			fieldDefs.Set(relName, slice.Interface())

		default:
			panic(fmt.Sprintf("expected field %s to be a slice, got %s", relName, typ))
		}
	default:
		panic(fmt.Sprintf("unknown relation type %s for field %s in %T", relTyp, relName, obj))
	}
}

type chainPart struct {
	relTyp  attrs.RelationType // The relation type of the field, if any
	chain   string
	pk      any
	object  attrs.Definer
	through attrs.Definer
}

func buildChainParts(actualField *scannableField) []chainPart {
	// Get the stack of fields from target to parent
	var stack = make([]chainPart, 0)
	for cur := actualField; cur != nil; cur = cur.srcField {
		var (
			inst    = cur.field.Instance()
			defs    = inst.FieldDefs()
			primary = defs.Primary()
		)

		stack = append(stack, chainPart{
			relTyp:  cur.relType,
			chain:   cur.chainPart,
			pk:      primary.GetValue(),
			object:  inst,
			through: cur.through,
		})
	}

	// Reverse the stack to get the fields in the correct order
	// i.e. parent to target
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}

	return stack
}
