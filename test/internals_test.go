package queries_test

import (
	"reflect"
	"strings"
	"testing"
	_ "unsafe"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

//go:linkname newObjectFromIface github.com/Nigel2392/go-django-queries/internal.NewObjectFromIface
func newObjectFromIface(obj attrs.Definer) attrs.Definer

//go:linkname walkFields github.com/Nigel2392/go-django-queries/internal.WalkFields
func walkFields(
	m attrs.Definer,
	column string,
) (
	definer attrs.Definer,
	parent attrs.Definer,
	f attrs.Field,
	chain []string,
	aliases []string,
	isRelated bool,
	err error,
)

func TestNewObjectFromIface(t *testing.T) {
	var obj = &Todo{
		ID:          1,
		Title:       "Test",
		Description: "Test",
		Done:        false,
	}

	var definer = newObjectFromIface(obj)
	if definer == nil {
		t.Fatal("newObjectFromIface returned nil")
	}

	if *(definer).(*Todo) != (Todo{}) {
		t.Fatalf("newObjectFromIface returned wrong type: %T", definer)
	}
}

type walkFieldsExpected struct {
	definer   attrs.Definer
	parent    attrs.Definer
	field     attrs.Field
	chain     []string
	aliases   []string
	isRelated bool
	err       error
}

type walkFieldsTest struct {
	name     string
	model    attrs.Definer
	column   string
	expected walkFieldsExpected
}

func getField(m attrs.Definer, field string) attrs.Field {
	defs := m.FieldDefs()
	f, _ := defs.Field(field)
	return f
}

func fieldEquals(f1, f2 attrs.Field) bool {

	var (
		instance1 = f1.Instance()
		name1     = f1.Name()
		instance2 = f2.Instance()
		name2     = f2.Name()
	)

	return reflect.TypeOf(instance1) == reflect.TypeOf(instance2) && name1 == name2
}

var walkFieldsTests = []walkFieldsTest{
	{
		name:   "TestTodoID",
		model:  &Todo{},
		column: "ID",
		expected: walkFieldsExpected{
			definer:   &Todo{},
			parent:    nil,
			field:     getField(&Todo{}, "ID"),
			chain:     []string{},
			aliases:   []string{},
			isRelated: false,
			err:       nil,
		},
	},
	{
		name:   "TestTodoUser",
		model:  &Todo{},
		column: "User",
		expected: walkFieldsExpected{
			definer:   &Todo{},
			parent:    nil,
			field:     getField(&Todo{}, "User"),
			chain:     []string{},
			aliases:   []string{},
			isRelated: false,
			err:       nil,
		},
	},
	{
		name:   "TestTodoUserWithID",
		model:  &Todo{},
		column: "User.ID",
		expected: walkFieldsExpected{
			definer:   &User{},
			parent:    &Todo{},
			field:     getField(&User{}, "ID"),
			chain:     []string{"User"},
			aliases:   []string{"user_id_todos_0"},
			isRelated: true,
			err:       nil,
		},
	},
	{
		name:   "TestObjectWithMultipleRelationsID1",
		model:  &ObjectWithMultipleRelations{},
		column: "Obj1.ID",
		expected: walkFieldsExpected{
			definer:   &User{},
			parent:    &ObjectWithMultipleRelations{},
			field:     getField(&User{}, "ID"),
			chain:     []string{"Obj1"},
			aliases:   []string{"obj1_id_object_with_multiple_relations_0"},
			isRelated: true,
			err:       nil,
		},
	},
	{
		name:   "TestObjectWithMultipleRelationsID2",
		model:  &ObjectWithMultipleRelations{},
		column: "Obj2.ID",
		expected: walkFieldsExpected{
			definer:   &User{},
			parent:    &ObjectWithMultipleRelations{},
			field:     getField(&User{}, "ID"),
			chain:     []string{"Obj2"},
			aliases:   []string{"obj2_id_object_with_multiple_relations_0"},
			isRelated: true,
			err:       nil,
		},
	},
	{
		name:   "TestNestedCategoriesParent",
		model:  &Category{},
		column: "Parent.Parent",
		expected: walkFieldsExpected{
			definer:   &Category{},
			parent:    &Category{},
			field:     getField(&Category{}, "Parent"),
			chain:     []string{"Parent"},
			aliases:   []string{"parent_id_categories_0"},
			isRelated: true,
			err:       nil,
		},
	},
	{
		name:   "TestNestedCategoriesName",
		model:  &Category{},
		column: "Parent.Parent.Name",
		expected: walkFieldsExpected{
			definer:   &Category{},
			parent:    &Category{},
			field:     getField(&Category{}, "Name"),
			chain:     []string{"Parent", "Parent"},
			aliases:   []string{"parent_id_categories_0", "parent_id_categories_1"},
			isRelated: true,
			err:       nil,
		},
	},
}

func TestWalkFields(t *testing.T) {
	for _, test := range walkFieldsTests {
		t.Run(test.name, func(t *testing.T) {
			var (
				definer, parent, field, chain, aliases, isRelated, err = walkFields(test.model, test.column)
			)

			if reflect.TypeOf(definer) != reflect.TypeOf(test.expected.definer) {
				t.Errorf("expected definer %T, got %T", test.expected.definer, definer)
			}

			if test.expected.parent != nil {
				if reflect.TypeOf(parent) != reflect.TypeOf(test.expected.parent) {
					t.Errorf("expected parent %T, got %T", test.expected.parent, parent)
				}
			}

			if test.expected.parent == nil && parent != nil {
				t.Errorf("expected parent nil, got %T", parent)
			}

			if !fieldEquals(field, test.expected.field) {
				t.Errorf("expected field %T.%s, got %T.%s", test.expected.field.Instance(), test.expected.field.Name(), field.Instance(), field.Name())
			}

			if len(chain) != len(test.expected.chain) {
				t.Errorf("expected chain length %d, got %d", len(test.expected.chain), len(chain))
			} else {
				for i := range chain {
					if chain[i] != test.expected.chain[i] {
						t.Errorf("expected chain %s, got %s", test.expected.chain[i], chain[i])
					}
				}
			}

			if len(aliases) != len(test.expected.aliases) {
				t.Errorf("expected aliases length %d, got %d", len(test.expected.aliases), len(aliases))
			} else {
				for i := range aliases {
					if aliases[i] != test.expected.aliases[i] {
						t.Errorf("expected alias %s, got %s", test.expected.aliases[i], aliases[i])
					}
				}
			}

			if isRelated != test.expected.isRelated {
				t.Errorf("expected isRelated %v, got %v", test.expected.isRelated, isRelated)
			}

			if err != nil && err.Error() != test.expected.err.Error() {
				t.Errorf("expected error %v, got %v", test.expected.err.Error(), err.Error())
			}
		})
	}
}

func TestWalkFieldPaths(t *testing.T) {
	for _, test := range walkFieldsTests {

		t.Run(test.name, func(t *testing.T) {
			var meta, err = internal.WalkFieldPath(test.model, test.column)
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if meta == nil {
				t.Errorf("expected meta not nil, got nil")
			}

			if reflect.TypeOf(meta.Last.Object) != reflect.TypeOf(test.expected.definer) {
				t.Errorf("expected meta.Last.Object %T, got %T", test.expected.definer, meta.Last.Object)
			}

			if meta.Last.Parent != nil {
				if reflect.TypeOf(meta.Last.Parent.Object) != reflect.TypeOf(test.expected.parent) {
					t.Errorf("expected meta.Last.Parent.Object %T, got %T", test.expected.parent, meta.Last.Parent.Object)
				}
			}

			if test.expected.parent == nil && meta.Last.Parent != nil {
				t.Errorf("expected meta.Last.Parent nil, got %T", meta.Last.Parent.Object)
			}

			if !fieldEquals(meta.Last.Field, test.expected.field) {
				t.Errorf("expected meta.Last.Field %T.%s, got %T.%s", test.expected.field.Instance(), test.expected.field.Name(), meta.Last.Field.Instance(), meta.Last.Field.Name())
			}

			if meta.Last.String() != strings.Join(append(test.expected.chain, test.expected.field.Name()), ".") {
				t.Errorf("expected meta.Last.String() %s, got %s", strings.Join(append(test.expected.chain, test.expected.field.Name()), "."), meta.Last.String())
			}
		})
	}
}
