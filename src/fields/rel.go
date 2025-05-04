package fields

import (
	"github.com/Nigel2392/go-django-queries/internal"
	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var (
	_ queries.ForUseInQueriesField = (*RelationField[any])(nil)
	_ queries.RelatedField         = (*RelationField[any])(nil)
	_ internal.CanReverseAlias     = (*RelationField[any])(nil)
)

type rel struct {
	model   queries.RelationTarget
	through queries.RelationTarget
}

func (r *rel) Model() attrs.Definer {
	if r.model == nil {
		return nil
	}
	return r.model.Model()
}

func (r *rel) Through() attrs.Definer {
	if r.through == nil {
		return nil
	}
	return r.through.Model()
}

type RelationField[T any] struct {
	*DataModelField[T]
	rel queries.Relation
	col string
}

func NewRelatedField[T any](forModel attrs.Definer, dst any, reverseName string, columnName string, rel queries.Relation) *RelationField[T] {
	//var (
	//	inst = field.Instance()
	//	defs = inst.FieldDefs()
	//)

	return &RelationField[T]{
		DataModelField: NewDataModelField[T](forModel, dst, reverseName),
		col:            columnName,
		rel:            rel,
	}
}

func (m *RelationField[T]) ForSelectAll() bool {
	return false
}

func (r *RelationField[T]) Relation() queries.Relation {
	return r.rel
}

func (r *RelationField[T]) ColumnName() string {
	return r.col
}

func (r *RelationField[T]) GetTargetField() attrs.Field {
	return r.rel.Target().Field()
}

func (r *RelationField[T]) ReverseAlias() string {
	return r.DataModelField.Name()
}

func (r *RelationField[T]) Rel() attrs.Definer {
	var (
		m2o = r.ForeignKey()
		o2m = r.ForeignKeyReverse()
		m2m = r.ManyToMany()
		oto = r.OneToOne()
	)
	if m2o != nil {
		return m2o
	}
	if m2m != nil {
		return m2m.Through()
	}
	if o2m != nil {
		var through = o2m.Through()
		if through != nil {
			return through
		}
		return o2m.Model()
	}
	if oto != nil {
		var through = oto.Through()
		if through != nil {
			return through
		}
		return oto.Model()
	}
	return nil
}

func (r *RelationField[T]) ForeignKey() attrs.Definer {
	if r.rel.Type() == queries.RelationTypeForeignKey {
		return r.rel.Target().Model()
	}
	return nil
}

func (e *RelationField[T]) ForeignKeyReverse() attrs.Relation {
	if e.rel.Type() == queries.RelationTypeForeignKeyReverse {
		var relTarget = e.rel.Target()
		return &rel{
			model:   relTarget,
			through: relTarget.From(),
		}
	}
	return nil
}

func (e *RelationField[T]) ManyToMany() attrs.Relation {
	if e.rel.Type() == queries.RelationTypeManyToMany {
		var relTarget = e.rel.Target()
		return &rel{
			model:   relTarget,
			through: relTarget.From(),
		}
	}
	return nil
}

func (e *RelationField[T]) OneToOne() attrs.Relation {
	if e.rel.Type() == queries.RelationTypeOneToOne {
		var relTarget = e.rel.Target()
		return &rel{
			model:   relTarget,
			through: relTarget.From(),
		}
	}
	return nil
}

func (r *RelationField[T]) Inject(qs *queries.QuerySet) *queries.QuerySet {
	return qs
}
