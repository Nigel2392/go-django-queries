package testsql

import (
	"time"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

var ExtendedDefinitions = false

type User struct {
	ID        int64  `attrs:"primary"`
	Name      string `attrs:"max_length=255"`
	Email     string `attrs:"max_length=255"`
	Age       int32  `attrs:"min_value=0;max_value=120"`
	FirstName string `attrs:"-"`
	LastName  string `attrs:"-"`
}

func (m *User) FieldDefs() attrs.Definitions {
	var fieldDefs = attrs.AutoDefinitions(m)
	if ExtendedDefinitions {
		var fields = fieldDefs.Fields()
		fields = append(fields, attrs.NewField(m, "FirstName", &attrs.FieldConfig{}))
		fields = append(fields, attrs.NewField(m, "LastName", &attrs.FieldConfig{}))
		fieldDefs = attrs.Define(m, fields...)
	}
	return fieldDefs
}

type Todo struct {
	ID        int64     `attrs:"primary"`
	Title     string    `attrs:"max_length=255"`
	Completed bool      `attrs:"default=false"`
	User      *User     `attrs:"fk=test_sql.User;column=user_id"`
	CreatedAt time.Time `attrs:"-"`
	UpdatedAt time.Time `attrs:"-"`
}

func (m *Todo) FieldDefs() attrs.Definitions {
	var fieldDefs = attrs.AutoDefinitions(m)
	if ExtendedDefinitions {
		var fields = fieldDefs.Fields()
		fields = append(fields, attrs.NewField(m, "CreatedAt", &attrs.FieldConfig{}))
		fields = append(fields, attrs.NewField(m, "UpdatedAt", &attrs.FieldConfig{}))
		fieldDefs = attrs.Define(m, fields...)
	}
	return fieldDefs

}

type Profile struct {
	ID        int64  `attrs:"primary"`
	Image     string `attrs:"max_length=255"`
	User      *User  `attrs:"o2o=test_sql.User;column=user_id"`
	Biography string `attrs:"-"`
	Website   string `attrs:"-"`
}

func (m *Profile) FieldDefs() attrs.Definitions {
	var fieldDefs = attrs.AutoDefinitions(m)
	if ExtendedDefinitions {
		var fields = fieldDefs.Fields()
		fields = append(fields, attrs.NewField(m, "Biography", &attrs.FieldConfig{}))
		fields = append(fields, attrs.NewField(m, "Website", &attrs.FieldConfig{}))
		fieldDefs = attrs.Define(m, fields...)
	}
	return fieldDefs
}
