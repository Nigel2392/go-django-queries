package queries

import (
	"github.com/Nigel2392/go-django-queries/src/query_errors"
)

type wrappedTransaction struct {
	Transaction
	compiler *genericQueryBuilder
}

func (w *wrappedTransaction) Rollback() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	w.compiler.transaction = nil
	return w.Transaction.Rollback()
}

func (w *wrappedTransaction) Commit() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	w.compiler.transaction = nil
	return w.Transaction.Commit()
}
