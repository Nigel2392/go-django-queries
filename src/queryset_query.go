package queries

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var (
	_ Query[int64] = &QueryObject[int64]{}
	_ Query[int64] = &wrappedQuery[int64, int64]{}

	LogQueries = true
)

type Query[T1 any] interface {
	SQL() string
	Args() []any
	Model() attrs.Definer
	Exec() (T1, error)
	Compiler() QueryCompiler
}

type CountQuery Query[int64]
type ExistsQuery Query[bool]
type ValuesListQuery Query[[][]any]

type QueryObject[T1 any] struct {
	exec     func(sql string, args ...any) (T1, error)
	model    attrs.Definer
	args     []any
	sql      string
	compiler QueryCompiler
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
	if LogQueries {
		if err != nil {
			logger.Errorf("Query (%T, %T): %s", q.Model(), *new(T1), err.Error())
			return result, err
		}
		logger.Debugf("Query (%T, %T): %s", q.Model(), *new(T1), q.sql)
	}
	return result, err
}

func (q *QueryObject[T1]) Compiler() QueryCompiler {
	return q.compiler
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

func (w *wrappedQuery[T1, T3]) Compiler() QueryCompiler {
	return w.query.Compiler()
}

func (w *wrappedQuery[T1, T3]) Exec() (T3, error) {
	return w.exec(w.query)
}

type ErrorQuery[T any] struct {
	Obj     attrs.Definer
	Compile QueryCompiler
	Err     error
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

func (e *ErrorQuery[T]) Compiler() QueryCompiler {
	return e.Compile
}

func (e *ErrorQuery[T]) Exec() (T, error) {
	return *new(T), e.Err
}
