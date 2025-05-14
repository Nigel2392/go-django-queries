# 📝 Querying Objects

The QuerySet type is used to build and execute queries against the database.

## Documentation Structure

* [Objects](#objects)
* [ChangeObjectsType](#changeobjectstype)
* [QuerySet Methods](#queryset-methods)
  * [DB](#db)
  * [Model](#model)
  * [Compiler](#compiler)
  * [LatestQuery](#latestquery)
  * [StartTransaction](#starttransaction)
  * [Clone](#clone)
  * [String](#string)
  * [GoString](#gostring)
  * [Select](#select)
  * [Filter](#filter)
  * [Having](#having)
  * [GroupBy](#groupby)
  * [OrderBy](#orderby)
  * [Reverse](#reverse)
  * [Limit](#limit)
  * [Offset](#offset)
  * [ForUpdate](#forupdate)
  * [Distinct](#distinct)
  * [ExplicitSave](#explicitsave)
  * [Annotate](#annotate)
* [Executing Queries](#executing-queries)
  * [All](#all)
  * [ValuesList](#valueslist)
  * [Aggregate](#aggregate)
  * [Get](#get)
  * [GetOrCreate](#getorcreate)
  * [First](#first)
  * [Last](#last)
  * [Exists](#exists)
  * [Count](#count)
  * [Create](#create)
  * [Update](#update)
  * [Delete](#delete)

## Objects

Queries are built using the `queries.Objects` function, which takes a model type as an argument,  
and returns a QuerySet.

Example:

```go
var querySet = queries.Objects[*User](&User{})
```

## ChangeObjectsType

The above example will return a `QuerySet[*User]`, this however proves some issues:

The `QuerySet[*User]` object cannot be generically used in other functions.

I.E. the following code will work:

```go
func ExecQuery(qs *QuerySet[*User]) error {
    return qs.All()
}

var querySet = queries.Objects[*User](&User{})
ExecQuery(querySet)
```

However, the following code will not work:

```go

var querySet = queries.Objects[attrs.Definer](&User{})
ExecQuery(querySet) // error: cannot use qs (type *QuerySet[*User]) as type *QuerySet[attrs.Definer] in argument to ExecQuery
```

This is because the `QuerySet[*User]` object is not a `QuerySet[attrs.Definer]`, it is a `QuerySet[*User]`, which is a different type.

To fix this issue, we should conver the queryset's type to a `QuerySet[attrs.Definer]`.

This can be done with the `ChangeObjectsType` function:

```go
var querySet = queries.Objects[attrs.Definer](&User{})
var newQuerySet = queries.ChangeObjectsType[attrs.Definer, *User](querySet)
ExecQuery(newQuerySet)
```

## QuerySet Methods

The QuerySet type provides a set of methods to use in queries.

### DB

**Signature:** `DB() queries.DB`

Returns the database interface used by the compiler.

This interface is compatible with `*sql.DB` and `*sql.Tx`.

If a transaction was started, it will return the transaction instead of the database connection.

### Model

**Signature:** `Model() attrs.Definer`

The model which the queryset is for.

### Compiler

**Signature:** `Compiler() queries.QueryCompiler`

The underlying SQL compiler used by the queryset.

### LatestQuery

**Signature:** `LatestQuery() queries.QueryInfo`

Returns the latest query that was executed on the queryset.

See [`QueryInfo`](../interfaces.md#queryinfo) for more details.

### StartTransaction

**Signature:** `StartTransaction(ctx context.Context) (Transaction, error)`

StartTransaction starts a new transaction on the underlying database.

It returns a Transaction object which will put the compiler in transaction mode.

If a transaction was already started, it will return `query_errors.ErrTransactionStarted`

See [`Transaction`](../interfaces.md#transaction) for more details.

### Clone

**Signature:** `Clone() *QuerySet[T]`

Perform a clone of the current queryset.

It returns a new QuerySet with the same parameters as the original one.

The new QuerySet will not be able to modify values in the original QuerySet.

### String

**Signature:** `String() string`

Return a cut-down string representation of the queryset.

### GoString

**Signature:** `GoString() string`

Return a detailed string representation of the queryset.

### Select

**Signature:** `Select(fields ...any) *QuerySet[T]`

Select is used to select specific fields from the model.

It takes a list of field names as arguments and returns a new QuerySet with the selected fields.

If no fields are provided, it selects all fields from the model.

If the first field is "*", it selects all fields from the model,
extra fields (i.e. relations) can be provided thereafter - these will also be added to the selection.

How to call Select:

* `Select("*")`
* `Select("Field1", "Field2")`
* `Select("Field1", "Field2", "Relation.*")`
* `Select("*", "Relation.*")`
* `Select("Relation.*")`
* `Select("*", "Relation.Field1", "Relation.Field2", "Relation.Nested.*")`

### Filter

**Signature:** `Filter(key interface{}, vals ...interface{}) *QuerySet[T]`

Filter is used to filter the results of a query.

It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.

The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.

By default the `__exact` (=) operator is used, each where clause is separated by `AND`.

See [`expressions](./expressions.md) for more details on expressions and lookups.

Example:

```go
var qs = queries.Objects[*User](&User{}).
    Filter("Name", "John Doe").           // exact match
    Filter("Email__istartswith", "JOHN"). // case insensitive starts with
    Filter("Age__lt", 30).                // less than
    Filter(
        // Lower(Name) LIKE LOWER("JOHN") AND LOWER(Email) LIKE doe%,
        expr.Q("Name__icontains", "JOHN"),
        expr.Q("Email__icontains", "doe"),
    ).
    Filter(
        // Lower(Name) LIKE LOWER("JOHN") OR LOWER(Email) LIKE doe%,
        expr.Q("Name__icontains", "JOHN").
            Or("Email__icontains", "doe"),
    )
```

### Having

**Signature:** `Having(key interface, vals ...interface{}) *QuerySet[T]`

Having is used to filter the results of a query after grouping.

It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.

The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.

See ['Filter'](#filter) for more details on filtering results.

See [`expressions](./expressions.md) for more details on expressions and lookups.

### GroupBy

**Signature:** `GroupBy(fields ...string) *QuerySet[T]`

GroupBy is used to group the results of a query.

It takes a list of field names as arguments and returns a new QuerySet with the grouped results.

### OrderBy

**Signature:** `OrderBy(fields ...string) *QuerySet[T]`

OrderBy is used to order the results of a query.

It takes a list of field names as arguments and returns a new QuerySet with the ordered results.

The field names can be prefixed with a minus sign (-) to indicate descending order.

Example: `OrderBy("-Field1", "Field2")`

### Reverse

**Signature:** `Reverse() *QuerySet[T]`

Reverse is used after ordering to reverse the order of the OrderBy clause.

It returns a new QuerySet with the reversed order.

### Limit

**Signature:** `Limit(n int) *QuerySet[T]`

Limit is used to limit the number of results returned by a query.

It takes an integer as an argument and returns a new QuerySet with the limited results.

### Offset

**Signature:** `Offset(n int) *QuerySet[T]`

Offset is used to set the offset of the results returned by a query.

It takes an integer as an argument and returns a new QuerySet with the offset results.

### ForUpdate

**Signature:** `ForUpdate() *QuerySet[T]`

ForUpdate is used to lock the rows returned by a query for update.

It is used to prevent other transactions from modifying the rows until the current transaction is committed or rolled back.

It returns a new QuerySet with the locked rows.

### Distinct

**Signature:** `Distinct() *QuerySet[T]`

Distinct is used to select distinct rows from the results of a query.

It is used to remove duplicate rows from the results.

It returns a new QuerySet with the distinct rows.

### ExplicitSave

**Signature:** `ExplicitSave() *QuerySet[T]`

ExplicitSave is used to indicate that the save operation should be explicit.

It is used to prevent the automatic save operation from being performed on the model.

I.E. when using the `Create` method after calling `qs.ExplicitSave()`, it will **not** automatically
save the model to the database using the model's own `Save` method.

It returns a new QuerySet with the explicit save operation set.

### Annotate

**Signature:** `Annotate(aliasOrAliasMap interface{}, exprs ...expr.Expression) *QuerySet[T]`

Annotate is used to add annotations to the results of a query.

It takes a string or a map of strings to expr.Expressions as arguments and returns a new QuerySet with the annotations.

If a string is provided, it is used as the alias for the expr.Expression.

If a map is provided, the keys are used as aliases for the expr.Expressions.

See [`expressions`](./expressions.md) for more details on expressions and lookups.

## Executing Queries

Queries can be executed using the below methods.

These methods return a result and an error.

After executing any of these methods, the `LatestQuery()`  
method can be used to retrieve the latest query that was executed.

### All

**Signature:** `All() ([]*Row[T], error)`

All is used to retrieve all rows from the database.

Fields can be provided to select as strings or expressions.

The fields provided have to exist in the model or be annotated.

It returns a slice of Row objects, which contain the model object and a map of annotations.

If no fields are provided, it selects all fields from the model with `Select(*)`, see `Select()` for more details.

```go
type Row[T attrs.Definer] struct {
    Object      T
    Annotations map[string]any
}
```

### ValuesList

**Signature:** `ValuesList(fields ...any) ([][]any, error)`

ValuesList is used to retrieve a list of values from the database.

It takes a list of field names or expressions as arguments and returns a slice of slices of values.

See [`expressions`](./expressions.md) for more details on expressions and lookups.

### Aggregate

**Signature:** `Aggregate(annotations map[string]expr.Expression) (map[string]any, error)`

Aggregate is used to perform aggregation on the results of a query.

It takes a map of field names to expr.Expressions as arguments and returns a Query that can be executed to get the results.

Example:

```go
var agg, err = queries.Objects[*User](&User{}).
    Aggregate(map[string]expr.Expression{
        "Count": expr.FuncCount("ID"),
    })

var count = agg["Count"]
```

See [`expressions`](./expressions.md) for more details on expressions and lookups.

### Get

**Signature:** `Get() (*Row[T], error)`

Get is used to retrieve a single row from the database.

It returns a Row object and an error.
The row object contains the model object and a map of annotations.

If no rows are found, it returns queries.query_errors.ErrNoRows.

If multiple rows are found, it returns queries.query_errors.ErrMultipleRows.

### First

**Signature:** `First() (*Row[T], error)`

First is used to retrieve the first row from the database.

It returns a Row object and an error.

The row object contains the model object and a map of annotations.

### Last

**Signature:** `Last() (*Row[T], error)`

Last is used to retrieve the last row from the database.

It returns a Row object and an error.

The row object contains the model object and a map of annotations.

It reverses the order of the results and then calls First to get the last row.

### Exists

**Signature:** `Exists() (bool, error)`

Exists is used to check if any rows exist in the database.

It returns a boolean indicating if any rows exist.

### Count

**Signature:** `Count() (int64, error)`

Count is used to count the number of rows in the database.

It returns an int64 indicating the number of rows.

### Create

**Signature:** `Create(value T) (T, error)`

Create is used to create a new object in the database.

It takes a definer object as an argument and returns a Row object and an error.
It panics if a non- nullable field is null or if the field is not found in the model.

The model can adhere to django's `models.Saver` interface, in which case the `Save()` method will be called
unless `ExplicitSave()` was called on the queryset.

If `ExplicitSave()` was called, the `Create()` method will return a query that can be executed to create the object
without calling the `Save()` method on the model.

### GetOrCreate

See also: [`Get`](#get), [`Create`](#create)

**Signature:** `GetOrCreate(value T) (T, error)`

GetOrCreate is used to retrieve a single row from the database or create it if it does not exist.

It returns the definer object and an error if any occurred.

This method executes a transaction to ensure that the object is created only once.

It panics if the queryset has no where clause.

### Update

**Signature:** `Update(value T, expressions ...expr.NamedExpression) (int64, error)`

Update is used to update an object in the database.

It takes a definer object as an argument and returns a CountQuery that can be executed
to get the result, which is the number of rows affected.

It panics if a non- nullable field is null or if the field is not found in the model.

If the model adheres to django's `models.Saver` interface, no where clause is provided
and ExplicitSave() was not called, the `Save()` method will be called on the model

### Delete

**Signature:** `Delete() (int64, error)`

Delete is used to delete an object from the database.

It returns the number of rows affected and an error.

---

See [Writing Queries](./writing_queries.md) for more advanced queries and information…
