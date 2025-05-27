# Interfaces

## 🔌 Field Interfaces

Field interfaces are used to determine how a field should be used in queries.

These fields are defined using the [`FieldDefs` method of your model](./models.md#creating-your-models).

### `AliasField`

A field can adhere to this interface to indicate that the field should be aliased when generating the SQL for the field.

For example: this is used in annotations to alias the field name.

```go
type AliasField interface {
    attrs.Field
    Alias() string
}
```

### `VirtualField`

A field can adhere to this interface to indicate that the field should be rendered as SQL.

For example: this is used in `fields.ExpressionField` to render the expression as SQL.

```go
type VirtualField interface {
    SQL(inf *expr.ExpressionInfo) (string, []any)
}
```

### `RelatedField`

Indicates a field is related to another model.

Used in `fields.RelationField` to determine the target column name.

If `GetTargetField()` returns `nil`, the target's primary key is used.

```go
type RelatedField interface {
    attrs.Field
    GetTargetField() attrs.Field
}
```

### `ForUseInQueriesField`

Indicates a field should (or shouldn't) be selected in queries.

Mostly used to exclude reverse fields (like reverse FK/O2O).

```go
type ForUseInQueriesField interface {
    attrs.Field
    ForSelectAll() bool
}
```

---

## 📦 Model Interfaces

### `DataModel`

A model can implement this to receive **annotations** from query results.

Values from `Row.Annotations` will also be stored in the model.

```go
type DataModel interface {
	ModelDataStore() ModelDataStore
}
```

#### `ModelDataStore`

The `ModelDataStore` is used to store annotations and any possible relations
on the model which the datastore belongs to.

```go
type ModelDataStore interface {
	HasValue(key string) bool
	GetValue(key string) (any, bool)
	SetValue(key string, value any) error
	DeleteValue(key string) error
}
```

### `ForUseInQueries`

Indicates the model **should not** be auto-saved/deleted by `SaveObject` / `DeleteObject`.

```go
type ForUseInQueries interface {
    attrs.Definer
    ForUseInQueries() bool
}
```

### `QuerySetDefiner`

Lets a model **override the default QuerySet** used when calling `queries.GetQuerySet()`.

This model should avoid calling `queries.GetQuerySet` inside of the method - `queries.Objects` should be called instead.

```go
type QuerySetDefiner interface {
    attrs.Definer
    GetQuerySet() *QuerySet[attrs.Definer]
}
```

### `QuerySetDatabaseDefiner`

Lets a model **control which database** is used for queries.

Should return a key from `django.Global.Settings`.

```go
type QuerySetDatabaseDefiner interface {
    attrs.Definer
    QuerySetDatabase() string
}
```

---

## 🧩 Database Interfaces

### `DB`

Compatible with `*sql.DB` and `*sql.Tx`.

Used internally for executing queries, but can be used to directly execute queries.

```go
type DB interface {
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

### `Transaction`

A transactional `DB`, like `*sql.Tx`.

Used internally for transaction management.

Can be used to directly execute, commit or rollback transactions.

If a transaction was started, the queryset will return `query_errors.ErrTransactionStarted`.

```go
type Transaction interface {
    DB
    Commit() error
    Rollback() error
}
```

---

## 🧠 Query Interfaces

### `QueryInfo`

Describes a compiled query (SQL, args, etc).

```go
type QueryInfo interface {
    SQL() string
    Args() []any
    Model() attrs.Definer
    Compiler() QueryCompiler
}
```

### `CompiledQuery[T]`

A query that can be executed to return a result of type `T`.

```go
type CompiledQuery[T any] interface {
    QueryInfo
    Exec() (T, error)
}
```

#### `CompiledQuery` aliases

```go
type CompiledCountQuery         = CompiledQuery[int64]
type CompiledExistsQuery        = CompiledQuery[bool]
type CompiledValuesListQuery    = CompiledQuery[[][]any]
```

### `QueryCompiler`

Responsible for building SQL queries from a `QuerySet`.

```go
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
        fields FieldInfo,
        primary attrs.Field,
        values []any,
    ) CompiledQuery[[]interface{}]

    // BuildValuesListQuery builds a values list query with the given parameters.
    BuildUpdateQuery(
        ctx context.Context,
        qs *QuerySet[attrs.Definer],
        fields FieldInfo,
        where []expr.LogicalExpression,
        joins []JoinDef,
        groupBy []FieldInfo,
        values []any,
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
```
