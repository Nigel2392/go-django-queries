package queries_test

import (
	"reflect"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type relationTestExpected struct {
	type_ queries.RelationType
	final reflect.Type
}

type relationTest struct {
	name       string
	model      attrs.Definer
	fieldDefs  attrs.Definitions
	expectsFwd map[string]relationTestExpected
	expectsRev map[string]relationTestExpected
}

func getType(obj any) reflect.Type {
	return reflect.TypeOf(obj)
}

var tests = []relationTest{
	{
		name:  "ExpectedForwardRelation",
		model: &Category{},
		expectsFwd: map[string]relationTestExpected{
			"Parent": {
				type_: queries.RelationTypeOneToMany,
				final: getType(&Category{}),
			},
		},
		expectsRev: map[string]relationTestExpected{
			"CategorySet": {
				type_: queries.RelationTypeManyToOne,
				final: getType(&Category{}),
			},
		},
	},
}

func TestRegisterModelRelations(t *testing.T) {

	for _, test := range tests {
		test.fieldDefs = test.model.FieldDefs()
		t.Run(test.name, func(t *testing.T) {
			queries.RegisterModel(test.model)
			meta := queries.GetModelMeta(test.model)

			for field, exp := range test.expectsFwd {
				rel, ok := meta.Forward.Get(field)
				if !ok {
					t.Errorf("expected forward relation for field %q", field)
					continue
				}

				_, ok = test.fieldDefs.Field(field)
				if !ok {
					t.Errorf("expected field %q in model %T", field, test.model)
					continue
				}

				if rel.Type() != exp.type_ {
					t.Errorf("expected forward relation type %v for %q, got %v", exp.type_, field, rel.Type())
				}

				last := rel.Chain()
				for last.To() != nil {
					last = last.To()
				}

				if reflect.TypeOf(last.Model()) != exp.final {
					t.Errorf("expected final model type %v for %q, got %v", exp.final, field, reflect.TypeOf(last.Model()))
				}
			}

			for field, exp := range test.expectsRev {
				rel, ok := meta.Reverse.Get(field)
				if !ok {
					t.Errorf("expected reverse relation for field %q", field)
					continue
				}

				if rel.Type() != exp.type_ {
					t.Errorf("expected reverse relation type %v for %q, got %v", exp.type_, field, rel.Type())
				}

				_, ok = test.fieldDefs.Field(field)
				if !ok {
					t.Errorf("expected field %q in model %T", field, test.model)
					continue
				}

				last := rel.Chain()
				for last.To() != nil {
					last = last.To()
				}

				if reflect.TypeOf(last.Model()) != exp.final {
					t.Errorf("expected final model type %v for %q, got %v", exp.final, field, reflect.TypeOf(last.Model()))
				}
			}

			t.Logf("model %T has %d forward relations and %d reverse relations", test.model, meta.Forward.Len(), meta.Reverse.Len())
			for head := meta.Forward.Front(); head != nil; head = head.Next() {
				var rel = head.Value
				var field = head.Key
				t.Logf("forward relation %q: %T.%s", field, rel.Chain().Model(), rel.Chain().Field().Name())
			}
			for head := meta.Reverse.Front(); head != nil; head = head.Next() {
				var rel = head.Value
				var field = head.Key
				t.Logf("reverse relation %q: %T.%s", field, rel.Chain().Model(), rel.Chain().Field().Name())
			}
		})
	}
}

func TestReverseRelations(t *testing.T) {
	var user = &User{
		Name: "TestReverseRelations",
	}

	if err := queries.CreateObject(user); err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	var meta = queries.GetModelMeta(user)
	t.Logf("model %T has %d forward relations and %d reverse relations", user, meta.Forward.Len(), meta.Reverse.Len())
	for head := meta.Forward.Front(); head != nil; head = head.Next() {
		var rel = head.Value
		var field = head.Key
		t.Logf("forward relation %q: %T.%s", field, rel.Chain().Model(), rel.Chain().Field().Name())
	}
	for head := meta.Reverse.Front(); head != nil; head = head.Next() {
		var rel = head.Value
		var field = head.Key
		t.Logf("reverse relation %q: %T.%s", field, rel.Chain().Model(), rel.Chain().Field().Name())
	}

	var todo = &Todo{
		Title:       "TestReverseRelations",
		Description: "TestReverseRelations",
		Done:        false,
		User:        user,
	}

	if err := queries.CreateObject(todo); err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	var u = &User{}
	var defs = u.FieldDefs()
	var _, ok = defs.Field("TodoSet")
	if !ok {
		t.Errorf("expected field TodoSet, got nil")
		return
	}

	var q = queries.Objects(&User{}).
		Select("ID", "Name", "TodoSet.*").
		Filter("ID", user.ID).
		First()
	var dbTodo, err = q.Exec()
	if err != nil {
		t.Errorf("expected no error, got %v (%s)", err, q.SQL())
		return
	}

	if dbTodo == nil {
		t.Errorf("expected todo not nil, got nil")
		return
	}

	// fields
	if dbTodo.Object.(*User).ID != user.ID {
		t.Errorf("expected todo ID %d, got %d", user.ID, dbTodo.Object.(*User).ID)
		return
	}

	if dbTodo.Object.(*User).Name != user.Name {
		t.Errorf("expected todo Name %q, got %q", user.Name, dbTodo.Object.(*User).Name)
		return
	}

	// TodoSet.*
	todoSet, ok := dbTodo.Object.(*User).RelatedField("TodoSet")
	if !ok {
		t.Errorf("expected todoSet field, got nil")
		return
	}

	if todoSet == nil {
		t.Errorf("expected todoSet not nil, got nil")
		return
	}

	var val, isOk = todoSet.GetValue().(*Todo)
	if val == nil || !isOk {
		t.Errorf("expected todoSet value not nil, got %v", val)
		return
	}

	if val.ID != todo.ID {
		t.Errorf("expected todoSet ID %d, got %d", todo.ID, val.ID)
		return
	}

	if val.Title != todo.Title {
		t.Errorf("expected todoSet Title %q, got %q", todo.Title, val.Title)
		return
	}

	if val.Description != todo.Description {
		t.Errorf("expected todoSet Description %q, got %q", todo.Description, val.Description)
		return
	}

	if val.Done != todo.Done {
		t.Errorf("expected todoSet Done %v, got %v", todo.Done, val.Done)
		return
	}

	// TodoSet.User.*
	if val.User == nil {
		t.Errorf("expected todoSet User not nil, got nil")
		return
	}

	if val.User.ID != user.ID {
		t.Errorf("expected todoSet User ID %d, got %d", user.ID, val.User.ID)
		return
	}

	if val.User.Name != "" {
		t.Errorf("expected todoSet User Name %q, got %q", "", val.User.Name)
		return
	}
}

func TestReverseRelationsNested(t *testing.T) {
	var user = &User{
		Name: "TestReverseRelationsNested",
	}

	if err := queries.CreateObject(user); err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	var todo = &Todo{
		Title:       "TestReverseRelationsNested",
		Description: "TestReverseRelationsNested",
		Done:        false,
		User:        user,
	}

	if err := queries.CreateObject(todo); err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	var u = &User{}
	var defs = u.FieldDefs()
	var _, ok = defs.Field("TodoSet")
	if !ok {
		t.Errorf("expected field TodoSet, got nil")
		return
	}

	var q = queries.Objects(&User{}).
		Select("ID", "Name", "TodoSet.*", "TodoSet.User.*", "TodoSet.User.TodoSet.*", "TodoSet.User.TodoSet.User.*").
		Filter("ID", user.ID).
		Filter("TodoSet.ID", todo.ID).
		Filter("TodoSet.User.ID", user.ID).
		Filter("TodoSet.User.TodoSet.ID", todo.ID).
		Filter("TodoSet.User.TodoSet.User.ID", user.ID).
		First()
	var dbTodo, err = q.Exec()
	if err != nil {
		t.Errorf("expected no error, got %v (%s)", err, q.SQL())
		return
	}

	if dbTodo == nil {
		t.Errorf("expected todo not nil, got nil")
		return
	}

	// fields
	if dbTodo.Object.(*User).ID != user.ID {
		t.Errorf("expected todo ID %d, got %d", user.ID, dbTodo.Object.(*User).ID)
		return
	}

	if dbTodo.Object.(*User).Name != user.Name {
		t.Errorf("expected todo Name %q, got %q", user.Name, dbTodo.Object.(*User).Name)
		return
	}

	// TodoSet.*
	todoSet, ok := dbTodo.Object.(*User).RelatedField("TodoSet")
	if !ok {
		t.Errorf("expected todoSet field, got nil")
		return
	}

	if todoSet == nil {
		t.Errorf("expected todoSet not nil, got nil")
		return
	}

	var val, isOk = todoSet.GetValue().(*Todo)
	if val == nil || !isOk {
		t.Errorf("expected todoSet value not nil, got %v", val)
		return
	}

	if val.ID != todo.ID {
		t.Errorf("expected todoSet ID %d, got %d", todo.ID, val.ID)
		return
	}

	if val.Title != todo.Title {
		t.Errorf("expected todoSet Title %q, got %q", todo.Title, val.Title)
		return
	}

	if val.Description != todo.Description {
		t.Errorf("expected todoSet Description %q, got %q", todo.Description, val.Description)
		return
	}

	if val.Done != todo.Done {
		t.Errorf("expected todoSet Done %v, got %v", todo.Done, val.Done)
		return
	}

	// TodoSet.User.*
	if val.User == nil {
		t.Errorf("expected todoSet User not nil, got nil")
		return
	}

	if val.User.ID != user.ID {
		t.Errorf("expected todoSet User ID %d, got %d", user.ID, val.User.ID)
		return
	}

	if val.User.Name != user.Name {
		t.Errorf("expected todoSet User Name %q, got %q", user.Name, val.User.Name)
		return
	}

	// TodoSet.User.TodoSet.*
	todoSet, ok = val.User.RelatedField("TodoSet")
	if !ok {
		t.Errorf("expected user.todoSet field, got nil")
		return
	}

	if todoSet == nil {
		t.Errorf("expected user.todoSet not nil, got nil")
		return
	}

	val, isOk = todoSet.GetValue().(*Todo)
	if val == nil || !isOk {
		t.Errorf("expected user.todoSet value not nil, got %v", val)
		return
	}

	if val.ID != todo.ID {
		t.Errorf("expected user.todoSet ID %d, got %d", todo.ID, val.ID)
		return
	}

	if val.Title != todo.Title {
		t.Errorf("expected user.todoSet Title %q, got %q", todo.Title, val.Title)
		return
	}

	if val.Description != todo.Description {
		t.Errorf("expected user.todoSet Description %q, got %q", todo.Description, val.Description)
		return
	}

	if val.Done != todo.Done {
		t.Errorf("expected user.todoSet Done %v, got %v", todo.Done, val.Done)
		return
	}

	// TodoSet.User.TodoSet.User.*
	if val.User == nil {
		t.Errorf("expected user.todoSet User not nil, got nil")
		return
	}

	if val.User.ID != user.ID {
		t.Errorf("expected user.todoSet User ID %d, got %d", user.ID, val.User.ID)
		return
	}

	if val.User.Name != user.Name {
		t.Errorf("expected user.todoSet User Name %q, got %q", user.Name, val.User.Name)
		return
	}

	todoSet, ok = val.User.RelatedField("TodoSet")
	if !ok {
		t.Errorf("expected user.todoSet field, got nil")
		return
	}

	if todoSet == nil {
		t.Errorf("expected user.todoSet not nil, got nil")
		return
	}
}
