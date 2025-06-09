package queries

import (
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/logger"
)

type nullTransaction struct {
	DB
}

func NullTransction() Transaction {
	return &nullTransaction{DB: nil}
}

func (n *nullTransaction) Rollback() error {
	return nil
}

func (n *nullTransaction) Commit() error {
	return nil
}

type dbSpecificTransaction struct {
	Transaction
	dbName string
}

func (c *dbSpecificTransaction) DatabaseName() string {
	return c.dbName
}

type wrappedTransaction struct {
	Transaction
	compiler *genericQueryBuilder
}

func (w *wrappedTransaction) Rollback() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	if w.compiler != nil {
		w.compiler.transaction = nil
	}
	logger.Debugf("Rolling back transaction for %s", w.compiler.DatabaseName())
	return w.Transaction.Rollback()
}

func (w *wrappedTransaction) Commit() error {
	if !w.compiler.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	if w.compiler != nil {
		w.compiler.transaction = nil
	}
	logger.Debugf("Committing transaction for %s", w.compiler.DatabaseName())
	return w.Transaction.Commit()
}
