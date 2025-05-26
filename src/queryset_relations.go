package queries

import "github.com/Nigel2392/go-django/src/core/attrs"

var (
	_ Relation                     = (*typedRelation[attrs.Definer, attrs.Definer])(nil)
	_ Relation                     = (*baseRelation)(nil)
	_ SettableThroughRelation      = (*ThroughRelation[attrs.Definer, attrs.Definer])(nil)
	_ SettableMultiThroughRelation = (*MultiThroughRelation[attrs.Definer, attrs.Definer])(nil)
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

type ThroughRelation[ModelType, ThroughModelType attrs.Definer] struct {
	Object        ModelType
	ThroughObject ThroughModelType
}

func (rl *ThroughRelation[T1, T2]) SetValue(instance attrs.Definer, through attrs.Definer) {
	if instance != nil {
		rl.Object = instance.(T1)
	}
	if through != nil {
		rl.ThroughObject = through.(T2)
	}
}

type MultiThroughRelation[T1, T2 attrs.Definer] []ThroughRelation[T1, T2]

func (rl *MultiThroughRelation[T1, T2]) SetValues(rel []Relation) {
	if len(rel) == 0 {
		return
	}

	var trs = make([]ThroughRelation[T1, T2], len(rel))
	for i, r := range rel {
		if r == nil {
			continue
		}
		trs[i] = ThroughRelation[T1, T2]{
			Object:        r.Model().(T1),
			ThroughObject: r.Through().(T2),
		}
	}
	*rl = trs
}
