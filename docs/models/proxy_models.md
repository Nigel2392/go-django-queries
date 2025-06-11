# Proxy models documentation

Proxy models are a powerful feature in Go-Django Queries that allow you to embed other models into your own model.
These models provide a way to create a model that is a "view" or "wrapper" around another model, allowing you to extend or modify its behavior without changing the original model.
There is a caveat to this however, please refer to the [Caveats](#caveats) section below.

---

## Rules

There are however a few rules for proxy models:

* A model can have no more than one proxy model.
* A proxy model must embed the `models.Model` struct (much like a regular model).
* A proxy model must implement the `attrs.Definer` interface.
* A proxy model must implement the `CanTargetDefiner` interface, which
  allows the proxy model to effectively generate a join to the target model.
* The top-level (embedder) model must embed the `models.Model` struct.
* The top-level model must implement the `attrs.Definer` interface.
* The top-level model must embed a pointer to the proxy model, not a value.

---

## `CanTargetDefiner` Interface

This interface can be implemented by a proxy model to define which fields
are used to create a proper join between the target model and the proxy model.

The content type and primary field are used to then generate that join.

From the database perspective, the content type field holds the content type of the target model,
and the primary field holds the primary key of the target model.

```go
type CanTargetDefiner interface {
    TargetContentTypeField() attrs.FieldDefinition
    TargetPrimaryField() attrs.FieldDefinition
}
```

## Example of defining a proxied model

```go
type Page struct {
    // Embedding Model struct is required for proxy models
    models.Model
    
    // This is the primary key for the target model
    PageID       int

    // Content type of the target model
    ContentType  *contenttypes.BaseContentType[attrs.Definer]

    // Content fields of the Page model
    ID           int
    Title        string
    Content      string
}


func (p *Page) TargetContentTypeField() attrs.FieldDefinition {
    var defs = p.FieldDefs()
    var f, _ = defs.Field("PageContentType")
    return f
}

func (p *Page) TargetPrimaryField() attrs.FieldDefinition {
    var defs = p.FieldDefs()
    var f, _ = defs.Field("PageID")
    return f
}

func (m *Page) FieldDefs() attrs.Definitions {
    return m.Model.Define(m,
        attrs.NewField(m, "ID", &attrs.FieldConfig{
            Primary: true,
        }),
        attrs.NewField(m, "Title"),
        attrs.NewField(m, "Content"),
        attrs.NewField(m, "PageID"),
        attrs.NewField(m, "ContentType"),
    )
}

type BlogPage struct {
    // Embedding the Model struct is required for models with a proxy model
    models.Model

    // Embedding the Page model
    *Page
    
    // Extra fields for the BlogPage model
    Author string
}

func (m *BlogPage) FieldDefs() attrs.Definitions {
    return m.Model.Define(m,
        // Embed the fields of the Page model
        fields.Embed("Page"),
        attrs.NewField(m, "Author"),
    )
}
```

## Caveats

When using proxy models, there is a caveat to be aware of: the proxy model must be used in conjunction with the top-level model.
This means that any proxy models from fields in the top-level model will not be available automatically.

Example:

```go
type BaseUser struct {
    models.Model
    ID   int
    Name string
}
func (m *BaseUser) FieldDefs() attrs.Definitions {
    // ...
}

type BaseProfile struct {
    models.Model
    ID    int
    Email string
}
func (m *BaseProfile) FieldDefs() attrs.Definitions {
    // ...
}

type Profile struct {
    models.Model
    *BaseProfile // Embedding the BaseProfile model
}
func (m *Profile) FieldDefs() attrs.Definitions {
    // ...
}

type User struct {
    models.Model
    *BaseUser // Embedding the BaseUser model
    Profile *Profile
}
func (m *User) FieldDefs() attrs.Definitions {
    // ...
}
```

For now it is impossible to automatically join the BaseProfile model when querying the User model.  
This might be subject to change in the future.
