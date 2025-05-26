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

	// Register all schema editors for the migrator package.
	_ "github.com/Nigel2392/go-django-queries/src/migrator/sql/mysql"
	_ "github.com/Nigel2392/go-django-queries/src/migrator/sql/postgres"
	_ "github.com/Nigel2392/go-django-queries/src/migrator/sql/sqlite"
)

// A field can adhere to this interface to indicate that the field should be
// aliased when generating the SQL for the field.
//
// For example: this is used in annotations to alias the field name.
type AliasField interface {
	attrs.Field
	Alias() string
}

// A field can adhere to this interface to indicate that the field should be
// rendered as SQL.
//
// For example: this is used in fields.ExpressionField to render the expression as SQL.
type VirtualField interface {
	SQL(inf *expr.ExpressionInfo) (string, []any)
}

// RelatedField is an interface that can be implemented by fields to indicate
// that the field is a related field.
//
// For example, this is used in fields.RelationField to determine the column name for the target field.
//
// If `GetTargetField()` returns nil, the primary field of the target model should be used instead.
type RelatedField interface {
	attrs.Field

	// This is used to determine the column name for the field, for example for a through table.
	GetTargetField() attrs.Field

	RelatedName() string
}

// ForUseInQueriesField is an interface that can be implemented by fields to indicate
// that the field should be included in the query.
//
// For example, this is used in fields.RelationField to exclude the relation from the query,
// otherwise scanning errors will occur.
//
// This is mostly for fields that do not actually exist in the database, I.E. reverse fk, o2o
type ForUseInQueriesField interface {
	attrs.Field
	// ForUseInQueries returns true if the field is for use in queries.
	// This is used to determine if the field should be included in the query.
	// If the field does not implement this method, it is assumed to be for use in queries.
	ForSelectAll() bool
}

// ForSelectAll returns true if the field should be selected in the query.
//
// If the field is nil, it returns false.
//
// If the field is a ForUseInQueriesField, it returns the result of `ForSelectAll()`.
//
// Otherwise, it returns true.
func ForSelectAll(f attrs.Field) bool {
	if f == nil {
		return false
	}
	if f, ok := f.(ForUseInQueriesField); ok {
		return f.ForSelectAll()
	}
	return true
}

type Relation interface {
	Model() attrs.Definer
	Through() attrs.Definer
}

type SettableThroughRelation interface {
	SetValue(instance attrs.Definer, through attrs.Definer)
}

type SettableMultiThroughRelation interface {
	SetValues(instances []Relation)
}

// Annotations from the database are stored in the `Row` struct, and if the
// model has a `ModelDataStore()` method that implements this interface,
// annotated values will be stored there too.
//
// Relations are also stored in the model's data store.
type ModelDataStore interface {
	HasValue(key string) bool
	GetValue(key string) (any, bool)
	SetValue(key string, value any) error
	DeleteValue(key string) error
}

// A model can adhere to this interface to indicate that the queries package
// should use the model to store and retrieve annotated values.
//
// Relations will also be stored here.
type DataModel interface {
	ModelDataStore() ModelDataStore
}

// A model can adhere to this interface to indicate that the queries package
// should not automatically save or delete the model to/from the database when
// `django/models.SaveObject()` or `django/models.DeleteObject()` is called.
type ForUseInQueries interface {
	attrs.Definer
	ForUseInQueries() bool
}

// A model can adhere to this interface to indicate that the queries package
// should use the queryset returned by `GetQuerySet()` to execute the query.
//
// Calling `queries.Objects()` with a model that implements this interface will
// return the queryset returned by `GetQuerySet()`.
type QuerySetDefiner interface {
	attrs.Definer

	GetQuerySet() *QuerySet[attrs.Definer]
}

// A model can adhere to this interface to indicate that the queries package
// should use the database returned by `QuerySetDatabase()` to execute the query.
//
// The database should be retrieved from the django.Global.Settings object using the returned key.
type QuerySetDatabaseDefiner interface {
	attrs.Definer

	QuerySetDatabase() string
}

// This interface is compatible with `*sql.DB` and `*sql.Tx`.
//
// It is used for simple transaction management in the queryset.
//
// If a transaction was started, the queryset will return the transaction instead of the database connection.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// This interface is compatible with `*sql.Tx`.
//
// It is used for simple transaction management in the queryset.
type Transaction interface {
	DB
	Commit() error
	Rollback() error
}

// A QueryInfo interface is used to retrieve information about a query.
//
// It is possible to introspect the queries' SQL, arguments, model, and compiler.
type QueryInfo interface {
	SQL() string
	Args() []any
	Model() attrs.Definer
	Compiler() QueryCompiler
}

// A CompiledQuery interface is used to execute a query.
//
// It is possible to execute the query and retrieve the results.
//
// The compiler will generally return a CompiledQuery interface,
// which the queryset will then store to be used as result on `LatestQuery()`.
type CompiledQuery[T1 any] interface {
	QueryInfo
	Exec() (T1, error)
}

// A compiledQuery which returns the number of rows affected by the query.
type CompiledCountQuery CompiledQuery[int64]

// A compiledQuery which returns a boolean indicating if any rows were affected by the query.
type CompiledExistsQuery CompiledQuery[bool]

// A compiledQuery which returns a list of values from the query.
type CompiledValuesListQuery CompiledQuery[[][]any]

type UpdateInfo struct {
	Field  FieldInfo
	Where  []expr.LogicalExpression
	Joins  []JoinDef
	Values []any
}

// A QueryCompiler interface is used to compile a query.
//
// It should be able to generate SQL queries and execute them.
//
// It does not need to know about the model nor its field types.
type QueryCompiler interface {
	// DatabaseName returns the name of the database connection used by the query compiler.
	//
	// This is the name of the database connection as defined in the django.Global.Settings object.
	DatabaseName() string

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

	// WithTransaction wraps the transaction and binds it to the compiler.
	WithTransaction(tx Transaction) (Transaction, error)

	// CommitTransaction commits the current ongoing transaction.
	CommitTransaction() error

	// RollbackTransaction rolls back the current ongoing transaction.
	RollbackTransaction() error

	// InTransaction returns true if the current query compiler is in a transaction.
	InTransaction() bool

	// BuildSelectQuery builds a select query with the given parameters.
	BuildSelectQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		fields []FieldInfo,
		where []expr.LogicalExpression,
		having []expr.LogicalExpression,
		joins []JoinDef,
		groupBy []FieldInfo,
		orderBy []OrderBy,
		limit int,
		offset int,
		forUpdate bool,
		distinct bool,
	) CompiledQuery[[][]interface{}]

	// BuildCountQuery builds a count query with the given parameters.
	BuildCountQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		where []expr.LogicalExpression,
		joins []JoinDef,
		groupBy []FieldInfo,
		limit int,
		offset int,
	) CompiledQuery[int64]

	// BuildCreateQuery builds a create query with the given parameters.
	BuildCreateQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		primary attrs.Field,
		objects []FieldInfo,
		values []any,
	) CompiledQuery[[][]interface{}]

	BuildUpdateQuery(
		ctx context.Context,
		qs *GenericQuerySet,
		objects []UpdateInfo,
	) CompiledQuery[int64]

	// BuildUpdateQuery builds an update query with the given parameters.
	BuildDeleteQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		where []expr.LogicalExpression,
		joins []JoinDef,
		groupBy []FieldInfo,
	) CompiledQuery[int64]
}

var compilerRegistry = make(map[reflect.Type]func(model attrs.Definer, defaultDB string) QueryCompiler)

// RegisterCompiler registers a compiler for a given driver.
//
// It should be used in the init() function of the package that implements the compiler.
//
// The compiler function should take a model and a default database name as arguments,
// and return a QueryCompiler.
//
// The default database name is used to determine the database connection to use and
// retrieve from the django.Global.Settings object.
func RegisterCompiler(driver driver.Driver, compiler func(model attrs.Definer, defaultDB string) QueryCompiler) {
	var driverType = reflect.TypeOf(driver)
	if driverType == nil {
		panic("driver is nil")
	}

	compilerRegistry[driverType] = compiler
}

// Compiler returns a QueryCompiler for the given model and default database name.
//
// If the default database name is empty, it will use the APPVAR_DATABASE setting.
//
// If the database is not found in the settings, it will panic.
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
