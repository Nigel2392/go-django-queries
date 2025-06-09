package fields

import (
	"context"
	"fmt"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/migrator"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var (
	_ queries.ForUseInQueriesField = (*RelationField[any])(nil)
	_ attrs.CanRelatedName         = (*RelationField[any])(nil)
)

type RelationField[T any] struct {
	*DataModelField[T]
	rel  attrs.Relation
	name string
	col  string
}

type typedRelation struct {
	attrs.Relation
	typ attrs.RelationType
}

func (r *typedRelation) Type() attrs.RelationType {
	return r.typ
}

type wrappedRelation struct {
	attrs.Relation
	from attrs.RelationTarget
}

func (r *wrappedRelation) From() attrs.RelationTarget {
	if r.from == nil {
		return r.Relation.From()
	}
	return r.from
}

type relationTarget struct {
	model    attrs.Definer
	field    attrs.FieldDefinition
	fieldStr string
	prev     attrs.RelationTarget
}

func (r *relationTarget) From() attrs.RelationTarget {
	return r.prev
}

func (r *relationTarget) Model() attrs.Definer {
	return r.model
}

func (r *relationTarget) Field() attrs.FieldDefinition {
	if r.field != nil {
		return r.field
	}

	var meta = attrs.GetModelMeta(r.model)
	var defs = meta.Definitions()
	if r.fieldStr != "" {
		var ok bool
		r.field, ok = defs.Field(r.fieldStr)
		if !ok {
			panic(fmt.Errorf("field %q not found in model %T", r.fieldStr, r.model))
		}
	} else {
		r.field = defs.Primary()
	}

	return r.field
}

func NewRelatedField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *RelationField[T] {
	//var (
	//	inst = field.Instance()
	//	defs = inst.FieldDefs()
	//)

	var f = &RelationField[T]{
		DataModelField: NewDataModelField[T](forModel, dst, name),
		col:            columnName,
		name:           reverseName,
		rel:            rel,
	}
	f.DataModelField.fieldRef = f // Set the field reference to itself
	f.DataModelField.setupInitialVal()
	return f
}

func (m *RelationField[T]) Name() string {
	return m.DataModelField.Name()
}

func (m *RelationField[T]) ForSelectAll() bool {
	return false
}

func (r *RelationField[T]) ColumnName() string {
	if r.col == "" {
		var from = r.rel.From()
		if from != nil {
			return from.Field().ColumnName()
		}
	}
	return r.col
}

type (
	saveableDefiner interface {
		attrs.Definer
		Save(ctx context.Context) error
	}
	saveableRelation interface {
		queries.Relation
		Save(ctx context.Context, parent attrs.Definer) error
	}
	canSetup interface {
		Setup(def attrs.Definer) error
	}
)

func (r *RelationField[T]) Save(ctx context.Context, parent attrs.Definer) error {
	var val = r.GetValue()
	if val == nil {
		return nil
	}

	if canSetup, ok := val.(canSetup); ok {
		if err := canSetup.Setup(val.(attrs.Definer)); err != nil {
			return fmt.Errorf("failed to setup value for relation %s: %w", r.Name(), err)
		}
	}

	switch r.rel.Type() {
	case attrs.RelManyToMany, attrs.RelOneToMany:
		return fmt.Errorf(
			"cannot save relation %s with type %s: %w",
			r.Name(), r.rel.Type(), query_errors.ErrNotImplemented,
		)

	case attrs.RelOneToOne:

		switch v := val.(type) {
		case saveableDefiner:
			return v.Save(ctx)
		case saveableRelation:
			return v.Save(ctx, parent)
		}

	case attrs.RelManyToOne:
		if v, ok := val.(saveableDefiner); ok {
			return v.Save(ctx)
		}
	}

	return fmt.Errorf(
		"cannot save relation %s with type %s, value %T does not implement saveableDefiner or saveableRelation: %w",
		r.Name(), r.rel.Type(), val, query_errors.ErrNotImplemented,
	)
}

func (r *RelationField[T]) GetTargetField() attrs.FieldDefinition {
	var targetField = r.rel.Field()
	if targetField == nil {
		var defs = r.rel.Model().FieldDefs()
		return defs.Primary()
	}
	return targetField
}

func (r *RelationField[T]) IsReverse() bool {
	var targetField = r.GetTargetField()
	if targetField == nil || targetField.IsPrimary() {
		return false
	}
	return true
}

func (r *RelationField[T]) Attrs() map[string]any {
	var atts = make(map[string]any)
	atts[attrs.AttrNameKey] = r.Name()
	atts[migrator.AttrUseInDBKey] = r.rel.Through() == nil && !r.IsReverse()
	return atts
}

func (r *RelationField[T]) RelatedName() string {
	return r.name
}

func (r *RelationField[T]) Rel() attrs.Relation {
	return &wrappedRelation{
		Relation: r.rel,
		from: &relationTarget{
			model: r.Instance(),
			field: r,
		},
	}
}
