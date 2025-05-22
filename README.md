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
  * [QuerySet](./docs/queryset/queryset.md)
  * [Writing Queries](./docs/queryset/writing_queries.md) (WIP)
* [Relations & Joins](./docs/relations/relations.md) (WIP)
* [Expressions & Lookups](./docs/expressions.md) (WIP)
* [Advanced: Virtual Fields](./docs/virtual_fields.md) (WIP)

---

## 🔧 Quick Example

```go
// Query forward foreign key relation
var todos, err := queries.Objects[*Todo](&Todo{}).
    Select("*", "User.*").
    Filter("Done", false).
    OrderBy("-ID").
    All()

// Query reverse foreign key relation
var todos, err := queries.Objects[*User](&User{}).
    Select("*", "TodoSet.*").
    Filter("TodoSet.Done", false).
    OrderBy("-ID").
    All()
```

Continue with [Getting Started](./docs/getting_started.md)…

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
  * Forward one-to-one relations **with a through model**
  * Reverse one-to-one relations **with a through model**
  * Forward foreign key relations (many to one)
  * Reverse foreign key relations (one to many)
  * Forward many-to-many relations **with a through model**
  * Reverse many-to-many relations **with a through model**

**The following features are not supported, tested or probably don't work:**

* Nested many-to-many relations
* Nested one-to-many relations

Example of unsupported nested relations:

Say we have the following relationship defined:

Profile (o2o)-> User (m2m)-> Group (m2m)-> Permissions

We can't query the following:

```go
var objects = queries.Objects[*User](&User{}).
    // invalid, executing the query will
    // panic to prevent querying nested relations
    Select("*", "Group.*", "Group.Permissions.*")

var objects = queries.Objects[*Permission](&Permission{}).
    // invalid, executing the query will
    // panic to prevent querying nested relations
    Select("*", "GroupSet.*", "GroupSet.UserSet.*")

var objects = queries.Objects[*Profile](&Profile{}).
    // valid, supported
    Select("*", "User.*", "User.GroupSet.*")
    
    // invalid, executing the query will
    // panic to prevent querying nested relations
    Select("*", "User.GroupSet.*", "User.GroupSet.Permissions.*")
```
