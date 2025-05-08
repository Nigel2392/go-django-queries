package fields

import (
	"fmt"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/migrator"
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
	field    attrs.Field
	fieldStr string
	prev     attrs.RelationTarget
}

func (r *relationTarget) From() attrs.RelationTarget {
	return r.prev
}

func (r *relationTarget) Model() attrs.Definer {
	return r.model
}

func (r *relationTarget) Field() attrs.Field {
	if r.field != nil {
		return r.field
	}

	var defs = r.model.FieldDefs()
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

	return &RelationField[T]{
		DataModelField: NewDataModelField[T](forModel, dst, reverseName),
		col:            columnName,
		name:           name,
		rel:            rel,
	}
}

func (m *RelationField[T]) Name() string {
	return m.name
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

func (r *RelationField[T]) GetTargetField() attrs.Field {
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
	return r.DataModelField.Name()
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

func (r *RelationField[T]) Inject(qs *queries.QuerySet) *queries.QuerySet {
	return qs
}
