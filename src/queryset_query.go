package queries

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
)

var (
	_ CompiledQuery[int64] = &QueryObject[int64]{}

	LogQueries = true
)

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
			logger.Errorf("Query (%T, %T): %s", q.Model(), *new(T1), q.sql)
			return result, err
		}
		logger.Debugf("Query (%T, %T): %s", q.Model(), *new(T1), q.sql)
	}
	return result, err
}

func (q *QueryObject[T1]) Compiler() QueryCompiler {
	return q.compiler
}
