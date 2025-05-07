package fields

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type OneToOneField[T any] struct {
	*RelationField[T]
}

func NewOneToOneField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *OneToOneField[T] {
	var f = &OneToOneField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelOneToOne,
			},
		),
	}
	return f
}

type OneToOneReverseField[T any] struct {
	*RelationField[T]
}

func NewOneToOneReverseField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *OneToOneReverseField[T] {
	var f = &OneToOneReverseField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelOneToOne,
			},
		),
	}
	return f
}
