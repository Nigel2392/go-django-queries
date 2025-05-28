package queries

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/elliotchance/orderedmap/v2"
	"github.com/pkg/errors"
)

var (
	_ Relation                  = (*baseRelation)(nil)
	_ Relation                  = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ ThroughRelationValue      = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ MultiThroughRelationValue = (*RelM2M[attrs.Definer, attrs.Definer])(nil)
)

// A base relation type that implements the Relation interface.
//
// It is used to set the related object and it's through object on a model.
type baseRelation struct {
	pk      any
	object  attrs.Definer
	through attrs.Definer
}

func (r *baseRelation) PrimaryKey() any {
	if r == nil {
		return nil
	}
	return r.pk
}

func (r *baseRelation) Model() attrs.Definer {
	return r.object
}

func (r *baseRelation) Through() attrs.Definer {
	return r.through
}

// A value which can be used on models to represent a One-to-One relation
// with a through model.
//
// This implements the [SettableThroughRelation] interface, which allows setting
// the related object and its through object.
type RelO2O[ModelType, ThroughModelType attrs.Definer] struct {
	Parent        *ParentInfo // The parent model instance
	Object        ModelType
	ThroughObject ThroughModelType
}

func (rl *RelO2O[T1, T2]) BindToObject(parent *ParentInfo) error {
	if rl == nil {
		return nil
	}
	rl.Parent = parent
	return nil
}

func (rl *RelO2O[T1, T2]) Model() attrs.Definer {
	return rl.Object
}

func (rl *RelO2O[T1, T2]) Through() attrs.Definer {
	return rl.ThroughObject
}

func (rl *RelO2O[T1, T2]) SetValue(instance attrs.Definer, through attrs.Definer) {
	if instance != nil {
		rl.Object = instance.(T1)
	}
	if through != nil {
		rl.ThroughObject = through.(T2)
	}
}

func (rl *RelO2O[T1, T2]) GetValue() (obj attrs.Definer, through attrs.Definer) {
	if rl == nil {
		return nil, nil
	}
	return rl.Object, rl.ThroughObject
}

// A value which can be used on models to represent a Many-to-Many relation
// with a through model.
//
// This implements the [SettableMultiThroughRelation] interface, which allows setting
// the related objects and their through objects.
type RelM2M[T1, T2 attrs.Definer] struct {
	Parent    *ParentInfo                                 // The parent model instance
	relations *orderedmap.OrderedMap[any, RelO2O[T1, T2]] // can be changed to slice if needed
	// relations []RelO2O[T1, T2] // can be changed to OrderedMap if needed
}

func (rl *RelM2M[T1, T2]) BindToObject(parent *ParentInfo) error {
	if rl == nil {
		return nil
	}
	rl.Parent = parent
	return nil
}

func (rl *RelM2M[T1, T2]) SetValues(rel []Relation) {
	if len(rel) == 0 {
		rl.relations = orderedmap.NewOrderedMap[any, RelO2O[T1, T2]]()
		// rl.relations = make([]RelO2O[T1, T2], 0)
		return
	}

	rl.relations = orderedmap.NewOrderedMap[any, RelO2O[T1, T2]]()
	// rl.relations = make([]RelO2O[T1, T2], 0, len(rel))
	for _, r := range rel {
		if r == nil {
			continue
		}
		var o2o = RelO2O[T1, T2]{
			Object:        r.Model().(T1),
			ThroughObject: r.Through().(T2),
		}

		// rl.relations = append(rl.relations, o2o)
		//
		var pkValue any
		if canPk, ok := r.(interface{ PrimaryKey() any }); ok {
			pkValue = canPk.PrimaryKey()
		} else {
			var objDefs = o2o.Object.FieldDefs()
			var pkField = objDefs.Primary()
			pkValue = pkField.GetValue()
		}

		if pkValue == nil {
			panic(fmt.Sprintf("cannot set related object %T with nil primary key", o2o.Object))
		}

		rl.relations.Set(pkValue, o2o)
	}
}

// GetValues returns the related objects and their through objects.
func (rl *RelM2M[T1, T2]) GetValues() []Relation {
	if rl == nil || rl.relations == nil {
		return nil
	}
	// var relatedObjects = make([]Relation, len(rl.relations))
	// for i, rel := range rl.relations {
	// relatedObjects[i] = &rel
	// }
	// return relatedObjects
	var relatedObjects = make([]Relation, 0, rl.relations.Len())
	for relHead := rl.relations.Front(); relHead != nil; relHead = relHead.Next() {
		relatedObjects = append(relatedObjects, &relHead.Value)
	}
	return relatedObjects
}

func (rl *RelM2M[T1, T2]) Objects() []RelO2O[T1, T2] {
	if rl == nil || rl.relations == nil {
		return nil
	}

	var relatedObjects = make([]RelO2O[T1, T2], 0, rl.relations.Len())
	for relHead := rl.relations.Front(); relHead != nil; relHead = relHead.Next() {
		relatedObjects = append(relatedObjects, relHead.Value)
	}
	return relatedObjects
}

func (rl *RelM2M[T1, T2]) Len() int {
	if rl == nil || rl.relations == nil {
		return 0
	}
	// return len(rl.relations)
	return rl.relations.Len()
}

// setRelatedObjects sets the related objects for the given relation name and type.
//
// it provides a uniform way to set related objects on a model instance,
// allowing to handle different relation types and through models.
//
// used in [rows.compile] to set the related objects on the parent object.
func setRelatedObjects(relName string, relTyp attrs.RelationType, obj attrs.Definer, relatedObjects []Relation) {

	var fieldDefs = obj.FieldDefs()
	var field, ok = fieldDefs.Field(relName)
	if !ok {
		panic(fmt.Sprintf("relation %s not found in field defs of %T", relName, obj))
	}

	var (
		fieldType  = field.Type()
		fieldValue = field.GetValue()
	)
	switch {
	case fieldType.Implements(reflect.TypeOf((*BindableRelationValue)(nil)).Elem()):
		if fieldValue == nil {
			fieldValue = newSettableRelation[BindableRelationValue](fieldType)
			field.SetValue(fieldValue, true)
		}

		var bindable = fieldValue.(BindableRelationValue)
		var err = bindable.BindToObject(&ParentInfo{
			Object: obj,
			Field:  field,
		})
		if err != nil {
			panic(fmt.Sprintf("failed to bind relation %s to object %T: %v", relName, obj, err))
		}
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

		switch {
		case fieldType == reflect.TypeOf(related):
			// If the field is a slice of Definer, we can set the related objects directly
			field.SetValue(related, true)

		case fieldType.Kind() == reflect.Slice:
			// If the field is a slice, we can set the related objects directly after
			// converting them to the appropriate type.
			var slice = reflect.MakeSlice(fieldType, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				slice.Index(i).Set(reflect.ValueOf(relatedObj.Model()))
			}
			field.SetValue(slice.Interface(), true)

		default:
			panic(fmt.Sprintf("expected field %s to be a slice, got %s", relName, fieldType))
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
		case fieldType.Implements(reflect.TypeOf((*ThroughRelationValue)(nil)).Elem()):
			// If the field is a SettableThroughRelation, we can set the related object directly
			var rel = fieldValue.(ThroughRelationValue)
			rel.SetValue(relatedObject.Model(), relatedObject.Through())

		case fieldType.Implements(reflect.TypeOf((*Relation)(nil)).Elem()):
			// If the field is a Relation, we can set the related object directly
			field.SetValue(relatedObject, true)

		case fieldType.Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
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

		switch {
		case fieldType.Implements(reflect.TypeOf((*MultiThroughRelationValue)(nil)).Elem()):
			// If the field is a SettableMultiRelation, we can set the related objects directly
			var rel = fieldValue.(MultiThroughRelationValue)
			rel.SetValues(relatedObjects)

		case fieldType.Kind() == reflect.Slice && fieldType.Elem().Implements(reflect.TypeOf((*Relation)(nil)).Elem()):
			// If the field is a slice, we can set the related objects directly
			var slice = reflect.MakeSlice(fieldType, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				slice.Index(i).Set(reflect.ValueOf(relatedObj))
			}
			fieldDefs.Set(relName, slice.Interface())

		case fieldType.Kind() == reflect.Slice && fieldType.Elem().Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
			// If the field is a slice of Definer, we can set the related objects directly
			var slice = reflect.MakeSlice(fieldType, len(relatedObjects), len(relatedObjects))
			for i, relatedObj := range relatedObjects {
				var relatedDefiner = relatedObj.Model()
				slice.Index(i).Set(reflect.ValueOf(relatedDefiner))
			}
			fieldDefs.Set(relName, slice.Interface())

		default:
			panic(fmt.Sprintf("expected field %s to be a slice, got %s", relName, fieldType))
		}
	default:
		panic(fmt.Sprintf("unknown relation type %s for field %s in %T", relTyp, relName, obj))
	}
}

type walkInfo struct {
	idx       int
	depth     int
	fieldDefs attrs.Definitions
	field     attrs.Field
	chain     []string
}

//  func (w walkInfo) path() string {
//  	var path = w.field.Name()
//  	if len(w.chain) > 1 {
//  		path = fmt.Sprintf("%s.%s", w.chain[:w.depth], path)
//  	}
//  	return path
//  }

// walkFields traverses the fields of an object based on a chain of field names.
//
// It yields each field found at the last depth of the chain, allowing for
// custom processing of the field (e.g., collecting values).
func walkFieldValues(obj attrs.Definitions, chain []string, idx *int, depth int, yield func(walkInfo) bool) error {

	if depth > len(chain)-1 {
		return fmt.Errorf("depth %d exceeds chain length %d: %w", depth, len(chain), query_errors.ErrFieldNotFound)
	}

	var fieldName = chain[depth]
	var field, ok = obj.Field(fieldName)
	if !ok {
		return fmt.Errorf("field %s not found in object %T: %w", fieldName, obj, query_errors.ErrFieldNotFound)
	}

	if depth == len(chain)-1 {
		if !yield(walkInfo{
			idx:       *idx,
			depth:     depth,
			fieldDefs: obj,
			field:     field,
			chain:     chain,
		}) {
			return errStopIteration
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
		if err := walkFieldValues(definer, chain, idx, depth+1, yield); err != nil {
			if errors.Is(err, query_errors.ErrNilPointer) {
				return nil // Skip nil pointers
			}
			return fmt.Errorf("%s: %w", fieldName, err)
		}
	case rTyp.Implements(reflect.TypeOf((*ThroughRelationValue)(nil)).Elem()):
		var value = value.(ThroughRelationValue)
		var model, _ = value.GetValue()
		if model == nil {
			return query_errors.ErrNilPointer
		}
		if err := walkFieldValues(model.FieldDefs(), chain, idx, depth+1, yield); err != nil {
			if errors.Is(err, query_errors.ErrNilPointer) {
				return nil // Skip nil pointers
			}
			return fmt.Errorf("%s: %w", fieldName, err)
		}
	case rTyp.Implements(reflect.TypeOf((*MultiThroughRelationValue)(nil)).Elem()):
		var value = value.(MultiThroughRelationValue)
		var relatedObjects = value.GetValues()
		if len(relatedObjects) == 0 {
			return nil // Skip empty relations
		}
		for _, rel := range relatedObjects {
			var modelDefs = rel.Model().FieldDefs()
			if err := walkFieldValues(modelDefs, chain, idx, depth+1, yield); err != nil {
				if errors.Is(err, query_errors.ErrNilPointer) {
					continue // Skip nil relations
				}
				return fmt.Errorf("%s: %w", fieldName, err)
			}
		}
	case rTyp.Kind() == reflect.Slice && rTyp.Elem().Implements(reflect.TypeOf((*attrs.Definer)(nil)).Elem()):
		// If the field is a slice of Definer, we can walk its fields
		var slice = reflect.ValueOf(value)
		for i := 0; i < slice.Len(); i++ {
			var elem = slice.Index(i).Interface()
			if elem == nil {
				continue // Skip nil elements
			}
			if err := walkFieldValues(elem.(attrs.Definer).FieldDefs(), chain, idx, depth+1, yield); err != nil {
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
			if err := walkFieldValues(rel.Model().FieldDefs(), chain, idx, depth+1, yield); err != nil {
				if errors.Is(err, query_errors.ErrNilPointer) {
					continue // Skip elements where the field is nil
				}
				return fmt.Errorf("%s[%d]: %w", fieldName, i, err)
			}
		}
	default:
		return fmt.Errorf("expected field %s in object %T to be a Definer, slice of Definer, or slice of Relation, got %s", fieldName, obj.Instance(), rTyp)
	}
	return nil
}
