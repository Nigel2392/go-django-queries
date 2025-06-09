package queries

import (
	"context"
	"fmt"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/pkg/errors"
)

type transactionContextKey struct{}

type transactionContextValue struct {
	Transaction  Transaction
	DatabaseName string
}

func transactionFromContext(ctx context.Context) (Transaction, string, bool) {
	var tx, ok = ctx.Value(transactionContextKey{}).(*transactionContextValue)
	if !ok {
		return nil, "", false
	}
	return tx.Transaction, tx.DatabaseName, tx.Transaction != nil
}

func transactionToContext(ctx context.Context, tx Transaction, dbName string) context.Context {
	if tx == nil {
		panic("transactionToContext: transaction is nil")
	}
	return context.WithValue(ctx, transactionContextKey{}, &transactionContextValue{
		Transaction:  tx,
		DatabaseName: dbName,
	})
}

func StartTransaction(ctx context.Context, database ...string) (context.Context, DatabaseSpecificTransaction, error) {
	var (
		databaseName   = getDatabaseName(nil, database...)
		tx, dbName, ok = transactionFromContext(ctx)
		err            error
	)

	// If the context already has a transaction, use it.
	if ok && (dbName == "" || dbName == databaseName) {
		// return a null transaction when there is already a transaction in the context
		// this will make sure that RollBack and Commit are no-ops
		return ctx, &dbSpecificTransaction{&nullTransaction{tx}, databaseName}, nil
	}

	// Otherwise, start a new transaction.
	var compiler = Compiler(databaseName)
	tx, err = compiler.StartTransaction(ctx)
	if err != nil {
		return ctx, nil, errors.Wrap(err, "StartTransaction: failed to start transaction")
	}

	ctx = transactionToContext(ctx, tx, compiler.DatabaseName())
	return ctx, &dbSpecificTransaction{tx, databaseName}, nil
}

func RunInTransaction[T attrs.Definer](c context.Context, fn func(NewQuerySet ObjectsFunc[T]) (commit bool, err error), database ...string) error {
	var panicFromNewQuerySet error
	var comitted bool

	// If the context already has a transaction, use it.
	var ctx, transaction, err = StartTransaction(c, database...)
	if err != nil {
		return errors.Wrap(err, "RunInTransaction: failed to start transaction")
	}

	var dbName = transaction.DatabaseName()
	// a constructor function to create a new QuerySet with the given model
	// and then bind the transaction to it.
	var newQuerySetFunc = func(model T) *QuerySet[T] {
		var qs = GetQuerySet(model)

		// a transaction cannot be started if the database name is different
		// cross-database transactions are not supported
		var databaseName = qs.compiler.DatabaseName()
		if dbName != databaseName {
			panicFromNewQuerySet = fmt.Errorf(
				"RunInTransaction, %q != %q: %w",
				dbName, databaseName,
				query_errors.ErrCrossDatabaseTransaction,
			)
			panic(panicFromNewQuerySet)
		}

		return qs.WithContext(ctx)
	}

	// rollback the transaction if anything bad happens or the transaction is not committed.
	// this should do nothing if the transaction is already committed.
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("RunInTransaction: panic recovered: %v", rec)
		}

		if transaction != nil && !comitted {
			if err := transaction.Rollback(); err != nil {
				logger.Errorf("RunInTransaction: failed to rollback transaction: %v", err)
			}
		}
	}()

	// if the function returns an error, the transaction will be rolled back
	commit, err := fn(newQuerySetFunc)
	if err != nil {
		return errors.Wrap(err, "RunInTransaction: function returned an error")
	}

	// if the transaction is nil, it means that no transaction was started
	// i.e. no newQuerySetFunc was called
	if transaction == nil {
		return query_errors.ErrNoTransaction
	}

	if commit {
		// commit the transaction if everything went well
		err = transaction.Commit()
		if err != nil {
			return errors.Wrap(err, "RunInTransaction: failed to commit transaction")
		}
		comitted = true
	}

	return nil
}
