package internal

import (
	"fmt"
	"iter"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/elliotchance/orderedmap/v2"
)

type RelationType int

const (
	// OneToMany is a one-to-many relation (foreign key)
	RelationTypeOneToMany RelationType = iota

	// OneToOne is a one-to-one relation (foreign key or with through)
	RelationTypeOneToOne

	// ManyToMany is a many-to-many relation (through)
	RelationTypeManyToMany

	// ManyToOne is a many-to-one relation (foreign key reverse)
	RelationTypeManyToOne
)

type CanReverseAlias interface {
	attrs.Field

	// ReverseAlias returns the reverse alias for this field
	// This is used to determine the reverse name for the relation
	// when registering the reverse relation
	ReverseAlias() string
}

type RelationTarget interface {
	Model() attrs.Definer
	Field() attrs.Field
}

type RelationChain interface {
	From() RelationChain
	To() RelationChain
	RelationTarget
}

type Relation interface {
	// Type returns the type of the relation
	Type() RelationType

	// Chain returns the chain of relations
	// from the source model to the target model
	Chain() RelationChain

	// Target returns the eventual target model of the relation
	Target() RelationChain
}

type ModelMeta interface {
	// Model returns the model for this meta
	Model() attrs.Definer

	// Forward returns the forward relations for this model
	Forward(relField string) (Relation, bool)

	// ForwardMap returns the forward relations map for this model
	ForwardMap() *orderedmap.OrderedMap[string, Relation]

	// IterForward returns an iterator for the forward relations
	IterForward() iter.Seq2[string, Relation]

	// Reverse returns the reverse relations for this model
	Reverse(relField string) (Relation, bool)

	// ReverseMap returns the reverse relations map for this model
	ReverseMap() *orderedmap.OrderedMap[string, Relation]

	// IterReverse returns an iterator for the reverse relations
	IterReverse() iter.Seq2[string, Relation]
}

type relationChain struct {
	model attrs.Definer
	field attrs.Field
	next  RelationChain
	prev  RelationChain
}

func (r *relationChain) Model() attrs.Definer {
	return r.model
}

func (r *relationChain) Field() attrs.Field {
	return r.field
}

func (r *relationChain) From() RelationChain {
	return r.prev
}

func (r *relationChain) To() RelationChain {
	return r.next
}

type relationMeta struct {
	typ    RelationType
	chain  RelationChain
	target RelationChain
}

func (r *relationMeta) Type() RelationType {
	return r.typ
}

func (r *relationMeta) Chain() RelationChain {
	return r.chain
}

func (r *relationMeta) Target() RelationChain {
	if r.target == nil {
		var last = r.chain
		for last.To() != nil {
			last = last.To()
		}
		r.target = last
	}
	return r.target
}

type modelMeta struct {
	model   attrs.Definer
	forward *orderedmap.OrderedMap[string, Relation] // forward orderedmap
	reverse *orderedmap.OrderedMap[string, Relation] // forward orderedmap
}

func (m *modelMeta) Model() attrs.Definer {
	return m.model
}

func (m *modelMeta) Forward(relField string) (Relation, bool) {
	if rel, ok := m.forward.Get(relField); ok {
		return rel, true
	}
	return nil, false
}

func (m *modelMeta) ForwardMap() *orderedmap.OrderedMap[string, Relation] {
	return m.forward
}

func (m *modelMeta) ReverseMap() *orderedmap.OrderedMap[string, Relation] {
	return m.reverse
}

func (m *modelMeta) Reverse(relField string) (Relation, bool) {
	if rel, ok := m.reverse.Get(relField); ok {
		return rel, true
	}
	return nil, false
}

func (m *modelMeta) iter(mapVal *orderedmap.OrderedMap[string, Relation]) iter.Seq2[string, Relation] {
	return iter.Seq2[string, Relation](func(yield func(string, Relation) bool) {
		for front := mapVal.Front(); front != nil; front = front.Next() {
			if !yield(front.Key, front.Value) {
				return
			}
		}
	})
}

func (m *modelMeta) IterForward() iter.Seq2[string, Relation] {
	return m.iter(m.forward)
}

func (m *modelMeta) IterReverse() iter.Seq2[string, Relation] {
	return m.iter(m.reverse)
}

var modelReg = make(map[reflect.Type]*modelMeta)

func NewReverseAlias(c RelationChain) string {
	var name = fmt.Sprintf("%TSet", c.Model())
	var parts = strings.Split(name, ".")
	if len(parts) > 1 {
		name = parts[len(parts)-1]
	}
	return name
}

func GetReverseAlias(f attrs.Field, default_ string) string {
	if f == nil {
		return default_
	}

	var alias string
	if reverseName, ok := f.(CanReverseAlias); ok {
		alias = reverseName.ReverseAlias()
	}

	if alias == "" {
		var atts = f.Attrs()
		var s, ok = atts[ATTR_REVERSE_ALIAS]
		if ok {
			alias = s.(string)
		}
	}

	if alias != "" {
		return alias
	}

	return default_
}

func registerReverseRelation(
	targetModel attrs.Definer,
	forward RelationChain,
	relType RelationType,
	reverseName string,
) {
	//// Step 1: Get final target in the chain (the destination model)
	//var last = forward
	//for last.To() != nil {
	//	last = last.To()
	//}

	targetType := reflect.TypeOf(targetModel)

	// Step 2: Get or init ModelMeta for the target
	meta, ok := modelReg[targetType]
	if !ok {
		RegisterModel(targetModel)
		meta = modelReg[targetType]
	}

	// Step 3: Determine a reverse name
	// Prefer something explicit if available (you could add support for a "related_name" tag in field config)
	if reverseName == "" {
		reverseName = NewReverseAlias(forward)
	}

	// Step 4: Build reversed chain
	var reversed RelationChain
	var current = forward
	for current != nil {
		next := current.To()
		reversed = &relationChain{
			model: current.Model(),
			field: current.Field(),
			next:  reversed,
		}
		current = next
	}

	// Step 5: Store in reverseRelations
	if _, ok := meta.reverse.Get(reverseName); ok {
		panic(fmt.Errorf("reverse relation %q already registered for %T", reverseName, targetModel))
	}

	meta.reverse.Set(reverseName, &relationMeta{
		typ:    relType,
		chain:  reversed,
		target: current,
	})

	modelReg[targetType] = meta
}

func RegisterModel(model attrs.Definer) {
	var t = reflect.TypeOf(model)
	if _, ok := modelReg[t]; ok {
		//var stackFrame [10]uintptr
		//n := runtime.Callers(2, stackFrame[:])
		//frames := runtime.CallersFrames(stackFrame[:n])
		//
		//frame, _ := frames.Next()
		//
		//logger.Warnf(
		//	"model %T already registered, skipping registration (called from %s:%d)",
		//	model, frame.File, frame.Line,
		//)
		return
	}

	var meta = &modelMeta{
		model:   model,
		forward: orderedmap.NewOrderedMap[string, Relation](),
		reverse: orderedmap.NewOrderedMap[string, Relation](),
	}

	// set the model in the registry early - reverse relations may need it
	// if the model is self-referential (e.g. a tree structure)
	modelReg[t] = meta

	var defs = model.FieldDefs()
	if defs == nil {
		panic(fmt.Errorf("model %T has no field definitions", model))
	}

	var fields = defs.Fields()
	for _, field := range fields {

		var (
			fk  = field.ForeignKey()
			o2o = field.OneToOne()
			m2m = field.ManyToMany()
		)

		switch {
		case fk != nil:
			var relDefs = fk.FieldDefs()
			var chain = &relationChain{
				model: model,
				field: field,
			}
			chain.next = &relationChain{
				model: fk,
				field: relDefs.Primary(),
				prev:  chain,
			}

			meta.forward.Set(field.Name(), &relationMeta{
				typ:    RelationTypeOneToMany,
				chain:  chain,
				target: chain.next,
			})

			var reverseAlias = GetReverseAlias(field, "")
			registerReverseRelation(
				relDefs.Instance(),
				chain,
				RelationTypeManyToOne,
				reverseAlias,
			)

		case o2o != nil:

			var (
				through = o2o.Through()
				target  = o2o.Model()
				relPath = []attrs.Definer{}
			)

			if through != nil {
				relPath = append(relPath, through)
			}
			if target != nil {
				relPath = append(relPath, target)
			}

			// Build forward chain
			root := &relationChain{
				model: model,
				field: field,
			}
			chain := root
			for _, step := range relPath {
				def := step.FieldDefs()
				relField := def.Primary()
				next := &relationChain{
					model: step,
					field: relField,
					prev:  chain,
				}
				chain.next = next
				chain = next
			}

			meta.forward.Set(field.Name(), &relationMeta{
				typ:    RelationTypeOneToOne,
				chain:  root,
				target: chain,
			})

			// This is the actual end target
			finalTarget := chain.model

			var reverseAlias = GetReverseAlias(field, "")
			registerReverseRelation(
				finalTarget,
				root,
				RelationTypeOneToOne,
				reverseAlias,
			)

		case m2m != nil:

			var relDefs = []attrs.Definer{
				m2m.Through(),
				m2m.Model(),
			}

			var root = &relationChain{
				model: model,
				field: field,
			}

			var chain = root
			for _, relDef := range relDefs {
				if relDef == nil {
					continue
				}
				var relChain = &relationChain{
					model: relDef,
					field: relDef.FieldDefs().Primary(),
					prev:  chain,
				}
				chain.next = relChain
				chain = relChain
			}

			meta.forward.Set(field.Name(), &relationMeta{
				typ:    RelationTypeManyToMany,
				chain:  root,
				target: chain,
			})

			var reverseAlias = GetReverseAlias(field, "")
			registerReverseRelation(
				model,
				chain,
				RelationTypeManyToMany,
				reverseAlias,
			)
		}
	}
}

func GetModelMeta(model attrs.Definer) ModelMeta {
	if meta, ok := modelReg[reflect.TypeOf(model)]; ok {
		return meta
	}
	panic(fmt.Errorf("model %T not registered with `queries.RegisterModel`", model))
}

func GetRelationMeta(m attrs.Definer, name string) (Relation, bool) {
	var meta = GetModelMeta(m)
	if rel, ok := meta.Forward(name); ok {
		return rel, true
	}
	if rel, ok := meta.Reverse(name); ok {
		return rel, true
	}
	return nil, false
}

//type Relation struct {
//	From  attrs.Definer
//	To    attrs.Definer
//	Field string
//}
//
//	type Related struct {
//		Object        attrs.Definer
//		ThroughObject attrs.Definer
//	}
//
//	func (r *Related) Model() attrs.Definer {
//		return r.Object
//	}
//
//	func (r *Related) Through() attrs.Definer {
//		return r.ThroughObject
//	}
//
//	type Relation interface {
//		From() attrs.Definer
//		To() attrs.Definer
//
//		Forward(from attrs.Definer) ([]attrs.Definer, error)
//		Reverse(to attrs.Definer) ([]attrs.Definer, error)
//	}
//
//	type relationForeignKey struct {
//		from  attrs.Definer
//		field string
//		to    attrs.Definer
//	}
//
//	func NewForeignKeyRelation(from attrs.Definer, field string, to attrs.Definer) Relation {
//		if from == nil {
//			panic("relationForeignKey: from is nil")
//		}
//		if field == "" {
//			panic("relationForeignKey: field is empty")
//		}
//		if to == nil {
//			panic("relationForeignKey: to is nil")
//		}
//		return &relationForeignKey{
//			from:  from,
//			field: field,
//			to:    to,
//		}
//	}
//
//	func (r *relationForeignKey) From() attrs.Definer {
//		return r.from
//	}
//
//	func (r *relationForeignKey) To() attrs.Definer {
//		return r.to
//	}
//
//	func (r *relationForeignKey) Forward(from attrs.Definer) ([]attrs.Definer, error) {
//		if from == nil {
//			panic("relationForeignKey: from is nil or from model has no primary key")
//		}
//
//		var (
//			fromDefs = from.FieldDefs()
//			toDefs   = r.to.FieldDefs()
//			toPrim   = toDefs.Primary()
//		)
//
//		var f, ok = fromDefs.Field(r.field)
//		if !ok {
//			panic(fmt.Errorf(
//				"relationForeignKey: field %q not found in from model %T", r.field, from,
//			))
//		}
//
//		var val, err = f.Value()
//		if err != nil {
//			return nil, err
//		}
//
//		var qs = Objects(r.to)
//		qs = qs.Filter(
//			toPrim.Name(), val,
//		)
//
//		res, err := qs.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		var results = make([]attrs.Definer, len(res))
//		for i, obj := range res {
//			results[i] = obj.Object
//		}
//		return results, nil
//	}
//
//	func (r *relationForeignKey) Reverse(to attrs.Definer) ([]attrs.Definer, error) {
//		if to == nil {
//			panic("relationForeignKey: to is nil or to model has no primary key")
//		}
//
//		var (
//			toDefs = to.FieldDefs()
//			toPrim = toDefs.Primary()
//		)
//
//		var val, err = toPrim.Value()
//		if err != nil {
//			return nil, errors.Wrapf(
//				err, "failed to get value of primary key %q in model %T for reversing relationship", toPrim.Name(), to,
//			)
//		}
//
//		var qs = Objects(r.from)
//		qs = qs.Filter(
//			r.field, val,
//		)
//
//		res, err := qs.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		var results = make([]attrs.Definer, len(res))
//		for i, obj := range res {
//			results[i] = obj.Object
//		}
//		return results, nil
//	}
//
//	type relationManyToMany struct {
//		from      attrs.Definer
//		to        attrs.Definer
//		through   attrs.Definer
//		fromField string
//		toField   string
//	}
//
//	func NewManyToManyRelation(from attrs.Definer, to attrs.Definer, through attrs.Definer, fromField string, toField string) Relation {
//		if from == nil {
//			panic("relationManyToMany: from is nil")
//		}
//		if to == nil {
//			panic("relationManyToMany: to is nil")
//		}
//		if through == nil {
//			panic("relationManyToMany: through is nil")
//		}
//		if fromField == "" {
//			panic("relationManyToMany: fromField is empty")
//		}
//		if toField == "" {
//			panic("relationManyToMany: toField is empty")
//		}
//		return &relationManyToMany{
//			from:      from,
//			to:        to,
//			through:   through,
//			fromField: fromField,
//			toField:   toField,
//		}
//	}
//
//	func (r *relationManyToMany) From() attrs.Definer {
//		return r.from
//	}
//
//	func (r *relationManyToMany) To() attrs.Definer {
//		return r.to
//	}
//
//	func (r *relationManyToMany) Forward(from attrs.Definer) ([]attrs.Definer, error) {
//		if from == nil {
//			panic("relationManyToMany: from is nil")
//		}
//		pk := from.FieldDefs().Primary()
//		if pk == nil {
//			return nil, fmt.Errorf("m2m: from has no primary key")
//		}
//		pkVal, err := pk.Value()
//		if err != nil {
//			return nil, err
//		}
//
//		// 1. find all through objects where fromField == pk
//		throughQS := Objects(r.through).Filter(r.fromField, pkVal)
//		throughObjs, err := throughQS.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		// 2. extract all to IDs
//		var toIDs []interface{}
//		for _, obj := range throughObjs {
//			f, _ := obj.Object.FieldDefs().Field(r.toField)
//			idVal, _ := f.Value()
//			toIDs = append(toIDs, idVal)
//		}
//
//		// 3. fetch related To models
//		toQS := Objects(r.to).Filter(r.to.FieldDefs().Primary().Name(), toIDs...)
//		toRes, err := toQS.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		var result = make([]attrs.Definer, len(toRes))
//		for i, r := range toRes {
//			result[i] = r.Object
//		}
//		return result, nil
//	}
//	func (r *relationManyToMany) Reverse(to attrs.Definer) ([]attrs.Definer, error) {
//		if to == nil {
//			panic("relationManyToMany: to is nil")
//		}
//		pk := to.FieldDefs().Primary()
//		if pk == nil {
//			return nil, fmt.Errorf("m2m: to has no primary key")
//		}
//		pkVal, err := pk.Value()
//		if err != nil {
//			return nil, err
//		}
//
//		// 1. find all through objects where toField == pk
//		throughQS := Objects(r.through).Filter(r.toField, pkVal)
//		throughObjs, err := throughQS.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		// 2. extract all from IDs
//		var fromIDs []interface{}
//		for _, obj := range throughObjs {
//			f, _ := obj.Object.FieldDefs().Field(r.fromField)
//			idVal, _ := f.Value()
//			fromIDs = append(fromIDs, idVal)
//		}
//
//		// 3. fetch related From models
//		fromQS := Objects(r.from).Filter(r.from.FieldDefs().Primary().Name(), fromIDs...)
//		fromRes, err := fromQS.All().Exec()
//		if err != nil {
//			return nil, err
//		}
//
//		var result = make([]attrs.Definer, len(fromRes))
//		for i, r := range fromRes {
//			result[i] = r.Object
//		}
//		return result, nil
//	}
//
