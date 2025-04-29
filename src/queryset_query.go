package queries

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var (
	_ Query[int64] = &QueryObject[int64]{}
	_ Query[int64] = &wrappedQuery[int64, int64]{}
)

type Query[T1 any] interface {
	SQL() string
	Args() []any
	Model() attrs.Definer
	Exec() (T1, error)
}

type CountQuery Query[int64]
type ExistsQuery Query[bool]
type ValuesListQuery Query[[][]any]

type QueryObject[T1 any] struct {
	exec  func(sql string, args ...any) (T1, error)
	model attrs.Definer
	args  []any
	sql   string
}

func (q *QueryObject[T1]) SQL() string {
	return q.sql
}

func (q *QueryObject[T1]) Args() []any {
	return q.args
}

func (q *QueryObject[T1]) Model() attrs.Definer {
	return q.model
}

func (q *QueryObject[T1]) Exec() (T1, error) {
	var result, err = q.exec(q.sql, q.args...)
	if err != nil {
		logger.Errorf("Query (%T, %T): %s", q.Model(), *new(T1), err.Error())
		return result, err
	}
	logger.Debugf("Query (%T, %T): %s", q.Model(), *new(T1), q.sql)
	return result, nil
}

type wrappedQuery[T1, T3 any] struct {
	exec  func(q Query[T1]) (T3, error)
	query Query[T1]
}

func (w *wrappedQuery[T1, T3]) SQL() string {
	return w.query.SQL()
}

func (w *wrappedQuery[T1, T3]) Args() []any {
	return w.query.Args()
}

func (w *wrappedQuery[T1, T3]) Model() attrs.Definer {
	return w.query.Model()
}

func (w *wrappedQuery[T1, T3]) Exec() (T3, error) {
	return w.exec(w.query)
}

type ErrorQuery[T any] struct {
	Obj attrs.Definer
	Err error
}

func (e *ErrorQuery[T]) SQL() string {
	return ""
}

func (e *ErrorQuery[T]) Args() []any {
	return nil
}

func (e *ErrorQuery[T]) Model() attrs.Definer {
	return e.Obj
}

func (e *ErrorQuery[T]) Exec() (T, error) {
	return *new(T), e.Err
}
