package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

//
//  type throughProxy struct {
//  	troughDefinition attrs.Through
//  	object           attrs.Definer
//  	defs             attrs.Definitions
//  	sourceField      attrs.Field
//  	targetField      attrs.Field
//  }
//
//  func newThroughProxy(throughInstance attrs.Definer, troughDefinition attrs.Through) *throughProxy {
//  	var (
//  		ok             bool
//  		defs           = throughInstance.FieldDefs()
//  		sourceFieldStr = troughDefinition.SourceField()
//  		targetFieldStr = troughDefinition.TargetField()
//  		proxy          = &throughProxy{
//  			defs:             defs,
//  			object:           throughInstance,
//  			troughDefinition: troughDefinition,
//  		}
//  	)
//
//  	if proxy.sourceField, ok = defs.Field(sourceFieldStr); !ok {
//  		panic(fmt.Errorf(
//  			"source field %s not found in through model %T: %w",
//  			sourceFieldStr, throughInstance, query_errors.ErrFieldNotFound,
//  		))
//  	}
//
//  	if proxy.targetField, ok = defs.Field(targetFieldStr); !ok {
//  		panic(fmt.Errorf(
//  			"target field %s not found in through model %T: %w",
//  			targetFieldStr, throughInstance, query_errors.ErrFieldNotFound,
//  		))
//  	}
//
//  	return proxy
//  }

type relatedQuerySet[T attrs.Definer] struct {
	source     *ParentInfo
	rel        attrs.Relation
	originalQs QuerySet[T]
	qs         *QuerySet[T]
}

// NewrelatedQuerySet creates a new relatedQuerySet for the given model type.
func newRelatedQuerySet[T attrs.Definer](source *ParentInfo, model T) *relatedQuerySet[T] {
	var qs = GetQuerySet(model)
	return &relatedQuerySet[T]{
		source:     source,
		originalQs: *qs,
		qs:         qs,
	}
}

func (t *relatedQuerySet[T]) createTargets(targets []T) ([]T, error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot create targets")
	}

	return t.originalQs.Clone().BulkCreate(targets)
}

func (t *relatedQuerySet[T]) createThroughObjects(targets []T) ([]Relation, error) {
	if t.rel == nil {
		panic("Relation is nil, cannot create through object")
	}

	var throughModel = t.rel.Through()
	if throughModel == nil {
		panic("Through model is nil, cannot create through object")
	}

	var targetsToSave = make([]T, 0, len(targets))
	for _, target := range targets {
		var (
			defs    = target.FieldDefs()
			primary = defs.Primary()
			pkValue = primary.GetValue()
		)

		if fields.IsZero(pkValue) {
			targetsToSave = append(targetsToSave, target)
		}
	}

	if len(targetsToSave) > 0 {
		var err error
		targets, err = t.createTargets(targetsToSave)
		if err != nil {
			return nil, fmt.Errorf("failed to save targets: %w", err)
		}
	}

	// Create a new instance of the through model
	var (
		throughObj            = throughModel.Model()
		throughSourceFieldStr = throughModel.SourceField()
		throughTargetFieldStr = throughModel.TargetField()

		sourceObject        = t.source.Object.FieldDefs()
		sourceObjectPrimary = sourceObject.Primary()
		sourceObjectPk      = sourceObjectPrimary.GetValue()

		relations     = make([]Relation, 0, len(targets))
		throughModels = make([]attrs.Definer, 0, len(targets))
	)

	for _, target := range targets {
		var (
			// target related values
			targetDefs    = target.FieldDefs()
			targetPrimary = targetDefs.Primary()
			targetPk      = targetPrimary.GetValue()

			// through model values
			ok          bool
			sourceField attrs.Field
			targetField attrs.Field
			newInstance = internal.NewObjectFromIface(throughObj)
			fieldDefs   = newInstance.FieldDefs()
		)

		if sourceField, ok = fieldDefs.Field(throughSourceFieldStr); !ok {
			return nil, query_errors.ErrFieldNotFound
		}
		if targetField, ok = fieldDefs.Field(throughTargetFieldStr); !ok {
			return nil, query_errors.ErrFieldNotFound
		}

		if err := sourceField.SetValue(sourceObjectPk, true); err != nil {
			return nil, err
		}
		if err := targetField.SetValue(targetPk, true); err != nil {
			return nil, err
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

	throughModels, err := GetQuerySet(throughObj).BulkCreate(throughModels)
	if err != nil {
		return nil, err
	}

	return relations, nil
}

func (t *relatedQuerySet[T]) Select(fields ...any) *relatedQuerySet[T] {
	if t.qs == nil {
		panic("QuerySet is nil, cannot call Select()")
	}
	t.qs = t.qs.Select(fields...)
	return t
}

func (t *relatedQuerySet[T]) Filter(key any, vals ...any) *relatedQuerySet[T] {
	t.qs = t.qs.Filter(key, vals...)
	return t
}

func (t *relatedQuerySet[T]) OrderBy(fields ...string) *relatedQuerySet[T] {
	t.qs = t.qs.OrderBy(fields...)
	return t
}

func (t *relatedQuerySet[T]) Limit(limit int) *relatedQuerySet[T] {
	t.qs = t.qs.Limit(limit)
	return t
}

func (t *relatedQuerySet[T]) Offset(offset int) *relatedQuerySet[T] {
	t.qs = t.qs.Offset(offset)
	return t
}

func (t *relatedQuerySet[T]) Get() (*Row[T], error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot call Get()")
	}
	return t.qs.Get()
}

func (t *relatedQuerySet[T]) All() (Rows[T], error) {
	if t.qs == nil {
		panic("QuerySet is nil, cannot call All()")
	}
	return t.qs.All()
}

//
//type RelOneToOneQuerySet[T attrs.Definer] struct {
//	backRef             ThroughRelationValue
//	*relatedQuerySet[T] // Embedding the relatedQuerySet to inherit its methods
//}
//
//func NewRelOneToOneQuerySet[T attrs.Definer](backRef attrs.Binder, model T) *RelOneToOneQuerySet[T] {
//	var parentInfo = backRef.ParentInfo()
//	return &RelOneToOneQuerySet[T]{
//		backRef:         backRef,
//		relatedQuerySet: newRelatedQuerySet(parentInfo, model),
//	}
//}
//
//type RelManyToOneQuerySet[T attrs.Definer] struct {
//	backRef             MultiThroughRelationValue
//	*relatedQuerySet[T] // Embedding the relatedQuerySet to inherit its methods
//}
//
//func NewRelManyToOneQuerySet[T attrs.Definer](backRef attrs.Binder, model T) *RelManyToOneQuerySet[T] {
//	var parentInfo = backRef.ParentInfo()
//	return &RelManyToOneQuerySet[T]{
//		backRef:         backRef,
//		relatedQuerySet: newRelatedQuerySet(parentInfo, model),
//	}
//}
//
