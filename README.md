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
* [Defining Models](./docs/models.md)
* [Interfaces](./docs/interfaces.md)
* [Querying Objects](./docs/querying.md)
  * [QuerySet](./docs/querying/queryset.md)
  * [Writing Queries](./docs/querying/writing_queries.md) (WIP)
* [Relations & Joins](./docs/relations/relations.md) (WIP)
* [Expressions & Lookups](./docs/expressions.md) (WIP)
* [Advanced: Virtual Fields](./docs/virtual_fields.md) (WIP)

---

## 🔧 Quick Example

```go
var todos, err := queries.Objects[*Todo](&Todo{}).
    Select("*", "User.*").
    Filter("Done", false).
    OrderBy("-ID").
    All()
```

Continue with [Getting Started](./getting_started.md)…

## ✅ Supported / Unsupported Features

We try to support as many features as possible, but some stuff is either not supported, implemented or tested yet.

**The following features are currently supported:**

* Selecting fields
* Filtering
* Ordering
* Limiting
* Expressions
* Annotations
* Aggregates
* Virtual fields
* Forward and reverse relations
  * Forward one-to-one relations
  * Reverse one-to-one relations
  * Forward one-to-one relations with a through model
  * Reverse one-to-one relations with a through model
  * Forward foreign key relations

**The following features are not supported, tested or probably don't work:**

* Many-to-many relations
* Reverse foreign key relations (one to many)
