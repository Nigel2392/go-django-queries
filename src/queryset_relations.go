package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

type throughProxy struct {
	throughDefinition attrs.Through
	object            attrs.Definer
	defs              attrs.Definitions
	sourceField       attrs.Field
	targetField       attrs.Field
}

func newThroughProxy(throughDefinition attrs.Through) *throughProxy {
	var (
		ok              bool
		sourceFieldStr  = throughDefinition.SourceField()
		targetFieldStr  = throughDefinition.TargetField()
		throughInstance = throughDefinition.Model()
		defs            = throughInstance.FieldDefs()
		proxy           = &throughProxy{
			defs:              defs,
			object:            throughInstance,
			throughDefinition: throughDefinition,
		}
	)

	if proxy.sourceField, ok = defs.Field(sourceFieldStr); !ok {
		panic(fmt.Errorf(
			"source field %s not found in through model %T: %w",
			sourceFieldStr, throughInstance, query_errors.ErrFieldNotFound,
		))
	}

	if proxy.targetField, ok = defs.Field(targetFieldStr); !ok {
		panic(fmt.Errorf(
			"target field %s not found in through model %T: %w",
			targetFieldStr, throughInstance, query_errors.ErrFieldNotFound,
		))
	}

	return proxy
}

type relatedQuerySet[T attrs.Definer, T2 any] struct {
	embedder      T2
	source        *ParentInfo
	joinCondition *JoinCondition
	rel           attrs.Relation
	originalQs    QuerySet[T]
	qs            *QuerySet[T]
}

// NewrelatedQuerySet creates a new relatedQuerySet for the given model type.
func newRelatedQuerySet[T attrs.Definer, T2 any](embedder T2, rel attrs.Relation, source *ParentInfo) *relatedQuerySet[T, T2] {
	var (
		condition *JoinCondition

		qs           = GetQuerySet(internal.NewDefiner[T]())
		throughModel = rel.Through()
		front, back  = qs.compiler.Quote()
	)

	var targetFieldInfo = &FieldInfo{
		Model: qs.model,
		Table: Table{
			Name: qs.queryInfo.TableName,
		},
		Fields: ForSelectAllFields[attrs.Field](
			qs.queryInfo.Fields,
		),
	}

	if throughModel != nil {
		var throughObject = newThroughProxy(throughModel)

		targetFieldInfo.Through = &FieldInfo{
			Model: throughObject.object,
			Table: Table{
				Name: throughObject.defs.TableName(),
				Alias: fmt.Sprintf(
					"%s_through",
					qs.queryInfo.TableName,
				),
			},
			Fields: ForSelectAllFields[attrs.Field](throughObject.defs),
		}

		condition = &JoinCondition{
			Operator: LogicalOpEQ,
			ConditionA: fmt.Sprintf(
				"%s%s%s.%s%s%s",
				front, targetFieldInfo.Table.Name, back,
				front, source.Field.ColumnName(), back,
			),
			ConditionB: fmt.Sprintf(
				"%s%s%s.%s%s%s",
				front, targetFieldInfo.Through.Table.Alias, back,
				front, throughObject.targetField.ColumnName(), back,
			),
		}

		condition.Next = &JoinCondition{
			Operator: LogicalOpEQ,
			ConditionA: fmt.Sprintf(
				"%s%s%s.%s%s%s",
				front, targetFieldInfo.Through.Table.Alias, back,
				front, throughObject.sourceField.ColumnName(), back,
			),
			ConditionB: "?",
			Args: []any{
				source.Object.FieldDefs().Primary().GetValue(),
			},
		}

		var join = JoinDef{
			TypeJoin: TypeJoinInner,
			Table: Table{
				Name: throughObject.defs.TableName(),
				Alias: fmt.Sprintf(
					"%s_through",
					qs.queryInfo.TableName,
				),
			},
			JoinCondition: condition,
		}

		qs.internals.addJoin(join)
	}

	qs.internals.Fields = append(
		qs.internals.Fields, targetFieldInfo,
	)

	return &relatedQuerySet[T, T2]{
		embedder:      embedder,
		rel:           rel,
		source:        source,
		originalQs:    *qs,
		joinCondition: condition,
		qs:            qs,
	}
}

func (t *relatedQuerySet[T, T2]) createTargets(targets []T) ([]T, error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot create targets")
	}

	return t.originalQs.Clone().BulkCreate(targets)
}

func (t *relatedQuerySet[T, T2]) createThroughObjects(targets []T) (rels []Relation, created int64, _ error) {
	if t.rel == nil {
		panic("Relation is nil, cannot create through object")
	}

	var throughModel = t.rel.Through()
	if throughModel == nil {
		panic("Through model is nil, cannot create through object")
	}

	var (
		targetsToSave   = make([]T, 0, len(targets))
		existingTargets = make([]T, 0, len(targets))
	)
	for _, target := range targets {
		var (
			defs    = target.FieldDefs()
			primary = defs.Primary()
			pkValue = primary.GetValue()
		)

		if fields.IsZero(pkValue) {
			targetsToSave = append(targetsToSave, target)
		} else {
			existingTargets = append(existingTargets, target)
		}
	}

	if len(targetsToSave) > 0 {
		var err error
		targetsToSave, err = t.createTargets(targetsToSave)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to save targets: %w", err)
		}
		created = int64(len(targetsToSave))
	}

	targets = append(existingTargets, targetsToSave...)

	// Create a new instance of the through model
	var (
		throughObj            = throughModel.Model()
		throughSourceFieldStr = throughModel.SourceField()
		throughTargetFieldStr = throughModel.TargetField()

		sourceObject        = t.source.Object.FieldDefs()
		sourceObjectPrimary = sourceObject.Primary()
		sourceObjectPk, err = sourceObjectPrimary.Value()

		relations     = make([]Relation, 0, len(targets))
		throughModels = make([]attrs.Definer, 0, len(targets))
	)
	if err != nil {
		return nil, created, fmt.Errorf("failed to get primary key for source object %T: %w", t.source.Object, err)
	}

	for _, target := range targets {
		var (
			// target related values
			targetDefs    = target.FieldDefs()
			targetPrimary = targetDefs.Primary()
			targetPk, err = targetPrimary.Value()

			// through model values
			ok          bool
			sourceField attrs.Field
			targetField attrs.Field
			newInstance = internal.NewObjectFromIface(throughObj)
			fieldDefs   = newInstance.FieldDefs()
		)
		if err != nil {
			return nil, created, fmt.Errorf("failed to get primary key for target %T: %w", target, err)
		}

		if sourceField, ok = fieldDefs.Field(throughSourceFieldStr); !ok {
			return nil, created, query_errors.ErrFieldNotFound
		}
		if targetField, ok = fieldDefs.Field(throughTargetFieldStr); !ok {
			return nil, created, query_errors.ErrFieldNotFound
		}

		if err := sourceField.Scan(sourceObjectPk); err != nil {
			return nil, created, err
		}
		if err := targetField.Scan(targetPk); err != nil {
			return nil, created, err
		}

		// Create a new relation object
		var rel = &baseRelation{
			pk:      targetPk,
			object:  target,
			through: newInstance,
		}

		throughModels = append(throughModels, newInstance)
		relations = append(relations, rel)
	}

	throughModels, err = GetQuerySet(throughObj).BulkCreate(throughModels)
	if err != nil {
		return nil, created, err
	}

	return relations, created, nil
}

func (t *relatedQuerySet[T, T2]) Filter(key any, vals ...any) T2 {
	t.qs = t.qs.Filter(key, vals...)
	return t.embedder
}

func (t *relatedQuerySet[T, T2]) OrderBy(fields ...string) T2 {
	t.qs = t.qs.OrderBy(fields...)
	return t.embedder
}

func (t *relatedQuerySet[T, T2]) Limit(limit int) T2 {
	t.qs = t.qs.Limit(limit)
	return t.embedder
}

func (t *relatedQuerySet[T, T2]) Offset(offset int) T2 {
	t.qs = t.qs.Offset(offset)
	return t.embedder
}

func (t *relatedQuerySet[T, T2]) Get() (*Row[T], error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot call Get()")
	}
	return t.qs.Get()
}

func (t *relatedQuerySet[T, T2]) All() (Rows[T], error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot call All()")
	}
	return t.qs.All()
}

type RelManyToManyQuerySet[T attrs.Definer] struct {
	backRef                                        MultiThroughRelationValue
	*relatedQuerySet[T, *RelManyToManyQuerySet[T]] // Embedding the relatedQuerySet to inherit its methods
}

func ManyToManyQuerySet[T attrs.Definer](backRef MultiThroughRelationValue) *RelManyToManyQuerySet[T] {
	var parentInfo = backRef.ParentInfo()
	var mQs = &RelManyToManyQuerySet[T]{
		backRef: backRef,
	}
	mQs.relatedQuerySet = newRelatedQuerySet[T](mQs, parentInfo.Field.Rel(), parentInfo)
	return mQs
}

func (r *RelManyToManyQuerySet[T]) AddTarget(target T) (created bool, err error) {
	added, err := r.AddTargets(target)
	if err != nil {
		return false, err
	}
	return added == 1, nil
}

func (r *RelManyToManyQuerySet[T]) AddTargets(targets ...T) (int64, error) {
	if r.backRef == nil {
		return 0, fmt.Errorf("back reference is nil, cannot add targets")
	}

	var relations, added, err = r.createThroughObjects(targets)
	if err != nil {
		return 0, fmt.Errorf("failed to create through objects: %w", err)
	}

	if len(relations) == 0 {
		return added, fmt.Errorf("no relations created for targets %T", targets)
	}

	var relList = r.backRef.GetValues()
	if relList == nil {
		relList = make([]Relation, 0)
	}

	relList = append(relList, relations...)

	r.backRef.SetValues(relList)
	return added, nil
}

func (r *RelManyToManyQuerySet[T]) SetTargets(targets []T) (added int64, err error) {
	if r.backRef == nil {
		return 0, fmt.Errorf("back reference is nil, cannot set targets")
	}

	_, err = r.ClearTargets()
	if err != nil {
		return 0, fmt.Errorf("failed to clear targets: %w", err)
	}

	relations, added, err := r.createThroughObjects(targets)
	if err != nil {
		return 0, fmt.Errorf("failed to create through objects: %w", err)
	}

	if len(relations) == 0 {
		return added, fmt.Errorf("no relations created for targets %T", targets)
	}

	r.backRef.SetValues(relations)
	return added, nil
}

func (r *RelManyToManyQuerySet[T]) RemoveTargets(targets ...any) (int64, error) {
	if r.backRef == nil {
		return 0, fmt.Errorf("back reference is nil, cannot remove targets")
	}

	targets = internal.ListUnpack(targets)

	var (
		pkValues = make([]any, 0, len(targets))
		pkMap    = make(map[any]struct{}, len(targets))
	)
targetLoop:
	for _, target := range targets {
		var pkValue any
		if canPk, ok := target.(canPrimaryKey); ok {
			pkValue = canPk.PrimaryKey()
		}

		if pkValue != nil {
			pkValues = append(pkValues, pkValue)
			pkMap[pkValue] = struct{}{}
			continue targetLoop
		}

		switch t := target.(type) {
		case attrs.Definer:
			var defs = t.FieldDefs()
			var pkField = defs.Primary()
			pkValue = pkField.GetValue()
		case attrs.Definitions:
			var pkField = t.Primary()
			pkValue = pkField.GetValue()
		default:
			return 0, fmt.Errorf("target %T does not have a primary key", target)
		}

		if pkValue == nil {
			return 0, fmt.Errorf("target %T does not have a valid primary key", target)
		}

		pkValues = append(pkValues, pkValue)
		pkMap[pkValue] = struct{}{}
	}

	var throughModel = newThroughProxy(r.rel.Through())
	var throughQs = GetQuerySet(throughModel.object).
		Filter(
			expr.Q(
				throughModel.sourceField.Name(),
				r.source.Object.FieldDefs().Primary().GetValue(),
			),
			expr.Q(
				fmt.Sprintf(
					"%s__in",
					throughModel.targetField.Name(),
				),
				pkValues...,
			),
		)

	var deleted, err = throughQs.Delete()
	if err != nil {
		return 0, fmt.Errorf("failed to delete through objects: %w", err)
	}

	if deleted == 0 {
		return 0, fmt.Errorf("no through objects deleted for targets %v", pkValues)
	}

	var relList = r.backRef.GetValues()
	if len(relList) == 0 {
		return deleted, nil
	}

	var newRels = make([]Relation, 0, len(relList))
	for _, rel := range relList {
		var model = rel.Model()
		var fieldDefs = model.FieldDefs()
		var pkValue = fieldDefs.Primary().GetValue()
		if _, ok := pkMap[pkValue]; !ok {
			newRels = append(newRels, rel)
		}
	}

	r.backRef.SetValues(newRels)
	return deleted, nil
}

func (r *RelManyToManyQuerySet[T]) ClearTargets() (int64, error) {
	if r.backRef == nil {
		return 0, fmt.Errorf("back reference is nil, cannot clear targets")
	}

	var throughModel = newThroughProxy(r.rel.Through())
	var throughQs = GetQuerySet(throughModel.object).
		Filter(
			expr.Q(
				throughModel.sourceField.Name(),
				r.source.Object.FieldDefs().Primary().GetValue(),
			),
			SubqueryIn(
				throughModel.targetField.Name(),
				ChangeObjectsType[T, attrs.Definer](
					r.qs.Select(r.qs.queryInfo.Primary.Name()),
				),
			),
		)

	var deleted, err = throughQs.Delete()
	if err != nil {
		return 0, fmt.Errorf("failed to delete through objects: %w", err)
	}

	r.backRef.SetValues([]Relation{}) // Clear the back reference values

	return deleted, nil
}
