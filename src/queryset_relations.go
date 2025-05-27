package queries

import "github.com/Nigel2392/go-django/src/core/attrs"

var (
	_ Relation                     = (*typedRelation[attrs.Definer, attrs.Definer])(nil)
	_ Relation                     = (*baseRelation)(nil)
	_ Relation                     = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ SettableThroughRelation      = (*RelO2O[attrs.Definer, attrs.Definer])(nil)
	_ SettableMultiThroughRelation = (*RelM2M[attrs.Definer, attrs.Definer])(nil)
)

type typedRelation[T1, T2 attrs.Definer] struct {
	object  T1
	through T2
}

func (r *typedRelation[T1, T2]) Model() attrs.Definer {
	return r.object
}

func (r *typedRelation[T1, T2]) Through() attrs.Definer {
	return r.through
}

func (r *typedRelation[T1, T2]) Instance() T1 {
	return r.object
}

func (r *typedRelation[T1, T2]) InstanceThrough() T2 {
	return r.through
}

type baseRelation = typedRelation[attrs.Definer, attrs.Definer]

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
