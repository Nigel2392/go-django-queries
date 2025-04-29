# go-django-queries

While [Go-Django](https://github.com/Nigel2392/go-django) tries to do as little as possible with the database, sometimes helper functions make working with models easier.

This package provides simple ways to build queries, inserts, updates, deletes, and fetch related models automatically.

---

**Table of Contents**

- [Installation](#installation)
- [Defining your models](#defining-your-models)
- [Usage Examples](#usage-examples)
  - [Create a new record](#create-a-new-record)
  - [Update a record](#update-a-record)
  - [Delete a record](#delete-a-record)
  - [Count records](#count-records)
  - [Get a single record](#get-a-single-record)
  - [Get first or last record](#get-first-or-last-record)
  - [Get or create](#get-or-create)
  - [Exists](#exists)
  - [ValuesList query](#valueslist-query)
  - [Filter with `Or`](#filter-with-or)
  - [Fetch related models](#fetch-related-models)
- [Query Interface](#query-interface)

Latest version: `v1.0.7`

---

## Installation

```bash
go get github.com/Nigel2392/go-django-queries@latest
```

---

## Defining your models

The models are defined using the [`attrs`](https://github.com/Nigel2392/go-django/blob/main/docs/attrs.md) package.

```go
type Profile struct {
    ID    int
    Name  string
    Email string
}

func (m *Profile) FieldDefs() attrs.Definitions {
    return attrs.Define(m,
        attrs.NewField(m, "ID", &attrs.FieldConfig{
            Primary:  true,
            ReadOnly: true,
        }),
        attrs.NewField(m, "Name", &attrs.FieldConfig{}),
        attrs.NewField(m, "Email", &attrs.FieldConfig{}),
    ).WithTableName("profiles")
}

type User struct {
    ID      int
    Name    string
    Profile *Profile
}

func (m *User) FieldDefs() attrs.Definitions {
    return attrs.Define(m,
        attrs.NewField(m, "ID", &attrs.FieldConfig{
            Primary:  true,
            ReadOnly: true,
        }),
        attrs.NewField(m, "Name", &attrs.FieldConfig{}),
        attrs.NewField(m, "Profile", &attrs.FieldConfig{
            RelForeignKey: &Profile{},
            Column:        "profile_id",
        }),
    ).WithTableName("users")
}

type Todo struct {
    ID          int
    Title       string
    Description string
    Done        bool
    User        *User
}

func (m *Todo) FieldDefs() attrs.Definitions {
    return attrs.Define(m,
        attrs.NewField(m, "ID", &attrs.FieldConfig{
            Primary:  true,
            ReadOnly: true,
        }),
        attrs.NewField(m, "Title", &attrs.FieldConfig{}),
        attrs.NewField(m, "Description", &attrs.FieldConfig{}),
        attrs.NewField(m, "Done", &attrs.FieldConfig{}),
        attrs.NewField(m, "User", &attrs.FieldConfig{
            Column:        "user_id",
            RelForeignKey: &User{},
        }),
    ).WithTableName("todos")
}
```

---

## Usage Examples

### Querying records

Queries are built using the `queries.Objects` function, which takes a model type as an argument.

We will explain the following example query:

```go
  query := queries.Objects(&Todo{}).
    // Select the user and profile fields, append a star to
    // automatically select all related fields, User.Profile would always result in a join
    Select("ID", "Title", "Description", "Done", "User.*", "User.Profile.*").

    // Select the user and profile fields, leaving out star operator
    // This would not result in a join, and only fetch the user's ID
    Select("ID", "Title", "Description", "Done", "User").
    
    // Generate a WHERE clause with the given conditions
    Filter(
      queries.Q("Title__icontains", "new test"),
      queries.Q("Done", true),
      queries.Q("User.Name__icontains", "test"),
      queries.Q("User.ID", user.ID),
      queries.Q("User.Profile.Email__icontains", profile.Email),
      queries.Q("User.Profile.ID", profile.ID),
    ).
    // Generate an ORDER BY clause with the given conditions
    OrderBy("-ID", "-User.Name", "-User.Profile.Email").
    Limit(5).
    All()


    // todos is of type Query[[]*Todo, *Todo]
    todos, err := query.Exec() // / []*Todo, error
    if err != nil {
      t.Fatalf("Failed to query todos: %v", err)
    }

    fmt.Printf("Queried todos: %v\n", todos)
    fmt.Printf("Executed SQL: %s\n", query.SQL())
```

The above query will generate the following SQL:

```sql
SELECT "todos"."ID", "todos"."Title", "todos"."Description", "todos"."Done", "users"."id", "users"."name", "profiles"."id", "profiles"."name", "profiles"."email"
FROM "todos"
LEFT JOIN "users" ON "todos"."user_id" = "users"."id"
LEFT JOIN "profiles" ON "users"."profile_id" = "profiles"."id"
WHERE LOWER("todos"."Title", 'new test') LIKE LOWER('%new test%')
AND "todos"."Done" = true
AND "users"."name" LIKE LOWER('%test%')
AND "users"."id" = 1
AND "profiles"."email" LIKE LOWER('%example.com%')
AND "profiles"."id" = 1
ORDER BY "todos"."ID" DESC, "users"."name" DESC, "profiles"."email" DESC
LIMIT 5
```

The query will return a slice of `attrs.Definer` objects, which can be cast to the appropriate type.

### Create a new record

```go
todo := &Todo{
    Title: "Finish task",
    Description: "Write documentation",
    Done: false,
}

created, err := queries.Objects(&Todo{}).Create(todo).Exec()
```

---

### Update a record

```go
todo.Title = "Update documentation"
updatedRows, err := queries.Objects(&Todo{}).
    Filter("ID", todo.ID).
    Update(todo).Exec()
```

---

### Delete a record

```go
deletedRows, err := queries.Objects(&Todo{}).
    Filter("ID", todo.ID).
    Delete().Exec()
```

---

### Count records

```go
count, err := queries.Objects(&Todo{}).
    Filter("Done", false).
    Count().Exec()
```

---

### Get a single record

```go
todo, err := queries.Objects(&Todo{}).
    Filter(queries.Q("Title__istartswith", "Finish ta")).
    Get().Exec()
```

---

### Get first or last record

```go
firstTodo, err := queries.Objects(&Todo{}).
    OrderBy("ID").
    First().Exec()

lastTodo, err := queries.Objects(&Todo{}).
    OrderBy("ID").
    Last().Exec()
```

---

### Get or create

`GetOrCreate` is the only queryset function that will execute, and not return a Query[T] object.

It will always return the (retrieved or created) object and an error.

```go
todo := &Todo{ Title: "Unique task" }

dbTodo, err := queries.Objects(&Todo{}).
    Filter(queries.Q("Title", todo.Title)).
    GetOrCreate(todo)
```

---

### Exists

```go
exists, err := queries.Objects(&Todo{}).
    Filter(queries.Q("Title__icontains", "task")).
    Exists().Exec()
```

---

### ValuesList query

```go
values, err := queries.Objects(&Todo{}).
    Select("ID", "Title").
    Filter(queries.Q("Done", false)).
    ValuesList().Exec()

for _, row := range values {
    fmt.Println(row) // []interface{} { ID, Title }
}
```

---

### Filter with `Or`

```go
todos, err := queries.Objects(&Todo{}).
    Filter(
        queries.Or(
            queries.Q("Title__icontains", "urgent"),
            queries.Q("Title__icontains", "important"),
        ),
    ).
    OrderBy("-ID").
    All().Exec()
```

---

### Fetch related models

```go
todos, err := queries.Objects(&Todo{}).
    Select("ID", "Title", "Done", "User.*", "User.Profile.*").
    Filter(
        queries.Q("Done", false),
        queries.Q("User.Profile.Email__icontains", "example.com"),
    ).
    OrderBy("-ID").
    All().Exec()
```

---

## Query Interface

The `Query` represents a query that can be executed against the database.

```go
type Query[T any] interface {
    // SQL returns the SQL string
    SQL() string

    // Args returns the query arguments
    Args() []any

    // Model returns the model instance
    Model() attrs.Definer

    // Exec executes the query
    Exec() (T, error)
}
```
