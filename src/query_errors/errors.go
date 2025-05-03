package query_errors

import "github.com/Nigel2392/go-django/src/core/errs"

const (
	ErrNoDatabase        errs.Error = "No database connection"
	ErrUnknownDriver     errs.Error = "Unknown driver"
	ErrNoTableName       errs.Error = "No table name"
	ErrNoWhereClause     errs.Error = "No where clause in query"
	ErrFieldNull         errs.Error = "Field cannot be null"
	ErrLastInsertId      errs.Error = "Last insert id is not valid"
	ErrUnsupportedLookup errs.Error = "Unsupported lookup type"

	ErrNoResults    errs.Error = "No results found"
	ErrNoRows       errs.Error = "No rows in result set"
	ErrMultipleRows errs.Error = "Multiple rows in result set"

	ErrTransactionStarted     errs.Error = "Transaction already started"
	ErrFailedStartTransaction errs.Error = "Failed to start transaction"
	ErrNoTransaction          errs.Error = "Transaction was not started"
)
