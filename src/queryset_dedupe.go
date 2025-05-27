package queries

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/elliotchance/orderedmap/v2"
	"github.com/pkg/errors"
)

// objectRelation contains metadata about the list of related objects and
// the relation type itself.
type objectRelation struct {
	relTyp  attrs.RelationType
	objects *orderedmap.OrderedMap[any, *object]
}

// An object is a representation of a model instance in the rows structure.
//
// It contains the primary key, the field definitions, and the relations of the object.
//
// Any relations stored on this object are directly related to the object itself,
// if a through model is used, it is stored in the `through` field.
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

// the rootObject provides the top layer of the [rows] structure.
//
// It contains the object itself, and any annotations that are associated with it.
type rootObject struct {
	object      *object
	annotations map[string]any // Annotations for the root object
}

// rows represents a collection of root objects.
//
// each of those root objects can have multiple relations to other objects,
// which are stored in the [object] struct.
//
// The rows structure is used to deduplicate relations and compile the final result set.
//
// for deduplication of multi- valued relations, the primary key of the parent and child objects
// have to be set, otherwise the relation cannot be deduplicated.
type rows[T attrs.Definer] struct {
	objects *orderedmap.OrderedMap[any, *rootObject]
	forEach func(attrs.Definer) error
}

// addRoot adds a root object to the rows structure.
//
// this is used to add the top-level object to the rows,
// which can then have relations added to it.
//
// it has to be called before any relations are added - technically
// root objects can be added inside of the [addRelationChain] method,
// but this would lose any annotations that are associated with the root object.
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

// addRelationChain adds a relation chain to the rows structure.
//
// it is used to add relations to the root object, or any other object in the rows structure.
// the chain is a list of [chainPart] that contains the relation type, primary key, and the object itself.
//
// the root object has to be added with [addRoot] before this method is called,
// otherwise it will panic.
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

func (r *rows[T]) compile() ([]*Row[T], error) {
	var addRelations func(*object, uint64) error
	// addRelations is a recursive function that traverses the object and its relations,
	// and sets the related objects on the provided parent object.
	addRelations = func(obj *object, depth uint64) error {

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

				if err := addRelations(relatedObj, depth+1); err != nil {
					return fmt.Errorf("object %T: %w", obj, err)
				}

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

			// if the object has related objects we need to set them on the parent object
			if len(relatedObjects) > 0 {
				setRelatedObjects(relName, rel.relTyp, obj.obj, relatedObjects)
			}
		}

		if r.forEach != nil {
			if obj.through != nil {
				// Call the forEach function with the through object if it exists
				if err := r.forEach(obj.through); err != nil {
					return fmt.Errorf("error in forEach[%d] for through object %T: %w", depth, obj.through, err)
				}
			}

			// If a forEach function is set, we call it for each object
			if err := r.forEach(obj.obj); err != nil {
				return fmt.Errorf("error in forEach[%d] for object %T: %w", depth, obj.obj, err)
			}
		}

		return nil
	}

	var root = make([]*Row[T], 0, r.objects.Len())
	for head := r.objects.Front(); head != nil; head = head.Next() {
		var obj = head.Value
		if obj == nil {
			continue
		}

		if err := addRelations(obj.object, 0); err != nil {
			return nil, fmt.Errorf("failed to add relations for object with primary key %v: %w", obj.object.pk, err)
		}

		var definer = obj.object.obj
		if definer == nil {
			continue
		}

		root = append(root, &Row[T]{
			Object:      definer.(T),
			Annotations: obj.annotations,
		})
	}

	return root, nil
}

// newSettableRelation creates a new instance of a SettableRelation or SettableMultiThroughRelation.
//
// It checks if the type is a slice or a pointer, and returns a new instance of the appropriate type.
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

// setRelatedObjects sets the related objects for the given relation name and type.
//
// it provides a uniform way to set related objects on a model instance,
// allowing to handle different relation types and through models.
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
		case typ == reflect.TypeOf(related):
			// If the field is a slice of Definer, we can set the related objects directly
			field.SetValue(related, true)

		case typ.Kind() == reflect.Slice:
			// If the field is a slice, we can set the related objects directly after
			// converting them to the appropriate type.
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

// walkFields traverses the fields of an object based on a chain of field names.
//
// It yields each field found at the last depth of the chain, allowing for
// custom processing of the field (e.g., collecting values).
func walkFields(obj attrs.Definitions, chain []string, idx *int, depth int, yield func(idx int, field attrs.Field) bool) error {

	if depth > len(chain)-1 {
		return fmt.Errorf("depth %d exceeds chain length %d: %w", depth, len(chain), query_errors.ErrFieldNotFound)
	}

	var fieldName = chain[depth]
	var field, ok = obj.Field(fieldName)
	if !ok {
		return fmt.Errorf("field %s not found in object %T: %w", fieldName, obj, query_errors.ErrFieldNotFound)
	}

	if depth == len(chain)-1 {
		if !yield(*idx, field) {
			return fmt.Errorf("stopped yielding at field %s in object %T: %w", fieldName, obj, errStopIteration)
		}
		*idx++     // Increment index for the next field found
		return nil // Found the field at the last depth
	}

	var value = field.GetValue()
	if value == nil {
		return query_errors.ErrNilPointer
	}

	var rTyp = reflect.TypeOf(value)

	switch {
	case rTyp.Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
		// If the field is a Definer, we can walk its fields
		var definer = value.(attrs.Definer).FieldDefs()
		if err := walkFields(definer, chain, idx, depth+1, yield); err != nil {
			if errors.Is(err, query_errors.ErrNilPointer) {
				return nil // Skip nil pointers
			}
			return fmt.Errorf("%s: %w", fieldName, err)
		}
	case rTyp.Kind() == reflect.Slice && rTyp.Elem().Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
		// If the field is a slice of Definer, we can walk its fields
		var slice = reflect.ValueOf(value)
		for i := 0; i < slice.Len(); i++ {
			var elem = slice.Index(i).Interface()
			if elem == nil {
				continue // Skip nil elements
			}
			if err := walkFields(elem.(attrs.Definer).FieldDefs(), chain, idx, depth+1, yield); err != nil {
				if errors.Is(err, query_errors.ErrNilPointer) {
					continue // Skip elements where the field is nil
				}
				return fmt.Errorf("%s[%d]: %w", fieldName, i, err)
			}
		}
	case rTyp.Kind() == reflect.Slice && rTyp.Elem().Implements(reflect.TypeOf((*Relation)(nil)).Elem()):
		// If the field is a slice of Relation, we can walk its fields
		var slice = reflect.ValueOf(value)
		for i := 0; i < slice.Len(); i++ {
			var elem = slice.Index(i).Interface()
			if elem == nil {
				continue // Skip nil elements
			}
			var rel = elem.(Relation)
			if rel.Model() == nil {
				continue // Skip relations with nil model
			}
			if err := walkFields(rel.Model().FieldDefs(), chain, idx, depth+1, yield); err != nil {
				if errors.Is(err, query_errors.ErrNilPointer) {
					continue // Skip elements where the field is nil
				}
				return fmt.Errorf("%s[%d]: %w", fieldName, i, err)
			}
		}
	default:
		return fmt.Errorf("expected field %s in object %T to be a Definer, slice of Definer, or slice of Relation, got %s", fieldName, obj, rTyp)
	}
	return nil
}

// a chainPart represents a part of a relation chain.
// it contains information about the relation and object.
type chainPart struct {
	relTyp  attrs.RelationType
	chain   string
	pk      any
	object  attrs.Definer
	through attrs.Definer
}

// buildChainParts builds a chain of parts from the actual field to the parent field.
//
// It traverses the scannableField structure and collects the relation type, primary key,
// object, and through model for each part of the chain.
//
// The [getScannableFields] function builds this chain of *scannableField objects,
// which represent the fields that can be scanned from the database.
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
