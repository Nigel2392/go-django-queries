package queries

import (
	"github.com/Nigel2392/go-django-queries/src/query_errors"
)

type nullTransaction struct {
	Transaction
}

func (n *nullTransaction) Rollback() error {
	return nil
}

func (n *nullTransaction) Commit() error {
	return nil
}

type wrappedTransaction struct {
	Transaction
	compiler *genericQueryBuilder
	expired  bool
}

func (w *wrappedTransaction) Rollback() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	w.expired = true
	w.compiler.transaction = nil
	return w.Transaction.Rollback()
}

func (w *wrappedTransaction) Commit() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	w.expired = true
	w.compiler.transaction = nil
	return w.Transaction.Commit()
}
