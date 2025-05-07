package queries

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django-queries/src/expr"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type AliasField interface {
	attrs.Field
	Alias() string
}

type VirtualField interface {
	AliasField
	SQL(d driver.Driver, m attrs.Definer, quote string) (string, []any)
}

type InjectorField interface {
	attrs.Field
	Inject(qs *QuerySet) *QuerySet
}

type ForUseInQueriesField interface {
	attrs.Field
	// ForUseInQueries returns true if the field is for use in queries.
	// This is used to determine if the field should be included in the query.
	// If the field does not implement this method, it is assumed to be for use in queries.
	ForSelectAll() bool
}

type RelatedField interface {
	attrs.Field

	// This is used to determine the column name for the field, for example for a through table.
	GetTargetField() attrs.Field
}

func ForSelectAll(f attrs.Field) bool {
	if f == nil {
		return false
	}
	if f, ok := f.(ForUseInQueriesField); ok {
		return f.ForSelectAll()
	}
	return true
}

type DataModel interface {
	HasQueryValue(key string) bool
	GetQueryValue(key string) (any, bool)
	SetQueryValue(key string, value any) error
}

type QuerySetDefiner interface {
	attrs.Definer

	GetQuerySet() *QuerySet
}

type QuerySetDatabaseDefiner interface {
	attrs.Definer

	QuerySetDatabase() string
}

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

type Query[T1 any] interface {
	SQL() string
	Args() []any
	Model() attrs.Definer
	Exec() (T1, error)
	Compiler() QueryCompiler
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
		where []expr.Expression,
		having []expr.Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
		orderBy []OrderBy,
		limit int,
		offset int,
		forUpdate bool,
		distinct bool,
	) Query[[][]interface{}]

	// BuildCountQuery builds a count query with the given parameters.
	BuildCountQuery(
		ctx context.Context,
		qs *QuerySet,
		where []expr.Expression,
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
		where []expr.Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
		values []any,
	) CountQuery

	// BuildUpdateQuery builds an update query with the given parameters.
	BuildDeleteQuery(
		ctx context.Context,
		qs *QuerySet,
		where []expr.Expression,
		joins []JoinDef,
		groupBy []FieldInfo,
	) CountQuery
}

var compilerRegistry = make(map[reflect.Type]func(model attrs.Definer, defaultDB string) QueryCompiler)

func RegisterCompiler(driver driver.Driver, compiler func(model attrs.Definer, defaultDB string) QueryCompiler) {
	var driverType = reflect.TypeOf(driver)
	if driverType == nil {
		panic("driver is nil")
	}

	compilerRegistry[driverType] = compiler
}

func Compiler(model attrs.Definer, defaultDB string) QueryCompiler {
	if defaultDB == "" {
		defaultDB = django.APPVAR_DATABASE
	}

	var db = django.ConfigGet[interface{ Driver() driver.Driver }](
		django.Global.Settings,
		defaultDB,
	)
	if db == nil {
		panic(fmt.Errorf(
			"no database connection found for %q",
			defaultDB,
		))
	}

	var driverType = reflect.TypeOf(db.Driver())
	if driverType == nil {
		panic("driver is nil")
	}

	var compiler, ok = compilerRegistry[driverType]
	if !ok {
		panic(fmt.Errorf("no compiler registered for driver %T", db.Driver()))
	}

	return compiler(model, defaultDB)
}
