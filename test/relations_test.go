package queries_test

import (
	"reflect"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type relationTestExpected struct {
	type_ attrs.RelationType
	final reflect.Type
	chain []string
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
				type_: attrs.RelManyToOne,
				final: getType(&Category{}),
			},
		},
		expectsRev: map[string]relationTestExpected{
			"CategorySet": {
				type_: attrs.RelOneToMany,
				final: getType(&Category{}),
			},
		},
	},
	{
		name:  "ExpectedReverseRelation",
		model: &Todo{},
		expectsFwd: map[string]relationTestExpected{
			"User": {
				type_: attrs.RelOneToOne,
				final: getType(&User{}),
			},
		},
		expectsRev: map[string]relationTestExpected{},
	},
	{
		name:  "ExpectedReverseRelation",
		model: &User{},
		expectsRev: map[string]relationTestExpected{
			"Todo": {
				type_: attrs.RelOneToOne,
				final: getType(&Todo{}),
			},
		},
	},
}

func TestRegisterModelRelations(t *testing.T) {

	for _, test := range tests {
		test.fieldDefs = test.model.FieldDefs()
		t.Run(test.name, func(t *testing.T) {
			attrs.RegisterModel(test.model)
			meta := attrs.GetModelMeta(test.model)

			for field, exp := range test.expectsFwd {
				rel, ok := meta.Forward(field)
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

				if reflect.TypeOf(rel.Model()) != exp.final {
					t.Errorf("expected final model type %v for %q, got %v", exp.final, field, reflect.TypeOf(rel.Model()))
				}
			}

			for field, exp := range test.expectsRev {
				rel, ok := meta.Reverse(field)
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

				if reflect.TypeOf(rel.Model()) != exp.final {
					t.Errorf("expected final model type %v for %q, got %v", exp.final, field, reflect.TypeOf(rel.Model()))
				}
			}

			t.Logf("model %T has %d forward relations and %d reverse relations", test.model, meta.ForwardMap().Len(), meta.ReverseMap().Len())
			for head := meta.ForwardMap().Front(); head != nil; head = head.Next() {
				field := head.Key
				rel := head.Value
				if rel == nil {
					t.Errorf("expected forward relation %q, got nil", field)
					continue
				}
				model := rel.Model()
				f := rel.Field()
				if f == nil {
					t.Errorf("expected forward relation %q, got nil", field)
					continue
				}
				t.Logf("forward relation %q: %T.%s", field, model, f.Name())
			}
			for head := meta.ReverseMap().Front(); head != nil; head = head.Next() {
				field := head.Key
				rel := head.Value
				if rel == nil {
					t.Errorf("expected reverse relation %q, got nil", field)
					continue
				}
				model := rel.Model()
				f := rel.Field()
				if f == nil {
					t.Errorf("expected reverse relation %q, got nil", field)
					continue
				}

				t.Logf("reverse relation %q: %T.%s", field, model, f.Name())
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

	var meta = attrs.GetModelMeta(user)
	t.Logf("model %T has %d forward relations and %d reverse relations", user, meta.ForwardMap().Len(), meta.ReverseMap().Len())
	for head := meta.ForwardMap().Front(); head != nil; head = head.Next() {
		field := head.Key
		rel := head.Value
		t.Logf("forward relation %q: %T.%s", field, rel.Model(), rel.Field().Name())
	}
	for head := meta.ReverseMap().Front(); head != nil; head = head.Next() {
		field := head.Key
		rel := head.Value
		t.Logf("reverse relation %q: %T.%s", field, rel.Model(), rel.Field().Name())
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
	var _, ok = defs.Field("Todo")
	if !ok {
		t.Errorf("expected field Todo, got nil")
		return
	}

	var q = queries.Objects(&User{}).
		Select("ID", "Name", "Todo.*").
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

	// Todo.*
	todoSet, ok := dbTodo.Object.(*User).RelatedField("Todo")
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

	// Todo.User.*
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
	var _, ok = defs.Field("Todo")
	if !ok {
		t.Errorf("expected field Todo, got nil")
		return
	}

	var q = queries.Objects(&User{}).
		Select("ID", "Name", "Todo.*", "Todo.User.*", "Todo.User.Todo.*", "Todo.User.Todo.User.*").
		Filter("ID", user.ID).
		Filter("Todo.ID", todo.ID).
		Filter("Todo.User.ID", user.ID).
		Filter("Todo.User.Todo.ID", todo.ID).
		Filter("Todo.User.Todo.User.ID", user.ID).
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

	// Todo.*
	todoSet, ok := dbTodo.Object.(*User).RelatedField("Todo")
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

	// Todo.User.*
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

	// Todo.User.Todo.*
	todoSet, ok = val.User.RelatedField("Todo")
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

	// Todo.User.Todo.User.*
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

	todoSet, ok = val.User.RelatedField("Todo")
	if !ok {
		t.Errorf("expected user.todoSet field, got nil")
		return
	}

	if todoSet == nil {
		t.Errorf("expected user.todoSet not nil, got nil")
		return
	}
}
func TestOneToOneWithThrough(t *testing.T) {
	// Create the target
	target := &OneToOneWithThrough_Target{
		Name: "Target Name",
		Age:  42,
	}
	if err := queries.CreateObject(target); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create the main object
	main := &OneToOneWithThrough{
		Title: "Main Title",
	}
	if err := queries.CreateObject(main); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}

	// Create the through relation manually
	through := &OneToOneWithThrough_Through{
		SourceModel: main,
		TargetModel: target,
	}
	if err := queries.CreateObject(through); err != nil {
		t.Fatalf("failed to create through: %v", err)
	}

	// Query and include the through-relation
	var q = queries.Objects(&OneToOneWithThrough{}).
		Select("ID", "Title", "Target.*").
		Filter("ID", main.ID).
		First()

	result, err := q.Exec()
	if err != nil {
		t.Fatalf("query failed: %v (%s)", err, q.SQL())
	}
	if result == nil {
		t.Fatalf("expected result, got nil")
	}

	obj := result.Object.(*OneToOneWithThrough)
	if obj.Title != main.Title {
		t.Errorf("expected title %q, got %q", main.Title, obj.Title)
	}

	if obj.Through == nil {
		t.Fatalf("expected Through field not nil")
	}

	var targetVal = obj.Through
	if targetVal.ID != target.ID {
		t.Errorf("expected target ID %d, got %d", target.ID, targetVal.ID)
	}
	if targetVal.Name != target.Name {
		t.Errorf("expected target Name %q, got %q", target.Name, targetVal.Name)
	}
	if targetVal.Age != target.Age {
		t.Errorf("expected target Age %d, got %d", target.Age, targetVal.Age)
	}
}

func TestOneToOneWithThroughReverse(t *testing.T) {
	target := &OneToOneWithThrough_Target{
		Name: "ReverseTarget",
		Age:  30,
	}
	if err := queries.CreateObject(target); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	main := &OneToOneWithThrough{
		Title: "ReverseMain",
	}
	if err := queries.CreateObject(main); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}

	through := &OneToOneWithThrough_Through{
		SourceModel: main,
		TargetModel: target,
	}
	if err := queries.CreateObject(through); err != nil {
		t.Fatalf("failed to create through: %v", err)
	}

	// Now test reverse relation (Target → Main)
	result, err := queries.Objects(&OneToOneWithThrough_Target{}).
		Select("ID", "Name", "TargetReverse.*"). // TargetReverse is the reverse field name
		Filter("ID", target.ID).
		First().
		Exec()
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	obj := result.Object.(*OneToOneWithThrough_Target)
	if obj.Name != target.Name {
		t.Errorf("expected name %q, got %q", target.Name, obj.Name)
	}

	reverseVal, ok := obj.RelatedField("TargetReverse")
	if !ok || reverseVal == nil {
		t.Fatalf("expected reverse field, got nil")
	}

	source := reverseVal.GetValue().(*OneToOneWithThrough)
	if source == nil {
		t.Fatalf("expected source, got nil")
	}

	if source.ID != main.ID {
		t.Errorf("expected reverse ID %d, got %d", main.ID, source.ID)
	}
	if source.Title != main.Title {
		t.Errorf("expected reverse title %q, got %q", main.Title, source.Title)
	}
}

func TestOneToOneWithThroughReverseIntoForward(t *testing.T) {
	target := &OneToOneWithThrough_Target{
		Name: "ReverseTarget",
		Age:  30,
	}
	if err := queries.CreateObject(target); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	user := &User{
		Name: "TestOneToOneWithThroughReverseIntoForward",
	}
	if err := queries.CreateObject(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	main := &OneToOneWithThrough{
		Title: "ReverseMain",
		User:  user,
	}
	if err := queries.CreateObject(main); err != nil {
		t.Fatalf("failed to create main: %v", err)
	}

	through := &OneToOneWithThrough_Through{
		SourceModel: main,
		TargetModel: target,
	}
	if err := queries.CreateObject(through); err != nil {
		t.Fatalf("failed to create through: %v", err)
	}

	// Now test reverse relation (Target → Main)
	result, err := queries.Objects(&OneToOneWithThrough_Target{}).
		Select("ID", "Name", "TargetReverse.*", "TargetReverse.User.*"). // TargetReverse is the reverse field name
		Filter("ID", target.ID).
		First().
		Exec()
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	obj := result.Object.(*OneToOneWithThrough_Target)
	if obj.Name != target.Name {
		t.Errorf("expected name %q, got %q", target.Name, obj.Name)
	}

	reverseVal, ok := obj.RelatedField("TargetReverse")
	if !ok || reverseVal == nil {
		t.Fatalf("expected reverse field, got nil")
	}

	source := reverseVal.GetValue().(*OneToOneWithThrough)
	if source == nil {
		t.Fatalf("expected source, got nil")
	}

	if source.ID != main.ID {
		t.Errorf("expected reverse ID %d, got %d", main.ID, source.ID)
	}
	if source.Title != main.Title {
		t.Errorf("expected reverse title %q, got %q", main.Title, source.Title)
	}
	if source.User == nil {
		t.Fatalf("expected source user, got nil")
	}

	if source.User.ID != user.ID {
		t.Errorf("expected source user ID %d, got %d", user.ID, source.User.ID)
	}

	if source.User.Name != user.Name {
		t.Errorf("expected source user name %q, got %q", user.Name, source.User.Name)
	}
}

//
//func TestOneToOneWithThroughNested(t *testing.T) {
//	target := &OneToOneWithThrough_Target{
//		Name: "NestedTarget",
//		Age:  25,
//	}
//	if err := queries.CreateObject(target); err != nil {
//		t.Fatalf("create target: %v", err)
//	}
//
//	main := &OneToOneWithThrough{
//		Title: "NestedMain",
//	}
//	if err := queries.CreateObject(main); err != nil {
//		t.Fatalf("create main: %v", err)
//	}
//
//	through := &OneToOneWithThrough_Through{
//		SourceModel: main,
//		TargetModel: target,
//	}
//	if err := queries.CreateObject(through); err != nil {
//		t.Fatalf("create through: %v", err)
//	}
//
//	// Nested: Target → Reverse → Target
//	result, err := queries.Objects(&OneToOneWithThrough_Target{}).
//		Select("ID", "Name", "TargetReverse.*", "TargetReverse.Target.*").
//		Filter("ID", target.ID).
//		First().
//		Exec()
//
//	if err != nil {
//		t.Fatalf("nested query failed: %v", err)
//	}
//
//	obj := result.Object.(*OneToOneWithThrough_Target)
//
//	reverse, ok := obj.RelatedField("TargetReverse")
//	if !ok || reverse == nil {
//		t.Fatalf("expected Reverse relation")
//	}
//	mainObj := reverse.GetValue().(*OneToOneWithThrough)
//	if mainObj == nil {
//		t.Fatalf("expected main from reverse")
//	}
//
//	relatedTarget := mainObj.Through
//	if relatedTarget == nil || relatedTarget.ID != target.ID {
//		t.Errorf("expected reloaded target ID %d, got %v", target.ID, relatedTarget)
//	}
//}
//
