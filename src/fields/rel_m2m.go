package fields

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ManyToManyField[T any] struct {
	*RelationField[T]
}

func NewManyToManyField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ManyToManyField[T] {
	var f = &ManyToManyField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelManyToMany,
			},
		),
	}
	return f
}

type ManyToManyReverseField[T any] struct {
	*RelationField[T]
}

func NewManyToManyReverseField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ManyToManyReverseField[T] {
	var f = &ManyToManyReverseField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelManyToMany,
			},
		),
	}
	return f
}
