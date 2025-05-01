package queries_test

import (
	"database/sql"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

const (
	createTableTestStruct = `CREATE TABLE IF NOT EXISTS test_struct (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT,
	text TEXT
)`
)

func init() {
	var db, err = sql.Open("sqlite3", "file:queries_memory?mode=memory&cache=shared")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	_, err = db.Exec(createTableTestStruct)
	if err != nil {
		panic(err)
	}
}

type TestStruct struct {
	queries.BaseModel
	ID   int64
	Name string
	Text string
}

func (t *TestStruct) FieldDefs() attrs.Definitions {
	return attrs.Define(t,
		attrs.NewField(t, "ID", &attrs.FieldConfig{
			Column:  "id",
			Primary: true,
		}),
		attrs.NewField(t, "Name", &attrs.FieldConfig{
			Column: "name",
		}),
		attrs.NewField(t, "Text", &attrs.FieldConfig{
			Column: "text",
		}),
		queries.NewVirtualField[string](t, t, "TestNameText", &queries.RawExpr{
			Statement: "%s || ' ' || %s || ' ' || ?",
			Fields:    []string{"Name", "Text"},
			Params:    []any{"test"},
		}),
		queries.NewVirtualField[string](t, t, "TestNameLower", &queries.RawExpr{
			Statement: "LOWER(%s)",
			Fields:    []string{"Name"},
		}),
		queries.NewVirtualField[string](t, t, "TestNameUpper", &queries.RawExpr{
			Statement: "UPPER(%s)",
			Fields:    []string{"Name"},
		}),
	).WithTableName("test_struct")
}

func TestSetName(t *testing.T) {
	var test = &TestStruct{}
	var defs = test.FieldDefs()

	var (
		fText, _  = defs.Field("TestNameText")
		fLower, _ = defs.Field("TestNameLower")
		fUpper, _ = defs.Field("TestNameUpper")
	)

	fText.SetValue("test1", false)
	fLower.SetValue("test2", false)
	fUpper.SetValue("test3", false)

	var (
		textV, _  = test.BaseModel.Get("TestNameText")
		lowerV, _ = test.BaseModel.Get("TestNameLower")
		upperV, _ = test.BaseModel.Get("TestNameUpper")
	)

	if textV != "test1" {
		t.Errorf("Expected TestNameText to be 'test1 test2', got %v", textV)
	}

	if lowerV != "test2" {
		t.Errorf("Expected TestNameLower to be 'test2', got %v", lowerV)
	}

	if upperV != "test3" {
		t.Errorf("Expected TestNameUpper to be 'test3', got %v", upperV)
	}

	if fText.GetValue() != "test1" {
		t.Errorf("Expected fText to be 'test1', got %v", fText.GetValue())
	}

	if fLower.GetValue() != "test2" {
		t.Errorf("Expected fLower to be 'test2', got %v", fLower.GetValue())
	}

	if fUpper.GetValue() != "test3" {
		t.Errorf("Expected fUpper to be 'test3', got %v", fUpper.GetValue())
	}

	t.Logf("Test: %+v", test)
}

func TestVirtualFieldsQuerySetSingleObject(t *testing.T) {
	var test = &TestStruct{
		Name: "test1",
		Text: "test2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v, %T", err, err)
	}

	var qs = queries.Objects(test)
	qs = qs.Select("*")
	qs = qs.Filter("ID", test.ID)
	qs = qs.Filter("TestNameLower", "test1")
	qs = qs.Filter("TestNameUpper", "TEST1")
	qs = qs.OrderBy("-TestNameText")

	var a = qs.Get()
	var (
		sql      = a.SQL()
		args     = a.Args()
		obj, err = a.Exec()
	)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	var o = obj.(*TestStruct)
	if o.ID != test.ID {
		t.Errorf("Expected ID to be %d, got %d", test.ID, o.ID)
	}

	if o.Name != test.Name {
		t.Errorf("Expected Name to be %q, got %q", test.Name, o.Name)
	}

	if o.Text != test.Text {
		t.Errorf("Expected Text to be %q, got %q", test.Text, o.Text)
	}

	var textV, _ = o.BaseModel.Get("TestNameText")
	if textV != "test1 test2 test" {
		t.Errorf("Expected TestNameText to be 'test1 test2', got %v", textV)
	}

	var lowerV, _ = o.BaseModel.Get("TestNameLower")
	if lowerV != "test1" {
		t.Errorf("Expected TestNameLower to be 'test1', got %v", lowerV)
	}

	var upperV, _ = o.BaseModel.Get("TestNameUpper")
	if upperV != "TEST1" {
		t.Errorf("Expected TestNameUpper to be 'TEST1', got %v", upperV)
	}

	t.Logf("SQL: %s", sql)
	t.Logf("Args: %v", args)
	t.Logf("Object: %+v", obj)
}
