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
  - [Get a single record](#retrieve-a-single-record)
  - [Get first or last record](#retrieve-first-or-last-record)
  - [Get or create](#retrieve-an-object-or-create-it)
  - [Exists](#exists)
  - [ValuesList query](#valueslist-query)
  - [Filter with `Or`](#filter-with-or)
  - [Fetch related models](#fetch-related-models)
- [Query Interface](#query-interface)
- [Signals](#signals)
  - [Helper functions](#helper-functions)
    - [Inserting new records](#inserting-new-records)
    - [Updating records](#updating-records)
    - [Deleting records](#deleting-records)

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
    // Select the user and profile fields, leaving out star operator
    // This would not result in a join, and only fetch the user's ID
    Select("ID", "Title", "Description", "Done", "User").

    // Select the user and profile fields, append a star to
    // automatically select all related fields, User.Profile would always result in a join
    Select("ID", "Title", "Description", "Done", "User.*", "User.Profile.*").
    
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


    // todos is of type Query[[]attrs.Definer] which is a slice of
    // Definer objects with the underlying type of the model
    todos, err := query.Exec() // / []attrs.Definer, error
    if err != nil {
      t.Fatalf("Failed to query todos: %v", err)
    }

    fmt.Printf("Queried todos: %v\n", todos)
    fmt.Printf("Executed SQL: %s\n", query.SQL())
```

The above query will generate the following SQL:

```sql
SELECT 
  "todos"."ID", 
  "todos"."Title", 
  "todos"."Description", 
  "todos"."Done", 
  "users"."id", 
  "users"."name", 
  "profiles"."id", 
  "profiles"."name", 
  "profiles"."email"
FROM "todos"
LEFT JOIN 
  "users" ON "todos"."user_id" = "users"."id"
LEFT JOIN 
  "profiles" ON "users"."profile_id" = "profiles"."id"
WHERE (
  LOWER("todos"."Title", 'new test') LIKE LOWER(?)
  AND "todos"."Done" = ?
  AND "users"."name" LIKE LOWER(?)
  AND "users"."id" = ?
  AND "profiles"."email" LIKE LOWER(?)
  AND "profiles"."id" = ?
) 
ORDER BY
  "todos"."ID" DESC,
  "users"."name" DESC,
  "profiles"."email" DESC
LIMIT 5
```

The query will return a slice of `attrs.Definer` objects, which can be cast to the appropriate type.

### Create a new record

If the model adheres to `models.Saver`, the model's Save method will be called when creating a new record,  
this can be skipped by calling the queryset's `.ExplicitSave()` method - this way it will always update through the queryset.

```go
todo := &Todo{
    Title: "Finish task",
    Description: "Write documentation",
    Done: false,
}

// This always calls the model's save method when the model adheres to `models.Saver`
createdObj, err := queries.Objects(&Todo{}).Create(todo).Exec()
```

---

### Update a record

If the object has a non- zero primary key value and the model adheres to `models.Saver`, the model's Save method will be called  
thus, skipping the queryset's update method.
If `.ExplicitSave()` is called, on the queryset, the model's save method will never be called.

```go
todo.Title = "Update documentation"
// This does not call the model's Save method, even if it adheres to `models.Saver`
updatedRowCount, err := queries.Objects(&Todo{}).
    Filter("ID", todo.ID).
    Update(todo).Exec()

// This will call the model's Save method, if it is defined
// 
// This is because there is not Filter method called on the queryset, and the model's
// primary key is non- zero.
updatedRowCount, err := queries.Objects(&Todo{}).
    Update(todo).Exec()
```

---

### Delete a record

The delete method will **not** be called on the model, even if it adheres to `models.Deleter`.

If you want to call the model's delete method instead (if it has one), you should use the `DeleteObject` helper function.

```go
deletedRowCount, err := queries.Objects(&Todo{}).
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

### Retrieve a single record

```go
todo, err := queries.Objects(&Todo{}).
    Filter(queries.Q("Title__istartswith", "Finish ta")).
    Get().Exec()
```

---

### Retrieve first or last record

```go
firstTodo, err := queries.Objects(&Todo{}).
    OrderBy("ID").
    First().Exec()

lastTodo, err := queries.Objects(&Todo{}).
    OrderBy("ID").
    Last().Exec()
```

---

### Retrieve an object or create it

`GetOrCreate` is the only queryset function that will execute, and not return a Query[T] object.

It will always return the (retrieved or created) object and an error.

If the object is created and the model adheres to `models.Saver`, the model's Save method will be called,  
this can be skipped by calling the queryset's `.ExplicitSave()` method - this way it will always update through the queryset.

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

---

## Signals

Signals are a way to hook into the lifecycle of a model and perform actions when certain events occur.

These events include:

- `SignalPreModelSave`:    This signal is sent before a model is saved to the database.
- `SignalPostModelSave`:   This signal is sent after a model is saved to the database.
- `SignalPreModelDelete`:  This signal is sent before a model is deleted from the database.
- `SignalPostModelDelete`: This signal is sent after a model is deleted from the database.

There is a caveat however, the signals are not executed by the queryset itself - but by helper functions.

The following shows how you can connect to a signal (although there are multiple ways, this is the easiest).

```go
  // The receiver and error are optional, it is unlikely that you will need them
  // but they are here for completeness.
  recv, err = SignalPreModelSave.Listen(func(s signals.Signal[SignalSave], ss SignalSave) error {
    // Do something before the model is saved
  })
```

---

### Helper functions

Mentioned before in the [signals](#signals) section, the following helper functions are available and will send signals:

- `CreateObject`: This function will create a new object in the database and send the following 2 signals:
  - `SignalPreModelSave`
  - `SignalPostModelSave`

- `UpdateObject`: This function will update an existing object in the database and send the following 2 signals:
  - `SignalPreModelSave`
  - `SignalPostModelSave`

- `DeleteObject`: This function will delete an object from the database and send the following 2 signals:
  - `SignalPreModelDelete`
  - `SignalPostModelDelete`

---

#### Models Methods

Along with the previously mentioned signals, the [following methods can be used on the model itself](https://github.com/Nigel2392/go-django/blob/main/docs/models.md#defining-models) to control saving and deleting of the model:

- `Save(context.Context) error`: This method will save the model to the database.
- `Delete(context.Context) error`: This method will delete the model from the database.

Again, these methods (if defined) will only be called when using the helper functions.

---

#### Inserting new records

```go
  // Create a new profile instance
  var profile = &Profile{
    Name:  "test profile",
    Email: "test@example.com",
  }

  // Create the object with the queries.Objects function, send signals before and after
  if err := queries.CreateObject(profile); err != nil || profile.ID == 0 {
     t.Fatalf("Failed to insert profile: %v", err)
  }

  // Create a new user instance
  var user = &User{
     Name:    "test user",
     Profile: profile,
  }

  // Create the object with the queries.Objects function, send signals before and after
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

  // Create the object with the queries.Objects function, send signals before and after
  if err := queries.CreateObject(todo); err != nil {
    t.Fatalf("Failed to insert todo: %v", err)
  }
```

---

#### Updating records

```go
  // Update the todo instance
  todo.Title = "Updated Test Todo"
  todo.Done = true

  // Create the object with the queries.Objects function using the primary key in the where clause,
  // send signals before and after
  if err := queries.UpdateObject(todo); err != nil {
    t.Fatalf("Failed to update todo: %v", err)
  }
```

---

#### Deleting records

```go
  // Create the object with the queries.Objects function using the primary key in the where clause,
  // send signals before and after
  if err := queries.DeleteObject(todo); err != nil {1
    t.Fatalf("Failed to delete todo: %v", err)
  }
```
