# Go Django Queries Documentation

Welcome to the documentation for the `go-django-queries` package.

While [Go-Django](https://github.com/Nigel2392/go-django) tries to do as little as possible with the database, sometimes helper functions make working with models easier.

This library brings Django-style ORM queries to Go-Django, allowing you to:

* Define models with relationships
* Compose queries with filters, ordering, limits
* Use select expressions and annotations

---

## 📁 Documentation Structure

* [Getting Started](./docs/getting_started.md)
* [Defining Models](./docs/models/models.md)
* [Interfaces](./docs/interfaces.md)
* [Querying Objects](./docs/querying.md)
  * [QuerySet](./docs/queryset/queryset.md)
  * [Writing Queries](./docs/queryset/writing_queries.md) (WIP)
* [Relations & Joins](./docs/relations/relations.md) (WIP)
* [Expressions](./docs/expressions/expressions.md)
  * [Lookups](./docs/expressions/lookups.md)
  * [Case Expressions](./docs/expressions/cases.md)
* [Advanced: Virtual Fields](./docs/virtual_fields.md) (WIP)

---

## 🔧 Quick Example

```go
// Query forward foreign key relation
var todos, err := queries.GetQuerySet(&Todo{}).
    Select("*", "User.*").
    Filter("Done", false).
    OrderBy("-ID").
    All()

// Query reverse foreign key relation
var todos, err := queries.GetQuerySet(&User{}).
    Select("*", "TodoSet.*").
    Filter("TodoSet.Done", false).
    OrderBy("-ID").
    All()
```

Continue with [Getting Started](./docs/getting_started.md)…

## ✅ Supported Features

We try to support as many features as possible, but some stuff is either not supported, implemented or tested yet.

### Tested Databases

But more tests / databases will be added over time.

* SQLite
* MySQL
* MariaDB (with returning support, custom driver - use "mariadb" in sql.Open(...))
* [dolthub/go-mysql-server](https://github.com/dolthub/go-mysql-server)

#### Caveats

* MySQL does not support the `RETURNING` clause - to get around this and still be able to retrieve the `LastInsertID`,
  we have to do each INSERT in a separate query.

* MySQL and MariaDB do not support preparing multiple statements in a single query, we have to prepare each statement separately
  this results in BulkUpdate not being properly supported, each update will be executed separately (in a transaction if one was not present).

### The following features are currently supported

* Selecting fields
* Selecting forward and reverse relations
* Filtering
* Lookups
* Ordering
* Limiting
* Expressions
* Annotations
* Aggregates
* Virtual fields
