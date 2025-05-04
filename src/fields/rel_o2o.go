package fields

import (
	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type OneToOneField[T any] struct {
	*RelationField[T]
}

func NewOneToOneField[T any](forModel attrs.Definer, dst any, reverseName string, columnName string, rel queries.Relation) *OneToOneField[T] {
	var f = &OneToOneField[T]{
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
