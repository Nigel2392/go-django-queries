package fields

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ForeignKeyReverseField[T any] struct {
	*RelationField[T]
}

func NewForeignKeyReverseField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ForeignKeyReverseField[T] {
	var f = &ForeignKeyReverseField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelOneToMany,
			},
		),
	}
	return f
}
