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

type walkFieldPathsExpected struct {
	definer       attrs.Definer
	parent        attrs.Definer
	field         attrs.Field
	chain         []string
	relationchain [][]string
	relationType  attrs.RelationType
}

type walkFieldPathsTest struct {
	name     string
	model    attrs.Definer
	column   string
	expected walkFieldPathsExpected
}

var walkFieldsTests2 = []walkFieldPathsTest{
	{
		name:   "TestTodoID",
		model:  &Todo{},
		column: "ID",
		expected: walkFieldPathsExpected{
			definer: &Todo{},
			field:   getField(&Todo{}, "ID"),
			chain:   []string{"ID"},
		},
	},
	{
		name:   "TestTodoUser",
		model:  &Todo{},
		column: "User",
		expected: walkFieldPathsExpected{
			definer:       &Todo{},
			field:         getField(&Todo{}, "User"),
			chain:         []string{"User"},
			relationchain: [][]string{{"User.Todo", "ID.User"}},
			relationType:  attrs.RelOneToOne,
		},
	},
	{
		name:   "TestTodoUserWithID",
		model:  &Todo{},
		column: "User.Name",
		expected: walkFieldPathsExpected{
			definer:       &User{},
			parent:        &Todo{},
			field:         getField(&User{}, "Name"),
			chain:         []string{"User", "Name"},
			relationchain: [][]string{{"User.Todo", "ID.User"}},
			relationType:  attrs.RelOneToOne,
		},
	},
	{
		name:   "TestUserWithTodoID",
		model:  &User{},
		column: "Todo.Title",
		expected: walkFieldPathsExpected{
			definer:       &Todo{},
			parent:        &User{},
			field:         getField(&Todo{}, "Title"),
			chain:         []string{"Todo", "Title"},
			relationchain: [][]string{{"ID.User", "User.Todo"}},
			relationType:  attrs.RelOneToOne,
		},
	},
	{
		name:   "TestObjectWithMultipleRelationsID1",
		model:  &ObjectWithMultipleRelations{},
		column: "Obj1.Name",
		expected: walkFieldPathsExpected{
			definer:       &User{},
			parent:        &ObjectWithMultipleRelations{},
			field:         getField(&User{}, "Name"),
			chain:         []string{"Obj1", "Name"},
			relationchain: [][]string{{"Obj1.ObjectWithMultipleRelations", "ID.User"}},
			relationType:  attrs.RelManyToOne,
		},
	},
	{
		name:   "TestObjectWithMultipleRelationsID2",
		model:  &ObjectWithMultipleRelations{},
		column: "Obj2.Name",
		expected: walkFieldPathsExpected{
			definer:       &User{},
			parent:        &ObjectWithMultipleRelations{},
			field:         getField(&User{}, "Name"),
			chain:         []string{"Obj2", "Name"},
			relationchain: [][]string{{"Obj2.ObjectWithMultipleRelations", "ID.User"}},
			relationType:  attrs.RelManyToOne,
		},
	},
	{
		name:   "TestNestedCategoriesParent",
		model:  &Category{},
		column: "Parent.Parent",
		expected: walkFieldPathsExpected{
			definer:       &Category{},
			parent:        &Category{},
			field:         getField(&Category{}, "Parent"),
			chain:         []string{"Parent", "Parent"},
			relationchain: [][]string{{"Parent.Category", "ID.Category"}, {"Parent.Category", "ID.Category"}},
			relationType:  attrs.RelManyToOne,
		},
	},
	{
		name:   "TestNestedCategoriesName",
		model:  &Category{},
		column: "Parent.Parent.Name",
		expected: walkFieldPathsExpected{
			definer:       &Category{},
			parent:        &Category{},
			field:         getField(&Category{}, "Name"),
			chain:         []string{"Parent", "Parent", "Name"},
			relationchain: [][]string{{"Parent.Category", "ID.Category"}, {"Parent.Category", "ID.Category"}},
			relationType:  attrs.RelManyToOne,
		},
	},
	{
		name:   "TestOneToOneWithThrough",
		model:  &OneToOneWithThrough{},
		column: "Target.Name",
		expected: walkFieldPathsExpected{
			definer:       &OneToOneWithThrough_Target{},
			parent:        &OneToOneWithThrough{},
			field:         getField(&OneToOneWithThrough_Target{}, "Name"),
			chain:         []string{"Target", "Name"},
			relationchain: [][]string{{"Target.OneToOneWithThrough", "SourceModel.OneToOneWithThrough_Through.TargetModel", "ID.OneToOneWithThrough_Target"}},
			relationType:  attrs.RelOneToOne,
		},
	},
	{
		name:   "TestOneToOneWithThroughTarget",
		model:  &OneToOneWithThrough_Target{},
		column: "TargetReverse.Title",
		expected: walkFieldPathsExpected{
			definer:       &OneToOneWithThrough{},
			parent:        &OneToOneWithThrough_Target{},
			field:         getField(&OneToOneWithThrough{}, "Title"),
			chain:         []string{"TargetReverse", "Title"},
			relationchain: [][]string{{"TargetReverse.OneToOneWithThrough_Target", "TargetModel.OneToOneWithThrough_Through.SourceModel", "ID.OneToOneWithThrough"}},
			relationType:  attrs.RelOneToOne,
		},
	},
	{
		name:   "TestOneToOneWithThroughNested",
		model:  &OneToOneWithThrough{},
		column: "Target.TargetReverse.Target.Name",
		expected: walkFieldPathsExpected{
			definer:      &OneToOneWithThrough_Target{},
			parent:       &OneToOneWithThrough{},
			field:        getField(&OneToOneWithThrough_Target{}, "Name"),
			chain:        []string{"Target", "TargetReverse", "Target", "Name"},
			relationType: attrs.RelOneToOne,
			relationchain: [][]string{
				{"Target.OneToOneWithThrough", "SourceModel.OneToOneWithThrough_Through.TargetModel", "ID.OneToOneWithThrough_Target"},
				{"TargetReverse.OneToOneWithThrough_Target", "TargetModel.OneToOneWithThrough_Through.SourceModel", "ID.OneToOneWithThrough"},
				{"Target.OneToOneWithThrough", "SourceModel.OneToOneWithThrough_Through.TargetModel", "ID.OneToOneWithThrough_Target"},
			},
		},
	},
	{
		name:   "TestOneToOneWithThroughTargetNested",
		model:  &OneToOneWithThrough_Target{},
		column: "TargetReverse.Target.TargetReverse.Title",
		expected: walkFieldPathsExpected{
			definer:      &OneToOneWithThrough{},
			parent:       &OneToOneWithThrough_Target{},
			field:        getField(&OneToOneWithThrough{}, "Title"),
			chain:        []string{"TargetReverse", "Target", "TargetReverse", "Title"},
			relationType: attrs.RelOneToOne,
			relationchain: [][]string{
				{"TargetReverse.OneToOneWithThrough_Target", "TargetModel.OneToOneWithThrough_Through.SourceModel", "ID.OneToOneWithThrough"},
				{"Target.OneToOneWithThrough", "SourceModel.OneToOneWithThrough_Through.TargetModel", "ID.OneToOneWithThrough_Target"},
				{"TargetReverse.OneToOneWithThrough_Target", "TargetModel.OneToOneWithThrough_Through.SourceModel", "ID.OneToOneWithThrough"},
			},
		},
	},
}

func nameParts(name string) (front, back string) {
	var parts = strings.Split(name, ".")
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[len(parts)-1]
}

func TestWalkFieldPaths(t *testing.T) {
	for _, test := range walkFieldsTests2 {

		attrs.RegisterModel(test.model)

		t.Run(test.name, func(t *testing.T) {

			var modelMeta = attrs.GetModelMeta(test.model)
			if modelMeta == nil {
				t.Errorf("expected modelMeta not nil, got nil")
				return
			}

			var relationsFwd = modelMeta.ForwardMap()
			var relationsRev = modelMeta.ReverseMap()

			for head := relationsFwd.Front(); head != nil; head = head.Next() {
				var key = head.Key
				var value = head.Value
				t.Logf("forward relation %s -> %T.%s", key, value.Model(), value.Field().Name())
			}

			for head := relationsRev.Front(); head != nil; head = head.Next() {
				var key = head.Key
				var value = head.Value
				t.Logf("reverse relation %s -> %T.%s", key, value.Model(), value.Field().Name())
			}

			var meta, err = internal.WalkFieldPath(test.model, test.column)
			if err != nil {
				t.Errorf("expected no error, got %v %v", err, attrs.FieldNames(test.model, nil))
				return
			}

			if meta == nil {
				t.Errorf("expected meta not nil, got nil")
				return
			}

			if meta.Last() == nil || meta.Last().Object == nil {
				t.Errorf("expected meta.Last not nil, got nil")
				return
			}

			if reflect.TypeOf(meta.Last().Object) != reflect.TypeOf(test.expected.definer) {
				t.Errorf("expected meta.Last.Object %T, got %T (%T)", test.expected.definer, meta.Last().Object, meta.First().Object)
			}

			if meta.Last().Parent() != nil {
				if reflect.TypeOf(meta.Last().Parent().Object) != reflect.TypeOf(test.expected.parent) {
					t.Errorf("expected meta.Last.Parent.Object %T, got %T", test.expected.parent, meta.Last().Parent().Object)
				}
			}

			if test.expected.parent == nil && meta.Last().Parent() != nil {
				t.Errorf("expected meta.Last.Parent nil, got %T", meta.Last().Parent().Object)
			}

			if !fieldEquals(meta.Last().Field, test.expected.field) {
				t.Errorf("expected meta.Last.Field %T.%s, got %T.%s", test.expected.field.Instance(), test.expected.field.Name(), meta.Last().Field.Instance(), meta.Last().Field.Name())
			}

			if meta.Last().String() != strings.Join(test.expected.chain, ".") {
				t.Errorf("expected meta.Last.String() %s, got %s", strings.Join(test.expected.chain, "."), meta.Last().String())
			}

		metaLoop:
			for i := range meta {
				var current = meta[i]

				if test.expected.relationchain == nil {
					if current.Relation != nil {
						t.Errorf("expected meta[%d].Relation nil, got %T", i, current.Relation)
					}
					continue metaLoop
				}

				var rel = current.Relation
				switch {
				case i >= len(test.expected.relationchain) && rel != nil:
					t.Errorf("expected meta[%d].Relation nil, got %T", i, rel)
					continue metaLoop
				case i >= len(test.expected.relationchain) && rel == nil:
					continue metaLoop
				}

				if len(test.expected.relationchain[i]) == 0 {
					continue metaLoop
				}

				var (
					from        = rel.From()
					targetModel = rel.Model()
					targetField = rel.Field()
				)

				if from == nil {
					t.Errorf("expected meta[%d].Relation.From() not nil, got nil, %T -> %T.%s", i, from, targetModel, targetField.Name())
					continue metaLoop
				}

				var (
					fromModel = from.Model()
					fromField = from.Field()
					through   = rel.Through()
				)

				if targetModel == nil {
					t.Errorf("expected meta[%d].Relation.Model() not nil, got nil", i)
					continue metaLoop
				}

				if targetField == nil {
					t.Errorf("expected meta[%d].Relation.Field() not nil, got nil", i)
					continue metaLoop
				}

				if fromModel == nil || fromField == nil {
					t.Errorf("expected meta[%d].Relation.From() model/field not nil, got %T/%T", i, fromModel, fromField)
					continue metaLoop
				}

				if len(test.expected.relationchain[i]) == 3 {
					if through == nil {
						t.Errorf("expected meta[%d].Relation.Through() not nil, got nil", i)
						continue metaLoop
					}

					var (
						sourceFieldStr  string
						throughModelStr string
						targetFieldStr  string
					)

					var parts = strings.Split(test.expected.relationchain[i][1], ".")
					if len(parts) != 3 {
						t.Error("Malformed test.expected.relationchain, expected 3 parts for through relation")
						continue metaLoop
					}

					sourceFieldStr = parts[0]
					throughModelStr = parts[1]
					targetFieldStr = parts[2]

					if sourceFieldStr != through.SourceField() {
						t.Errorf("expected meta[%d].Relation.Through.SourceField() %s, got %s", i, sourceFieldStr, through.SourceField())
					}

					var rtyp = reflect.TypeOf(through.Model())
					if rtyp.Kind() == reflect.Ptr {
						rtyp = rtyp.Elem()
					}

					if throughModelStr != rtyp.Name() {
						t.Errorf("expected meta[%d].Relation.Through.Model() %q, got %q", i, throughModelStr, rtyp.Name())
					}

					if targetFieldStr != through.TargetField() {
						t.Errorf("expected meta[%d].Relation.Through.TargetField() %q, got %q", i, targetFieldStr, through.TargetField())
					}

					t.Logf(
						"through relation %T.%s -> %T.%s, %T.%s -> %T.%s",
						fromModel, fromModel.FieldDefs().Primary().Name(), through.Model(), sourceFieldStr,
						through.Model(), targetFieldStr, targetModel, targetModel.FieldDefs().Primary().Name(),
					)
				} else {
					t.Logf("relation %T.%s -> %T.%s", fromModel, fromField.Name(), targetModel, targetField.Name())
				}

				if rel.Type() != test.expected.relationType {
					t.Errorf("expected meta[%d].Relation.Type() %d, got %d", i, test.expected.relationType, rel.Type())
				}

				//var checkPart = func(t *testing.T, i, j int, part string) {
				//	var expectedField, expectedModel = nameParts(part)
				//
				//	var (
				//		rtyp  = reflect.TypeOf(model)
				//	)
				//
				//	if rtyp.Kind() == reflect.Ptr {
				//		rtyp = rtyp.Elem()
				//	}
				//
				//	var modelName = rtyp.Name()
				//	if modelName == "" {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] model name empty for %s (%T)", i, j, part, model)
				//		return
				//	}
				//
				//	if modelName != expectedModel {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] model %s, got %s", i, j, expectedModel, modelName)
				//		return
				//	}
				//
				//	if field.Name() != expectedField {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] field %s, got %s", i, j, expectedField, field.Name())
				//		return
				//	}
				//}

				//var chain = rel.Chain()
				//for j, part := range test.expected.relationchain[i] {
				//	var expectedField, expectedModel = nameParts(part)
				//
				//	var (
				//		model = chain.Model()
				//		field = chain.Field()
				//		rtyp  = reflect.TypeOf(model)
				//	)
				//
				//	if rtyp.Kind() == reflect.Ptr {
				//		rtyp = rtyp.Elem()
				//	}
				//
				//	var modelName = rtyp.Name()
				//	if modelName == "" {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] model name empty for %s (%T)", i, j, part, model)
				//		break
				//	}
				//
				//	if modelName != expectedModel {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] model %s, got %s", i, j, expectedModel, modelName)
				//		break
				//	}
				//
				//	if field.Name() != expectedField {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] field %s, got %s", i, j, expectedField, field.Name())
				//		break
				//	}
				//
				//	chain = chain.To()
				//
				//	if chain == nil && j < len(test.expected.relationchain[i])-1 {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] child relation not nil, got nil %v", i, j, test.expected.relationchain[i])
				//		break
				//	}
				//
				//	if chain != nil && j >= len(test.expected.relationchain[i])-1 {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] child relation nil, got %T", i, j, model)
				//		break
				//	}
				//}

				//var j = 0
				//for chain != nil {
				//	var rTyp = reflect.TypeOf(chain.Model())
				//	var _, modelName = nameParts(rTyp.Name())
				//
				//	chainNames = append(chainNames, fmt.Sprintf(
				//		"%s.%s",
				//		chain.Field().Name(),
				//		modelName,
				//	))
				//	if j >= len(test.expected.relationchain[i]) {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] length %d, got %d %v", i, j, len(test.expected.relationchain[i]), len(chainNames), chainNames)
				//		break
				//	}
				//
				//	var expectedNameParts = test.expected.relationchain[i][j]
				//	var expectedField, expectedModel = nameParts(expectedNameParts)
				//
				//	if modelName != expectedModel {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] model %s, got %s %v", i, j, test.expected.relationchain[i][j], modelName, chainNames)
				//	}
				//
				//	if chain.Field().Name() != expectedField {
				//		t.Errorf("expected meta[%d].Relation.Chain()[%d] field %s, got %s %v", i, j, test.expected.relationchain[i][j], chain.Field().Name(), chainNames)
				//	}
				//
				//	j++
				//	chain = chain.To()
				//}
			}

			//var sb = strings.Builder{}
			//for _, current := range meta {
			//	if current.Field != nil {
			//		sb.WriteString(current.Field.Name())
			//	}
			//	if current.Child() != nil {
			//		sb.WriteString(".")
			//	}
			//}
			//
			//t.Logf("meta string = %s", sb.String())
			//t.Logf("meta.Last.String() = %s", meta.Last().String())
		})
	}
}
