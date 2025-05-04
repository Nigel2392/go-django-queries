package fields

import (
	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ManyToManyField[T any] struct {
	*RelationField[T]
}

func NewManyToManyField[T any](forModel attrs.Definer, dst any, reverseName string, columnName string, rel queries.Relation) *ManyToManyField[T] {
	var f = &ManyToManyField[T]{
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
