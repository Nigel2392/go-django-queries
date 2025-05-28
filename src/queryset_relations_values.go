package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/elliotchance/orderedmap/v2"
)

var (
	_ canPrimaryKey             = (*baseRelation)(nil)
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

func (rl *RelO2O[T1, T2]) ParentInfo() *ParentInfo {
	if rl == nil {
		return nil
	}
	return rl.Parent
}

func (rl *RelO2O[T1, T2]) BindToModel(parent attrs.Definer, parentField attrs.Field) error {
	if rl == nil {
		return nil
	}
	rl.Parent = &ParentInfo{
		Object: parent,
		Field:  parentField,
	}
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

func (rl *RelM2M[T1, T2]) ParentInfo() *ParentInfo {
	if rl == nil {
		return nil
	}
	return rl.Parent
}

func (rl *RelM2M[T1, T2]) BindToModel(parent attrs.Definer, parentField attrs.Field) error {
	if rl == nil {
		return nil
	}
	rl.Parent = &ParentInfo{
		Object: parent,
		Field:  parentField,
	}
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
			Parent:        rl.Parent,
			Object:        r.Model().(T1),
			ThroughObject: r.Through().(T2),
		}

		// rl.relations = append(rl.relations, o2o)

		var pkValue any
		if canPk, ok := r.(canPrimaryKey); ok {
			pkValue = canPk.PrimaryKey()
		}

		// First nil check we can get the primary key
		// from the relation's definitions.
		if pkValue == nil {
			var objDefs = o2o.Object.FieldDefs()
			var pkField = objDefs.Primary()
			pkValue = pkField.GetValue()
		}

		// If the primary key is still nill it is OK to panic,
		// because it means the object does not have a primary key set.
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
