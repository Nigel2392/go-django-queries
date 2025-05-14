# Creating your models

Models are defined using the [`attrs`](https://github.com/Nigel2392/go-django/blob/main/docs/attrs/attrs.md) package.

They should be defined as structs, and should implement the `attrs.Definer` interface.

Models are automatically registered to Go-Django when they are inside of an [apps' `Models()` list](https://github.com/Nigel2392/go-django/blob/main/docs/attrs/attrs.md).

A short example of defining some models with a relationship (we will assume the tables already exist):

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
        }),
        attrs.NewField(m, "Name", nil),
        attrs.NewField(m, "Email", nil),
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
        }),
        attrs.NewField(m, "Name", nil),
        attrs.NewField(m, "Profile", &attrs.FieldConfig{
            RelForeignKey: attrs.Relate(&Profile{}, "", nil),
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
        attrs.NewField(m, "Title", nil),
        attrs.NewField(m, "Description", nil),
        attrs.NewField(m, "Done", nil),
        attrs.NewField(m, "User", &attrs.FieldConfig{
            RelForeignKey: attrs.Relate(&User{}, "", nil),
            Column:        "user_id",
        }),
    ).WithTableName("todos")
}
```

---

For more information on defining or registering models, see the [attrs documentation](https://github.com/Nigel2392/go-django/blob/main/docs/attrs/attrs.md).

Or continue with [Querying Objects](./querying.md)…
