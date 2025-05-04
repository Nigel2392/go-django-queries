package fields

import (
	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ForeignKeyReverseField[T any] struct {
	*RelationField[T]
}

func NewForeignKeyReverseField[T any](forModel attrs.Definer, dst any, reverseName string, columnName string, rel queries.Relation) *ForeignKeyReverseField[T] {
	var f = &ForeignKeyReverseField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			reverseName,
			columnName,
			rel,
		),
	}
	return f
}
