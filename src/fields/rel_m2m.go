package fields

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

//
//type RelatedQuerySet[T any] interface {
//	Filter(any, ...any) RelatedQuerySet[T]
//	OrderBy(...string) RelatedQuerySet[T]
//	Reverse() RelatedQuerySet[T]
//	Limit(int) RelatedQuerySet[T]
//	Offset(int) RelatedQuerySet[T]
//
//	All() ([]T, error)
//	Get() (T, bool)
//	First() (T, error)
//	Last() (T, error)
//	Exists() (bool, error)
//	Count() (int64, error)
//}
//
//type ManyToManyRelation[T any] struct {
//	cached  orderedmap.OrderedMap[any, T]
//	latestQ queries.QueryInfo
//}
//
//func (r *ManyToManyRelation[T]) Set(objs []T) error {
//
//}
//
//func (r *ManyToManyRelation[T]) Add(obj T) error {
//
//}
//
//func (r *ManyToManyRelation[T]) Remove(obj T) error {
//
//}
//
//func (r *ManyToManyRelation[T]) Clear() error {
//
//}

type ManyToManyField struct {
	*MultipleRelationField
}

func NewManyToManyField(forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ManyToManyField {
	var f = &ManyToManyField{
		MultipleRelationField: NewMultipleRelatedField(
			forModel,
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
