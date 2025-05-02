package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/pkg/errors"
)

//type Relation struct {
//	From  attrs.Definer
//	To    attrs.Definer
//	Field string
//}

type Related struct {
	Object        attrs.Definer
	ThroughObject attrs.Definer
}

func (r *Related) Model() attrs.Definer {
	return r.Object
}

func (r *Related) Through() attrs.Definer {
	return r.ThroughObject
}

type Relation interface {
	From() attrs.Definer
	To() attrs.Definer

	Forward(from attrs.Definer) ([]attrs.Definer, error)
	Reverse(to attrs.Definer) ([]attrs.Definer, error)
}

type relationForeignKey struct {
	from  attrs.Definer
	field string
	to    attrs.Definer
}

func NewForeignKeyRelation(from attrs.Definer, field string, to attrs.Definer) Relation {
	if from == nil {
		panic("relationForeignKey: from is nil")
	}
	if field == "" {
		panic("relationForeignKey: field is empty")
	}
	if to == nil {
		panic("relationForeignKey: to is nil")
	}
	return &relationForeignKey{
		from:  from,
		field: field,
		to:    to,
	}
}

func (r *relationForeignKey) From() attrs.Definer {
	return r.from
}

func (r *relationForeignKey) To() attrs.Definer {
	return r.to
}

func (r *relationForeignKey) Forward(from attrs.Definer) ([]attrs.Definer, error) {
	if from == nil {
		panic("relationForeignKey: from is nil or from model has no primary key")
	}

	var (
		fromDefs = from.FieldDefs()
		toDefs   = r.to.FieldDefs()
		toPrim   = toDefs.Primary()
	)

	var f, ok = fromDefs.Field(r.field)
	if !ok {
		panic(fmt.Errorf(
			"relationForeignKey: field %q not found in from model %T", r.field, from,
		))
	}

	var val, err = f.Value()
	if err != nil {
		return nil, err
	}

	var qs = Objects(r.to)
	qs = qs.Filter(
		toPrim.Name(), val,
	)

	res, err := qs.All().Exec()
	if err != nil {
		return nil, err
	}

	var results = make([]attrs.Definer, len(res))
	for i, obj := range res {
		results[i] = obj.Object
	}
	return results, nil
}

func (r *relationForeignKey) Reverse(to attrs.Definer) ([]attrs.Definer, error) {
	if to == nil {
		panic("relationForeignKey: to is nil or to model has no primary key")
	}

	var (
		toDefs = to.FieldDefs()
		toPrim = toDefs.Primary()
	)

	var val, err = toPrim.Value()
	if err != nil {
		return nil, errors.Wrapf(
			err, "failed to get value of primary key %q in model %T for reversing relationship", toPrim.Name(), to,
		)
	}

	var qs = Objects(r.from)
	qs = qs.Filter(
		r.field, val,
	)

	res, err := qs.All().Exec()
	if err != nil {
		return nil, err
	}

	var results = make([]attrs.Definer, len(res))
	for i, obj := range res {
		results[i] = obj.Object
	}
	return results, nil
}

type relationManyToMany struct {
	from      attrs.Definer
	to        attrs.Definer
	through   attrs.Definer
	fromField string
	toField   string
}

func NewManyToManyRelation(from attrs.Definer, to attrs.Definer, through attrs.Definer, fromField string, toField string) Relation {
	if from == nil {
		panic("relationManyToMany: from is nil")
	}
	if to == nil {
		panic("relationManyToMany: to is nil")
	}
	if through == nil {
		panic("relationManyToMany: through is nil")
	}
	if fromField == "" {
		panic("relationManyToMany: fromField is empty")
	}
	if toField == "" {
		panic("relationManyToMany: toField is empty")
	}
	return &relationManyToMany{
		from:      from,
		to:        to,
		through:   through,
		fromField: fromField,
		toField:   toField,
	}
}

func (r *relationManyToMany) From() attrs.Definer {
	return r.from
}

func (r *relationManyToMany) To() attrs.Definer {
	return r.to
}

func (r *relationManyToMany) Forward(from attrs.Definer) ([]attrs.Definer, error) {
	if from == nil {
		panic("relationManyToMany: from is nil")
	}
	pk := from.FieldDefs().Primary()
	if pk == nil {
		return nil, fmt.Errorf("m2m: from has no primary key")
	}
	pkVal, err := pk.Value()
	if err != nil {
		return nil, err
	}

	// 1. find all through objects where fromField == pk
	throughQS := Objects(r.through).Filter(r.fromField, pkVal)
	throughObjs, err := throughQS.All().Exec()
	if err != nil {
		return nil, err
	}

	// 2. extract all to IDs
	var toIDs []interface{}
	for _, obj := range throughObjs {
		f, _ := obj.Object.FieldDefs().Field(r.toField)
		idVal, _ := f.Value()
		toIDs = append(toIDs, idVal)
	}

	// 3. fetch related To models
	toQS := Objects(r.to).Filter(r.to.FieldDefs().Primary().Name(), toIDs...)
	toRes, err := toQS.All().Exec()
	if err != nil {
		return nil, err
	}

	var result = make([]attrs.Definer, len(toRes))
	for i, r := range toRes {
		result[i] = r.Object
	}
	return result, nil
}
func (r *relationManyToMany) Reverse(to attrs.Definer) ([]attrs.Definer, error) {
	if to == nil {
		panic("relationManyToMany: to is nil")
	}
	pk := to.FieldDefs().Primary()
	if pk == nil {
		return nil, fmt.Errorf("m2m: to has no primary key")
	}
	pkVal, err := pk.Value()
	if err != nil {
		return nil, err
	}

	// 1. find all through objects where toField == pk
	throughQS := Objects(r.through).Filter(r.toField, pkVal)
	throughObjs, err := throughQS.All().Exec()
	if err != nil {
		return nil, err
	}

	// 2. extract all from IDs
	var fromIDs []interface{}
	for _, obj := range throughObjs {
		f, _ := obj.Object.FieldDefs().Field(r.fromField)
		idVal, _ := f.Value()
		fromIDs = append(fromIDs, idVal)
	}

	// 3. fetch related From models
	fromQS := Objects(r.from).Filter(r.from.FieldDefs().Primary().Name(), fromIDs...)
	fromRes, err := fromQS.All().Exec()
	if err != nil {
		return nil, err
	}

	var result = make([]attrs.Definer, len(fromRes))
	for i, r := range fromRes {
		result[i] = r.Object
	}
	return result, nil
}
