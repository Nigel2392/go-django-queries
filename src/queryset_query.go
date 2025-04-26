package queries

import (
	"github.com/Nigel2392/go-django/src/core/logger"
)

type Query[T1 any, T2 any] interface {
	SQL() string
	Args() []any
	Model() T2
	Exec() (T1, error)
}

type CountQuery[T any] Query[int64, T]
type ExistsQuery[T any] Query[bool, T]
type ValuesListQuery[T any] Query[[][]any, T]

type queryObject[T1 any, T2 any] struct {
	exec  func(sql string, args ...any) (T1, error)
	model T2
	args  []any
	sql   string
}

func (q *queryObject[T1, T2]) SQL() string {
	return q.sql
}

func (q *queryObject[T1, T2]) Args() []any {
	return q.args
}

func (q *queryObject[T1, T2]) Model() T2 {
	return q.model
}

func (q *queryObject[T1, T2]) Exec() (T1, error) {
	logger.Debugf("Query (%T): %s", q.model, q.sql)
	return q.exec(q.sql, q.args...)
}
