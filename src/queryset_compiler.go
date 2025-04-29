package queries

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Transaction interface {
	DB
	Commit() error
	Rollback() error
}

type QueryCompiler interface {
	// DB returns the database connection used by the query compiler.
	//
	// If a transaction was started, it will return the transaction instead of the database connection.
	DB() DB

	// Quote returns the quotes used by the database.
	//
	// This is used to quote table and field names.
	// For example, MySQL uses backticks (`) and PostgreSQL uses double quotes (").
	Quote() (front string, back string)

	// SupportsReturning returns the type of returning supported by the database.
	// It can be one of the following:
	//
	// - SupportsReturningNone: no returning supported
	// - SupportsReturningLastInsertId: last insert id supported
	// - SupportsReturningColumns: returning columns supported
	SupportsReturning() SupportsReturning

	// StartTransaction starts a new transaction.
	StartTransaction(ctx context.Context) (Transaction, error)

	// CommitTransaction commits the current ongoing transaction.
	CommitTransaction() error

	// RollbackTransaction rolls back the current ongoing transaction.
	RollbackTransaction() error

	// InTransaction returns true if the current query compiler is in a transaction.
	InTransaction() bool

	// BuildSelectQuery builds a select query with the given parameters.
	BuildSelectQuery(
		ctx context.Context,
		qs *QuerySet,
		fields []FieldInfo,
		where []Expression,
		having []Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
		orderBy []OrderBy,
		limit int,
		offset int,
		union []Union,
		forUpdate bool,
		distinct bool,
	) Query[[][]interface{}]

	// BuildCountQuery builds a count query with the given parameters.
	BuildCountQuery(
		ctx context.Context,
		qs *QuerySet,
		where []Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
		limit int,
		offset int,
	) CountQuery

	// BuildCreateQuery builds a create query with the given parameters.
	BuildCreateQuery(
		ctx context.Context,
		qs *QuerySet,
		fields FieldInfo,
		primary attrs.Field,
		values []any,
	) Query[[]interface{}]

	// BuildValuesListQuery builds a values list query with the given parameters.
	BuildUpdateQuery(
		ctx context.Context,
		qs *QuerySet,
		fields FieldInfo,
		where []Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
		values []any,
	) CountQuery

	// BuildUpdateQuery builds an update query with the given parameters.
	BuildDeleteQuery(
		ctx context.Context,
		qs *QuerySet,
		where []Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
	) CountQuery
}

var compilerRegistry = map[reflect.Type]func(model attrs.Definer) QueryCompiler{}

func RegisterCompiler(driver driver.Driver, compiler func(model attrs.Definer) QueryCompiler) {
	var driverType = reflect.TypeOf(driver)
	if driverType == nil {
		panic("driver is nil")
	}

	compilerRegistry[driverType] = compiler
}

func Compiler(model attrs.Definer) QueryCompiler {
	var db = django.ConfigGet[interface{ Driver() driver.Driver }](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)
	if db == nil {
		panic(ErrNoDatabase)
	}

	var driverType = reflect.TypeOf(db.Driver())
	if driverType == nil {
		panic("driver is nil")
	}

	var compiler, ok = compilerRegistry[driverType]
	if !ok {
		panic(fmt.Errorf("no compiler registered for driver %T", db.Driver()))
	}

	return compiler(model)
}
