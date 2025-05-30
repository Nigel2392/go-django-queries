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

const (
	// MetaUniqueTogetherKey is the key used to store the unique together
	// fields in the model's metadata.
	//
	// It is used to determine which fields are unique together in the model
	// and can be used to enforce uniqueness, generate SQL clauses for selections,
	// and to generate unique keys for the model in code.
	MetaUniqueTogetherKey = "unique_together"
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

func getTargetField(f any, targetDefs attrs.Definitions) attrs.Field {
	if f == nil {
		goto retTarget
	}

	if rf, ok := f.(RelatedField); ok {
		if targetField := rf.GetTargetField(); targetField != nil {
			return targetField
		}
	}

retTarget:
	return targetDefs.Primary()
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
func ForSelectAll(f attrs.FieldDefinition) bool {
	if f == nil {
		return false
	}
	if f, ok := f.(ForUseInQueriesField); ok {
		return f.ForSelectAll()
	}
	return true
}

func ForSelectAllFields[T any](fields any) []T {
	switch fieldsValue := fields.(type) {
	case []attrs.Field:
		var result = make([]T, 0, len(fieldsValue))
		for _, f := range fieldsValue {
			if ForSelectAll(f) {
				result = append(result, f.(T))
			}
		}
		return result
	case []attrs.FieldDefinition:
		var result = make([]T, 0, len(fieldsValue))
		for _, f := range fieldsValue {
			if ForSelectAll(f) {
				result = append(result, f.(T))
			}
		}
		return result
	case attrs.Definer:
		var defs = fieldsValue.FieldDefs()
		var fields = defs.Fields()
		return ForSelectAllFields[T](fields)
	case attrs.Definitions:
		var fields = fieldsValue.Fields()
		return ForSelectAllFields[T](fields)
	case attrs.StaticDefinitions:
		var fields = fieldsValue.Fields()
		return ForSelectAllFields[T](fields)
	default:
		panic(fmt.Errorf("cannot get ForSelectAllFields from %T", fields))
	}
}

// A base interface for relations.
//
// This interface should only be used for OneToOne relations with a through table,
// or for ManyToMany relations with a through table.
//
// It should contain the actual instances of the models involved in the relation,
// and the through model if applicable.
type Relation interface {

	// The target model of the relation.
	Model() attrs.Definer

	// The through model of the relation.
	Through() attrs.Definer
}

// ParentInfo holds information about a relation's parent model instance
// and the field on the parent model that holds the relation.
type ParentInfo struct {
	Object attrs.Definer
	Field  attrs.Field
}

// canPrimaryKey is an interface that can be implemented by a model field's value
type canPrimaryKey interface {
	// PrimaryKey returns the primary key of the relation.
	PrimaryKey() any
}

// A model field's value can adhere to this interface to indicate that the
// field's relation value can be set or retrieved.
//
// This is used for OneToOne relations without a through table,
// if a through table is specified, the field's value should be of type [ThroughRelationValue]
//
// A default implementation is provided with the [RelO2O] type.
type RelationValue interface {
	attrs.Binder
	ParentInfo() *ParentInfo
	GetValue() (obj attrs.Definer)
	SetValue(instance attrs.Definer)
}

// A model field's value can adhere to this interface to indicate that the
// field's relation values can be set or retrieved.
//
// This is used for OneToMany relations without a through table,
// if a through table is specified, the field's value should be of type [MultiThroughRelationValue]
type MultiRelationValue interface {
	attrs.Binder
	ParentInfo() *ParentInfo
	GetValues() []attrs.Definer
	SetValues(instances []attrs.Definer)
}

// A model field's value can adhere to this interface to indicate that the
// field's relation value can be set or retrieved.
//
// This is used for OneToOne relations with a through table,
// if no through table is specified, the field's value should be of type [attrs.Definer]
//
// A default implementation is provided with the [RelO2O] type.
type ThroughRelationValue interface {
	attrs.Binder
	ParentInfo() *ParentInfo
	GetValue() (obj attrs.Definer, through attrs.Definer)
	SetValue(instance attrs.Definer, through attrs.Definer)
}

// A model field's value can adhere to this interface to indicate that the
// field's relation values can be set or retrieved.
//
// This is used for ManyToMany relations with a through table,
// a through table is required for ManyToMany relations.
//
// A default implementation is provided with the [RelM2M] type.
type MultiThroughRelationValue interface {
	attrs.Binder
	ParentInfo() *ParentInfo
	SetValues(instances []Relation)
	GetValues() []Relation
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

// A model can adhere to this interface to indicate fields which are
// unique together.
type UniqueTogetherDefiner interface {
	attrs.Definer
	UniqueTogether() [][]string
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

// QuerySetChanger is an interface that can be implemented by models to indicate
// that the queryset should be changed when the model is used in a queryset.
type QuerySetChanger interface {
	attrs.Definer

	// ChangeQuerySet is called when the model is used in a queryset.
	// It should return a new queryset that will be used to execute the query.
	ChangeQuerySet(qs *QuerySet[attrs.Definer]) *QuerySet[attrs.Definer]
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
	FieldInfo[attrs.Field]
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

	// FormatColumn formats the given field column to be used in a query.
	// It should return the column name with the quotes applied.
	// Expressions should use this method to format the column name.
	FormatColumn(tableColumn *expr.TableColumn) (string, []any)

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
		internals *QuerySetInternals,
	) CompiledQuery[[][]interface{}]

	// BuildCountQuery builds a count query with the given parameters.
	BuildCountQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		internals *QuerySetInternals,
	) CompiledQuery[int64]

	// BuildCreateQuery builds a create query with the given parameters.
	BuildCreateQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		internals *QuerySetInternals,
		objects []*FieldInfo[attrs.Field],
		values []any,
	) CompiledQuery[[][]interface{}]

	BuildUpdateQuery(
		ctx context.Context,
		qs *GenericQuerySet,
		internals *QuerySetInternals,
		objects []UpdateInfo,
	) CompiledQuery[int64]

	// BuildUpdateQuery builds an update query with the given parameters.
	BuildDeleteQuery(
		ctx context.Context,
		qs *QuerySet[attrs.Definer],
		internals *QuerySetInternals,
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
