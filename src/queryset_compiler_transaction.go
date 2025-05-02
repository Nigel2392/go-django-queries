package queries

import (
	"database/sql"
)

type wrappedTransaction struct {
	*sql.Tx
	compiler *genericQueryBuilder
}

func (w *wrappedTransaction) Rollback() error {
	if !w.compiler.InTransaction() {
		return ErrNoTransaction
	}
	w.compiler.transaction = nil
	return w.Tx.Rollback()
}

func (w *wrappedTransaction) Commit() error {
	if !w.compiler.InTransaction() {
		return ErrNoTransaction
	}
	w.compiler.transaction = nil
	return w.Tx.Commit()
}
