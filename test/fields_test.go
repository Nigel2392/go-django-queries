package queries_test

import (
	"database/sql"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django-queries/src/models"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

const (
	createTableTestStruct = `CREATE TABLE IF NOT EXISTS test_struct (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT,
	text TEXT
)`
	createTableTestStructNoObject = `CREATE TABLE IF NOT EXISTS test_struct_no_object (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT,
	text TEXT
)`
	createAuthor = `CREATE TABLE IF NOT EXISTS author (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT
)`
	createBook = `CREATE TABLE IF NOT EXISTS book (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT,
	author_id INTEGER,
	FOREIGN KEY(author_id) REFERENCES author(id)
)`
)

var (
	_ queries.DataModel = &TestStruct{}
)

func init() {
	var db, err = sql.Open("sqlite3", "file:queries_memory?mode=memory&cache=shared")
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(createTableTestStruct)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(createTableTestStructNoObject)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(createAuthor)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(createBook)
	if err != nil {
		panic(err)
	}

	attrs.RegisterModel(&TestStruct{})
	attrs.RegisterModel(&TestStructNoObject{})
	attrs.RegisterModel(&Author{})
	attrs.RegisterModel(&Book{})
}

type TestStruct struct {
	models.Model
	ID   int64
	Name string
	Text string
}

func (t *TestStruct) FieldDefs() attrs.Definitions {
	return t.Model.Define(t,
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
		fields.NewVirtualField[string](t, t, "TestNameText", expr.Raw(
			"![Name] || ' ' || ![Text] || ' ' || ?",
			"test",
		)),
		fields.NewVirtualField[string](t, t, "TestNameLower", expr.Raw(
			"LOWER(![Name])",
		)),
		fields.NewVirtualField[string](t, t, "TestNameUpper", expr.Raw(
			"UPPER(![Name])",
		)),
	).WithTableName("test_struct")
}

type TestStructNoObject struct {
	ID   int64
	Name string
	Text string

	TestNameText  string
	TestNameLower string
	TestNameUpper string
}

func (t *TestStructNoObject) FieldDefs() attrs.Definitions {
	return attrs.Define[*TestStructNoObject, any](t,
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
		fields.NewVirtualField[string](t, &t.TestNameText, "TestNameText", expr.Raw(
			"![Name] || ' ' || ![Text] || ' ' || ?",
			"test",
		)),
		fields.NewVirtualField[string](t, &t.TestNameLower, "TestNameLower", expr.Raw(
			"LOWER(![Name])",
		)),
		fields.NewVirtualField[string](t, &t.TestNameUpper, "TestNameUpper", expr.Raw(
			"UPPER(![Name])",
		)),
	).WithTableName("test_struct_no_object")
}

func TestSetNameTestStruct(t *testing.T) {
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
		textV, _  = test.DataStore().GetValue("TestNameText")
		lowerV, _ = test.DataStore().GetValue("TestNameLower")
		upperV, _ = test.DataStore().GetValue("TestNameUpper")
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

func TestSetNameTestStructNoObject(t *testing.T) {
	var test = &TestStructNoObject{}
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
		textV  = test.TestNameText
		lowerV = test.TestNameLower
		upperV = test.TestNameUpper
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
		t.Errorf("Expected fText.GetValue() to be 'test1', got %v", fText.GetValue())
	}

	if fLower.GetValue() != "test2" {
		t.Errorf("Expected fLower.GetValue() to be 'test2', got %v", fLower.GetValue())
	}

	if fUpper.GetValue() != "test3" {
		t.Errorf("Expected fUpper.GetValue() to be 'test3', got %v", fUpper.GetValue())
	}

	t.Logf("Test: %+v", test)
}

func TestVirtualFieldsQuerySetSingleObjectTestStruct(t *testing.T) {
	var test = &TestStruct{
		Name: "test1",
		Text: "test2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v, %T", err, err)
	}

	var qs = queries.Objects[attrs.Definer](test)
	qs = qs.Select("*")
	qs = qs.Filter("ID", test.ID)
	qs = qs.Filter("TestNameLower", "test1")
	qs = qs.Filter("TestNameUpper", "TEST1")
	qs = qs.OrderBy("-TestNameText")

	var obj, err = qs.Get()
	var (
		sql  = qs.LatestQuery().SQL()
		args = qs.LatestQuery().Args()
	)
	if err != nil {
		t.Fatalf("Failed to execute query: %v, (%s)", err, sql)
	}

	var o = obj.Object.(*TestStruct)
	if o.ID != test.ID {
		t.Errorf("Expected ID to be %d, got %d", test.ID, o.ID)
	}

	if o.Name != test.Name {
		t.Errorf("Expected Name to be %q, got %q", test.Name, o.Name)
	}

	if o.Text != test.Text {
		t.Errorf("Expected Text to be %q, got %q", test.Text, o.Text)
	}

	var textV, _ = o.Model.DataStore().GetValue("TestNameText")
	if textV != "test1 test2 test" && obj.Annotations["TestNameText"] != "test1 test2 test" {
		t.Errorf("Expected TestNameText to be 'test1 test2', got %v", textV)
	}

	var lowerV, _ = o.Model.DataStore().GetValue("TestNameLower")
	if lowerV != "test1" && obj.Annotations["TestNameLower"] != "test1" {
		t.Errorf("Expected TestNameLower to be 'test1', got %v", lowerV)
	}

	var upperV, _ = o.Model.DataStore().GetValue("TestNameUpper")
	if upperV != "TEST1" && obj.Annotations["TestNameUpper"] != "TEST1" {
		t.Errorf("Expected TestNameUpper to be 'TEST1', got %v", upperV)
	}

	t.Logf("SQL: %s", sql)
	t.Logf("Args: %v", args)
	t.Logf("Object: %+v", obj)

	if _, err = queries.DeleteObject(test); err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}
}

func TestVirtualFieldsQuerySetSingleObjectTestStructNoObject(t *testing.T) {
	var test = &TestStructNoObject{
		Name: "test1",
		Text: "test2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v, %T", err, err)
	}

	var qs = queries.Objects[attrs.Definer](test)
	qs = qs.Select("*")
	qs = qs.Filter("ID", test.ID)
	qs = qs.Filter("TestNameLower", "test1")
	qs = qs.Filter("TestNameUpper", "TEST1")
	qs = qs.OrderBy("-TestNameText")

	var obj, err = qs.Get()
	var (
		sql  = qs.LatestQuery().SQL()
		args = qs.LatestQuery().Args()
	)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	var o = obj.Object.(*TestStructNoObject)
	if o.ID != test.ID {
		t.Errorf("Expected ID to be %d, got %d", test.ID, o.ID)
	}

	if o.Name != test.Name {
		t.Errorf("Expected Name to be %q, got %q", test.Name, o.Name)
	}

	if o.Text != test.Text {
		t.Errorf("Expected Text to be %q, got %q", test.Text, o.Text)
	}

	var textV = o.TestNameText
	if textV != "test1 test2 test" && obj.Annotations["TestNameText"] != "test1 test2 test" {
		t.Errorf("Expected TestNameText to be 'test1 test2', got %v", textV)
	}

	var lowerV = o.TestNameLower
	if lowerV != "test1" && obj.Annotations["TestNameLower"] != "test1" {
		t.Errorf("Expected TestNameLower to be 'test1', got %v", lowerV)
	}

	var upperV = o.TestNameUpper
	if upperV != "TEST1" && obj.Annotations["TestNameUpper"] != "TEST1" {
		t.Errorf("Expected TestNameUpper to be 'TEST1', got %v", upperV)
	}

	t.Logf("SQL: %s", sql)
	t.Logf("Args: %v", args)
	t.Logf("Object: %+v", obj)

	if _, err = queries.DeleteObject(test); err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}
}

func Test_Annotate_With_GroupBy(t *testing.T) {
	// Setup test data
	for i := 0; i < 3; i++ {
		err := queries.CreateObject(&TestStruct{
			Name: "GroupA",
			Text: "T" + string(rune('0'+i)),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Run query
	var qs = queries.Objects[attrs.Definer](&TestStruct{}).
		Select("Name").
		GroupBy("Name").
		Annotate("TextCount", expr.Raw("COUNT(![Text])"))
	var rows, err = qs.All()

	t.Logf("SQL: %s %v", qs.LatestQuery().SQL(), qs.LatestQuery().Args())

	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	count, ok := row.Annotations["TextCount"]
	if !ok {
		t.Fatalf("TextCount annotation not found")
	}
	if count != int64(3) {
		t.Errorf("expected count to be 3, got %v", count)
	}
}

func Test_Annotate_Only(t *testing.T) {
	// Query only virtual field, not full model
	var rows, err = queries.Objects[attrs.Definer](&TestStruct{}).
		Annotate("UpperName", expr.Raw("UPPER(![Name])")).
		Limit(1).
		All()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one result")
	}

	v := rows[0].Annotations["UpperName"]
	if v == nil {
		t.Errorf("expected annotation 'UpperName', got nil")
	}
}

func Test_Annotated_Filter(t *testing.T) {
	// Create test data
	test := &TestStruct{
		Name: "TEST1",
		Text: "TEST2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	qs := queries.Objects[attrs.Definer](&TestStruct{}).
		Select("*").
		Filter("Name", "TEST1").
		Filter("LowerName", "test1").
		Annotate("LowerName", expr.FuncLower("Name"))
	rows, err := qs.All()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) == 0 {
		t.Fatal("expected at least one result")
	}

	var obj = rows[0].Object.(*TestStruct)

	if obj.ID != test.ID {
		t.Errorf("expected ID = %d, got %d", test.ID, obj.ID)
	}

	if obj.Name != "TEST1" {
		t.Errorf("expected Name = 'TEST1', got %q (%d)", obj.Name, len(obj.Name))
	}

	if obj.Text != "TEST2" {
		t.Errorf("expected Text = 'TEST2', got %q (%d)", obj.Text, len(obj.Text))
	}

	if obj.Annotations["LowerName"] != "test1" {
		t.Errorf("expected LowerName = 'test1', got %v", obj.Annotations["LowerName"])
	}

	if _, err := queries.DeleteObject(test); err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}
}

func Test_Annotated_Get(t *testing.T) {
	// Create test data
	test := &TestStruct{
		Name: "test1",
		Text: "test2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	qs := queries.Objects[attrs.Definer](&TestStruct{}).
		Select("*").
		Filter("Name", "test1").
		Annotate("LowerName", &expr.RawExpr{
			Statement: "LOWER(%s)",
			Fields:    []string{"Name"},
		}).
		Annotate("UpperName", &expr.RawExpr{
			Statement: "UPPER(%s)",
			Fields:    []string{"Name"},
		}).
		Annotate("CustomAnnotation", &expr.RawExpr{
			Statement: "UPPER(%s) || ' ' || %s",
			Fields:    []string{"Name", "Text"},
		})
	row, err := qs.Get()
	if err != nil {
		t.Fatal(err)
	}

	if row.Annotations["LowerName"] != "test1" {
		t.Errorf("expected LowerName = 'test1', got %v", row.Annotations["LowerName"])
	}

	if row.Annotations["UpperName"] != "TEST1" {
		t.Errorf("expected UpperName = 'TEST1', got %v", row.Annotations["UpperName"])
	}

	if row.Annotations["CustomAnnotation"] != "TEST1 test2" {
		t.Errorf("expected CustomAnnotation = 'TEST1 test2', got %v", row.Annotations["CustomAnnotation"])
	}

	var obj = row.Object.(*TestStruct)

	if obj.ID != test.ID {
		t.Errorf("expected ID = %d, got %d", test.ID, obj.ID)
	}

	var (
		lowerNameV, _ = obj.Annotations["LowerName"]
		upperNameV, _ = obj.Annotations["UpperName"]
		customV, _    = obj.Annotations["CustomAnnotation"]
	)

	if lowerNameV != "test1" {
		t.Errorf("expected LowerName = 'test1', got %v", lowerNameV)
	}

	if upperNameV != "TEST1" {
		t.Errorf("expected UpperName = 'TEST1', got %v", upperNameV)
	}

	if customV != "TEST1 test2" {
		t.Errorf("expected CustomAnnotation = 'TEST1 test2', got %v", customV)
	}

	if obj.Name != "test1" {
		t.Errorf("expected Name = 'test1', got %q (%d)", row.Object.(*TestStruct).Name, len(row.Object.(*TestStruct).Name))
	}

	if obj.Text != "test2" {
		t.Errorf("expected Text = 'test2', got %q (%d)", row.Object.(*TestStruct).Text, len(row.Object.(*TestStruct).Text))
	}

	if _, err := queries.DeleteObject(test); err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}
}

func Test_Annotated_OrderBy(t *testing.T) {
	// Create test data
	test1 := &TestStruct{
		Name: "test1",
		Text: "Test_Annotated_OrderBy",
	}
	test2 := &TestStruct{
		Name: "test2",
		Text: "Test_Annotated_OrderBy",
	}

	if err := queries.CreateObject(test1); err != nil {
		t.Fatalf("Failed to create object 1: %v", err)
	}
	if err := queries.CreateObject(test2); err != nil {
		t.Fatalf("Failed to create object 2: %v", err)
	}

	qs := queries.Objects[attrs.Definer](&TestStruct{}).
		Select("*").
		Filter("Text", "Test_Annotated_OrderBy").
		Annotate("UpperName", expr.FuncUpper("Name")).
		OrderBy("-UpperName")

	rows, err := qs.All()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].Annotations["UpperName"] != "TEST2" || rows[1].Annotations["UpperName"] != "TEST1" {
		t.Errorf("expected UpperName annotations to be 'TEST2' and 'TEST1', got %v and %v",
			rows[0].Annotations["UpperName"], rows[1].Annotations["UpperName"])
	}

	if _, err := queries.DeleteObject(test1); err != nil {
		t.Fatalf("Failed to delete object 1: %v", err)
	}
	if _, err := queries.DeleteObject(test2); err != nil {
		t.Fatalf("Failed to delete object 2: %v", err)
	}
}

func Test_Annotated_ValuesList(t *testing.T) {
	qs := queries.Objects[attrs.Definer](&TestStruct{}).
		Annotate("Combined", &expr.RawExpr{
			Statement: "%s || ' ' || %s",
			Fields:    []string{"Name", "Text"},
		}).
		Select("ID", "Name")
	values, err := qs.ValuesList("ID", "Combined")
	if err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		t.Fatal("expected at least one result")
	}
	if len(values[0]) != 2 {
		t.Errorf("expected 2 fields per row, got %d", len(values[0]))
	}
}

func Test_Aggregate(t *testing.T) {
	// Create multiple entries
	for range 5 {
		err := queries.CreateObject(&TestStruct{
			Name: "agg",
			Text: "txt",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	result, err := queries.Objects[attrs.Definer](&TestStruct{}).
		Filter("Name", "agg").
		Aggregate(map[string]expr.Expression{
			"Total": &expr.RawExpr{
				Statement: "COUNT(*)",
			},
		})
	if err != nil {
		t.Fatal(err)
	}

	if result["Total"] != int64(5) {
		t.Errorf("expected count to be 5, got %v", result["Total"])
	}
}

func Test_MultiAggregate(t *testing.T) {
	for i := 0; i < 4; i++ {
		err := queries.CreateObject(&TestStruct{
			Name: "multiagg",
			Text: string(rune('A' + i)),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	res, err := queries.Objects[attrs.Definer](&TestStruct{}).
		Filter("Name", "multiagg").
		Aggregate(map[string]expr.Expression{
			"Total": &expr.RawExpr{Statement: "COUNT(*)"},
			"MinID": &expr.RawExpr{Statement: "MIN(id)"},
			"MaxID": &expr.RawExpr{Statement: "MAX(id)"},
		})
	if err != nil {
		t.Fatal(err)
	}

	if res["Total"] != int64(4) {
		t.Errorf("expected Total = 4, got %v", res["Total"])
	}
	if res["MinID"] == nil || res["MaxID"] == nil {
		t.Errorf("expected MinID and MaxID, got: %v", res)
	}
}

type Author struct {
	ID   int64
	Name string
}

func (a *Author) FieldDefs() attrs.Definitions {
	return attrs.Define(a,
		attrs.NewField(a, "ID", &attrs.FieldConfig{
			Primary: true,
		}),
		attrs.NewField(a, "Name", nil),
	).WithTableName("author")
}

type Book struct {
	ID     int64
	Title  string
	Author *Author
}

func (b *Book) FieldDefs() attrs.Definitions {
	return attrs.Define(b,
		attrs.NewField(b, "ID", &attrs.FieldConfig{
			Primary: true,
		}),
		attrs.NewField(b, "Title", nil),
		attrs.NewField(b, "Author", &attrs.FieldConfig{
			Column:        "author_id",
			RelForeignKey: attrs.Relate(&Author{}, "", nil),
		}),
	).WithTableName("book")
}

func Test_Annotate_With_Relation(t *testing.T) {
	author := &Author{Name: "Tolkien"}
	if err := queries.CreateObject(author); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		book := &Book{
			Title:  "Book " + string(rune('A'+i)),
			Author: author,
		}
		if err := queries.CreateObject(book); err != nil {
			t.Fatal(err)
		}
	}

	qs := queries.Objects[attrs.Definer](&Book{}).
		Select("Author.Name").
		GroupBy("Author.Name").
		Annotate("BookCount", &expr.RawExpr{
			Statement: "COUNT(%s)",
			Fields:    []string{"ID"},
		})

	var rows, err = qs.All()
	if err != nil {
		t.Fatalf("failed to execute query: %v (%s)", err, qs.LatestQuery().SQL())
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 grouped row, got %d", len(rows))
	}

	if rows[0].Annotations["BookCount"] != int64(3) {
		t.Errorf("expected BookCount = 3, got %v", rows[0].Annotations["BookCount"])
	}

	if _, err := queries.DeleteObject(author); err != nil {
		t.Fatalf("failed to delete author: %v", err)
	}

	if _, err := queries.Objects[attrs.Definer](&Book{}).Delete(); err != nil {
		t.Fatalf("failed to delete books: %v", err)
	}
}

func Test_Annotate_Relation(t *testing.T) {
	author := &Author{Name: "Tolkien"}
	if err := queries.CreateObject(author); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 9; i++ {
		book := &Book{
			Title:  "Book " + string(rune('A'+(i%3))),
			Author: author,
		}
		if err := queries.CreateObject(book); err != nil {
			t.Fatal(err)
		}
	}

	qs := queries.Objects[attrs.Definer](&Book{}).
		Select("Title", "Author.*").
		GroupBy("Title").
		Annotate("AuthorCount", &expr.RawExpr{
			Statement: "COUNT(%s)",
			Fields:    []string{"Author.Name"},
		})

	var rows, err = qs.All()
	if err != nil {
		t.Fatalf("failed to execute query: %v (%s)", err, qs.LatestQuery().SQL())
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 grouped rows, got %d", len(rows))
	}

	for _, row := range rows {
		if row.Annotations["AuthorCount"] != int64(3) {
			t.Errorf("expected AuthorCount = 3, got %v", row.Annotations["AuthorCount"])
		}
	}

	if _, err := queries.DeleteObject(author); err != nil {
		t.Fatalf("failed to delete author: %v", err)
	}

	if _, err := queries.Objects[attrs.Definer](&Book{}).Delete(); err != nil {
		t.Fatalf("failed to delete books: %v", err)
	}
}

func Test_Aggregate_With_Join(t *testing.T) {
	author := &Author{Name: "Rowling"}
	if err := queries.CreateObject(author); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		book := &Book{
			Title:  "HP " + string(rune('1'+i)),
			Author: author,
		}
		if err := queries.CreateObject(book); err != nil {
			t.Fatal(err)
		}
	}

	var qs = queries.Objects[attrs.Definer](&Book{}).
		Select("*", "Author.*").
		Filter("Author.Name", "Rowling")

	var res, err = qs.Aggregate(map[string]expr.Expression{
		"Author": &expr.RawExpr{
			Statement: "%s",
			Fields:    []string{"Author.Name"},
		},
		"CountBooks": &expr.RawExpr{Statement: "COUNT(*)"},
	})

	if err != nil {
		t.Fatalf("failed to execute query: %v (%s)", err, qs.LatestQuery().SQL())
	}

	if res["Author"] != "Rowling" {
		t.Errorf("expected Author = 'Rowling', got %v", res["Author"])
	}

	if res["CountBooks"] != int64(2) {
		t.Errorf("expected CountBooks = 2, got %v", res["CountBooks"])
	}

	if _, err := queries.DeleteObject(author); err != nil {
		t.Fatalf("failed to delete author: %v", err)
	}

	if _, err := queries.Objects[attrs.Definer](&Book{}).Delete(); err != nil {
		t.Fatalf("failed to delete books: %v", err)
	}
}

func TestAnnotatedValuesListWithSelectExpressions(t *testing.T) {
	var test = &TestStruct{
		Name: "TestAnnotatedValuesListWithSelectExpressions1",
		Text: "TestAnnotatedValuesListWithSelectExpressions2",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	var qs = queries.Objects[attrs.Definer](test).
		Filter("ID", test.ID).
		Annotate("Combined", expr.Raw("![Name] || ' ' || ![Text]"))

	var rows, err = qs.ValuesList(
		"ID",
		"Combined",
		expr.F("LOWER(![Text]) || ' ' || ?", "testSelectExpressions"),
	)
	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	if len(rows) == 0 {
		t.Fatal("expected at least one result")
	}

	if len(rows[0]) != 3 {
		t.Errorf("expected 3 fields per row, got %d", len(rows[0]))
	}

	if rows[0][0] != test.ID {
		t.Errorf("expected ID = %d, got %v", test.ID, rows[0][0])
	}

	if rows[0][1] != "TestAnnotatedValuesListWithSelectExpressions1 TestAnnotatedValuesListWithSelectExpressions2" {
		t.Errorf("expected Combined = 'TestAnnotatedValuesListWithSelectExpressions1 TestAnnotatedValuesListWithSelectExpressions2', got %v", rows[0][1])
	}

	if rows[0][2] != "testannotatedvalueslistwithselectexpressions2 testSelectExpressions" {
		t.Errorf("expected Text = 'testannotatedvalueslistwithselectexpressions2 testSelectExpressions', got %v", rows[0][2])
	}
}

func TestWhereFilterVirtualFieldAliassed(t *testing.T) {
	var test = &TestStruct{
		Name: "TestWhereFilterVirtualFieldAliassed",
		Text: "TestWhereFilterVirtualFieldAliassed",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	var qs = queries.Objects[attrs.Definer](test).
		Select("*").
		Filter(expr.F("UPPER(![TestNameText]) = ?", "TESTWHEREFILTERVIRTUALFIELDALIASSED TESTWHEREFILTERVIRTUALFIELDALIASSED TEST"))
	var rows, err = qs.All()

	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	if len(rows) == 0 {
		t.Fatal("expected at least one result")
	}

	if rows[0].Object.(*TestStruct).ID != test.ID {
		t.Errorf("expected ID = %d, got %d", test.ID, rows[0].Object.(*TestStruct).ID)
	}

	if rows[0].Object.(*TestStruct).Name != test.Name {
		t.Errorf("expected Name = %q, got %q", test.Name, rows[0].Object.(*TestStruct).Name)
	}

	if rows[0].Object.(*TestStruct).Text != test.Text {
		t.Errorf("expected Text = %q, got %q", test.Text, rows[0].Object.(*TestStruct).Text)
	}

	if rows[0].Annotations["TestNameText"] != "TestWhereFilterVirtualFieldAliassed TestWhereFilterVirtualFieldAliassed test" {
		t.Errorf("expected TestNameText = 'TestWhereFilterVirtualFieldAliassed TestWhereFilterVirtualFieldAliassed test', got %v", rows[0].Annotations["TestNameText"])
	}
}

func TestSubquery(t *testing.T) {
	var test = &TestStruct{
		Name: "TestSubquery",
		Text: "TestSubquery",
	}

	if err := queries.CreateObject(test); err != nil {
		t.Fatalf("Failed to create object: %v", err)
		return
	}

	var qs = queries.
		Objects[attrs.Definer](test).
		Select(expr.FuncLower("Name")).
		Filter("ID", test.ID)

	var rows, err = queries.Objects[attrs.Definer](&TestStruct{}).
		Select("*").
		Filter("TestNameUpper__lower__in", queries.Subquery(qs)).
		All()

	if err != nil {
		t.Fatalf("Failed to execute query: %v", err)
	}

	if len(rows) == 0 {
		t.Fatal("expected at least one result")
		return
	}

	if rows[0].Object.(*TestStruct).ID != test.ID {
		t.Errorf("expected ID = %d, got %d", test.ID, rows[0].Object.(*TestStruct).ID)
		return
	}

	if rows[0].Object.(*TestStruct).Name != test.Name {
		t.Errorf("expected Name = %q, got %q", test.Name, rows[0].Object.(*TestStruct).Name)
		return
	}

	if rows[0].Object.(*TestStruct).Text != test.Text {
		t.Errorf("expected Text = %q, got %q", test.Text, rows[0].Object.(*TestStruct).Text)
		return
	}

	t.Logf("Row: %#v", rows[0].Object.(*TestStruct))
}
