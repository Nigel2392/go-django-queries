package queries

import "github.com/Nigel2392/go-django/src/core/attrs"

var (
	_ Relation                     = (*baseRelation)(nil)
	_ Relation                     = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ SettableThroughRelation      = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ SettableMultiThroughRelation = (*RelM2M[attrs.Definer, attrs.Definer])(nil)
)

// A base relation type that implements the Relation interface.
//
// It is used to set the related object and it's through object on a model.
type baseRelation struct {
	object  attrs.Definer
	through attrs.Definer
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
	Object        ModelType
	ThroughObject ThroughModelType
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

// A value which can be used on models to represent a Many-to-Many relation
// with a through model.
//
// This implements the [SettableMultiThroughRelation] interface, which allows setting
// the related objects and their through objects.
type RelM2M[T1, T2 attrs.Definer] []RelO2O[T1, T2]

func (rl *RelM2M[T1, T2]) SetValues(rel []Relation) {
	if len(rel) == 0 {
		*rl = nil
		return
	}

	var trs = make([]RelO2O[T1, T2], len(rel))
	for i, r := range rel {
		if r == nil {
			continue
		}
		trs[i] = RelO2O[T1, T2]{
			Object:        r.Model().(T1),
			ThroughObject: r.Through().(T2),
		}
	}

	*rl = trs
}

func (rl *RelM2M[T1, T2]) Len() int {
	if rl == nil {
		return 0
	}
	return len(*rl)
}
