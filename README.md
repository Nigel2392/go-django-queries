# go-django-queries

while [Go-Django](https://github.com/Nigel2392/go-django) tries to do as little as possible with the database, some things might be made easier if we
provide some helper functions to work with the database.

**Table of Contents**

- [Installation](#installation)
- [Usage](#usage)
  - [Basic setup for different databases](#basic-setup-for-different-databases)
  - [Defining your models](#defining-your-models)
  - [Inserting new records](#inserting-new-records)
  - [Updating records](#updating-records)
  - [Deleting records](#deleting-records)
  - [Querying records](#querying-records)

Latest version: `v1.0.7`

## Installation

The package is easily installed with `go get`.

```bash
go get github.com/Nigel2392/go-django-queries@latest
```

## Usage

We provide a short example of how to use the package.

It is assumed that the database tables are already set-up and that the [go-django app is already instantiated](https://github.com/Nigel2392/go-django/blob/main/docs/configuring.md).

Most of this package is confirmed to be compatible with MySQL, SQLite3 and PostgreSQL - using [`SQLX.Rebind(...)`](https://github.com/jmoiron/sqlx) to translate the queries in between databases.

### Basic setup for different databases

Semantics for PostgreSQL, MySQL or SQLite3 might differ slightly, such as which quotes to use for the table names and column names.

This can be overridden (on a package level) by setting `queries.Quote` to the desired quote character, by default it is set to "`".

```go
package main

import (
    "github.com/Nigel2392/go-django-queries/src"
)

func init() {
    // Set the quote character to be used for table and column names
    queries.Quote = `"`
}
```

### Defining your models

The models are defined using the [`attrs`](https://github.com/Nigel2392/go-django/blob/main/docs/attrs.md) package, which is part of the `go-django` package.

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
            Column:   "id", // can be inferred, but explicitly set for clarity
            Primary:  true,
            ReadOnly: true,
        }),
        attrs.NewField(m, "Title", &attrs.FieldConfig{
            Column: "title", // can be inferred, but explicitly set for clarity
        }),
        attrs.NewField(m, "Description", &attrs.FieldConfig{
            Column: "description", // can be inferred, but explicitly set for clarity
            FormWidget: func(cfg attrs.FieldConfig) widgets.Widget {
             return widgets.NewTextarea(nil)
            },
        }),
        attrs.NewField(m, "Done", &attrs.FieldConfig{}),
        attrs.NewField(m, "User", &attrs.FieldConfig{
            Column:        "user_id",
            RelForeignKey: &User{},
        }),
    ).WithTableName("todos")
}
```

### Inserting new records

```go
  // Create a new profile instance
  var profile = &Profile{
    Name:  "test profile",
    Email: "test@example.com",
  }

  if err := queries.CreateObject(profile); err != nil || profile.ID == 0 {
     t.Fatalf("Failed to insert profile: %v", err)
  }

  // Create a new user instance
  var user = &User{
     Name:    "test user",
     Profile: profile,
  }

  if err := queries.CreateObject(user); err != nil || user.ID == 0 {
     t.Fatalf("Failed to insert user: %v", err)
  }

  // Create a new todo instance
  var todo = &Todo{
    Title:       "New Test Todo",
    Description: "This is a new test todo",
    Done:        false,
    User:        user,
  }

  if err := queries.CreateObject(todo); err != nil {
    t.Fatalf("Failed to insert todo: %v", err)
  }
```

### Updating records

```go
  // Update the todo instance
  todo.Title = "Updated Test Todo"
  todo.Done = true

  if err := queries.UpdateObject(todo); err != nil {
    t.Fatalf("Failed to update todo: %v", err)
  }
```

### Deleting records

```go
  if err := queries.DeleteObject(todo); err != nil {
    t.Fatalf("Failed to delete todo: %v", err)
  }
```

### Querying records

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

#### The `Query` interface

The `Query` represents a query that can be executed against the database.

It is used to give the developers a way to introspect the query and its arguments.

No database lookup will be performed until the `Exec()` method is called.

The `Exec()` method will return the result of the query and an error if one occurred.

```go
// - T1 is the type of the result
// - T2 is the type of the model
type Query[T1 any, T2 any] interface {
  // The SQL query string to be executed
  SQL() string

  // Arguments to be passed to the query
  Args() []any

  // The model which the query is for
  Model() T2

  // Execute the query and return the result 
  Exec() (T1, error)
}
```

### Database Drivers & Query translations

This only works automatically if your driver is one of the following:

- `github.com/go-sql-driver/mysql.MySQLDriver`
- `github.com/mattn/go-sqlite3.SQLiteDriver`
- `github.com/jackc/pgx/v5/stdlib.Driver`

Otherwise the database driver needs to be explicitly registered with `RegisterDriver`.

```go
package main

import (
    "github.com/Nigel2392/go-django-queries/src"
    myDriver "path/to/your/database/driver"
)

func init() {
    // Register the database driver with the package
    queries.RegisterDriver(&myDriver.SQLiteDriver{}, "sqlite3")
}
```
