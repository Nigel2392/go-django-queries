package quest_test

import (
	"fmt"
	"testing"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/quest"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

var (
	createTableModelA = `CREATE TABLE IF NOT EXISTS "model_a" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"field1" TEXT NOT NULL,
	"field2" INTEGER NOT NULL,
	"field3" BOOLEAN NOT NULL,
	"field4" REAL NOT NULL
);`
	createTableModelB = `CREATE TABLE IF NOT EXISTS "model_b" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"field1" TEXT NOT NULL,
	"field2" INTEGER NOT NULL,
	"field3" BOOLEAN NOT NULL,
	"field4" REAL NOT NULL,
	"field5" TEXT NOT NULL,
	"field6" INTEGER NOT NULL
);`
	createTableModelC = `CREATE TABLE IF NOT EXISTS "model_c" (
	"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
	"field1" TEXT NOT NULL,
	"field2" INTEGER NOT NULL,
	"field3" BOOLEAN NOT NULL,
	"field4" REAL NOT NULL,
	"field5" TEXT NOT NULL,
	"field6" INTEGER NOT NULL,
	"field7" TEXT NOT NULL,
	"field8" INTEGER NOT NULL
);`
)

type ModelA struct {
	ID     int64 `attrs:"primary"`
	Field1 string
	Field2 int
	Field3 bool
	Field4 float64
}

func (m *ModelA) FieldDefs() attrs.Definitions {
	return attrs.AutoDefinitions(m)
}

type ModelB struct {
	ID     int64 `attrs:"primary"`
	Field1 string
	Field2 int
	Field3 bool
	Field4 float64
	Field5 string
	Field6 int
}

func (m *ModelB) FieldDefs() attrs.Definitions {
	return attrs.AutoDefinitions(m)
}

type ModelC struct {
	ID     int64 `attrs:"primary"`
	Field1 string
	Field2 int
	Field3 bool
	Field4 float64
	Field5 string
	Field6 int
	Field7 string
	Field8 int
}

func (m *ModelC) FieldDefs() attrs.Definitions {
	return attrs.AutoDefinitions(m)
}

func init() {
	var db, err = db.Open()
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(createTableModelA)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(createTableModelB)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(createTableModelC)
	if err != nil {
		panic(err)
	}
}

var (
	_  quest.T = (*fakeT)(nil)
	db         = &quest.DB{
		DriverName:       "sqlite3",
		ConnectionString: "file:quest_tests?mode=memory&cache=shared",
	}
)

type CallList struct {
	Calls   []any
	LastIdx int
}

func (c *CallList) Len() int {
	return len(c.Calls)
}

func (c *CallList) Add(args ...any) {
	c.Calls = append(c.Calls, args...)
	c.LastIdx = len(c.Calls) - 1
}

func (c *CallList) Last() any {
	return c.Calls[c.LastIdx]
}

type fakeT struct {
	LogCalled   CallList
	ErrorCalled CallList
}

func (t *fakeT) Helper() {}

func (t *fakeT) Log(args ...any) {
}
func (t *fakeT) Logf(format string, args ...any) {
}
func (t *fakeT) Error(args ...any) {
	t.ErrorCalled.Add(fmt.Sprintf(
		"%v", args...,
	))
}
func (t *fakeT) Errorf(format string, args ...any) {
	t.ErrorCalled.Add(fmt.Sprintf(
		format, args...,
	))
}
func (t *fakeT) Fatal(args ...any) {
}
func (t *fakeT) Fatalf(format string, args ...any) {
}

func (t *fakeT) Fail() {
}

func TestQuest(t *testing.T) {
	var (
		testT = &fakeT{}
		q     = &quest.Quest[*fakeT]{
			T: testT,
		}
	)

	var expectedCount = 0

	// Assert
	q.Assert(true, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected no error, got %d", testT.ErrorCalled.Len())
	}

	q.Assert(false, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertEqual
	q.AssertEqual(1, 1, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertEqual(1, 2, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertNotEqual
	q.AssertNotEqual(1, 1, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertNotEqual(1, 2, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertNil
	q.AssertNil(nil, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertNil(1, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertNotNil
	q.AssertNotNil(nil, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertNotNil(1, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertError
	q.AssertErr(nil, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertErr(fmt.Errorf("test"), "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertErrNil
	q.AssertErrNil(nil, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertErrNil(fmt.Errorf("test"), "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertTypesEqual
	q.AssertTypesEqual(&ModelA{}, &ModelA{}, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertTypesEqual(&ModelA{}, &ModelB{}, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	// AssertModelsEqual
	q.AssertModelsEqual(&ModelA{}, &ModelA{}, "test")
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertModelsEqual(&ModelA{}, &ModelB{}, "test")
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertModelsEqual(
		&ModelA{
			Field1: "test",
			Field2: 1,
			Field3: true,
			Field4: 1.0,
		},
		&ModelA{
			Field1: "test",
			Field2: 1,
			Field3: true,
			Field4: 1.0,
		},
	)
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}

	q.AssertModelsEqual(
		&ModelA{
			Field1: "test",
			Field2: 1,
			Field3: true,
			Field4: 1.0,
		},
		&ModelA{
			Field1: "test2",
			Field2: 1,
			Field3: true,
			Field4: 1.0,
		},
	)
	expectedCount++
	if testT.ErrorCalled.Len() != expectedCount {
		t.Errorf("expected %d error, got %d", expectedCount, testT.ErrorCalled.Len())
	}
}

type testCase struct {
	test         quest.Test[*fakeT]
	expectedErrs int
}

var (
	testCreateObjectModelA_OK = testCase{
		expectedErrs: 0,
		test: quest.NewCreateTest("testCreateObjectModelA_OK", quest.CreateTestConfig[*fakeT, *ModelA]{
			ExpectedID: 1,
			ExpectedValues: map[string]any{
				"Field1": "test",
				"Field2": 1,
				"Field3": true,
				"Field4": 1.0,
			},
			ObjectValue: &ModelA{
				Field1: "test",
				Field2: 1,
				Field3: true,
				Field4: 1.0,
			},
		}),
	}
	testCreateObjectModelA_FAIL_ID = testCase{
		expectedErrs: 1,
		test: quest.NewCreateTest("testCreateObjectModelA_FAIL_ID", quest.CreateTestConfig[*fakeT, *ModelA]{
			ExpectedID: 3,
			ExpectedValues: map[string]any{
				"Field1": "test",
				"Field2": 1,
				"Field3": true,
				"Field4": 1.0,
			},
			ObjectValue: &ModelA{
				Field1: "test",
				Field2: 1,
				Field3: true,
				Field4: 1.0,
			},
		}),
	}
	testCreateObjectModelA_FAIL_VALUES = testCase{
		expectedErrs: 1,
		test: quest.NewCreateTest("testCreateObjectModelA_FAIL_VALUES", quest.CreateTestConfig[*fakeT, *ModelA]{
			ExpectedID: 3,
			ExpectedValues: map[string]any{
				"Field1": "test",
				"Field2": 1,
				"Field3": true,
				"Field4": 1.0,
			},
			ObjectValue: &ModelA{
				Field1: "test",
				Field2: 1,
				Field3: false,
				Field4: 1.0,
			},
		}),
	}
	testListObjects = testCase{
		expectedErrs: 0,
		test: &quest.BaseTest[*fakeT]{
			TestName: "testListObjects",
			GetQuerySet: func(q *quest.Quest[*fakeT], t quest.Test[*fakeT]) *queries.QuerySet {
				return queries.Objects(&ModelA{}).
					Select("*").
					Filter("Field1__istartswith", "testListObjects")
			},
			Setup: func(q *quest.Quest[*fakeT], t quest.Test[*fakeT], querySet *queries.QuerySet) error {
				var objects = []*ModelA{
					{Field1: "testListObjects 1", Field2: 1, Field3: true, Field4: 1.0},
					{Field1: "testListObjects 2", Field2: 1, Field3: true, Field4: 1.0},
					{Field1: "testListObjects 3", Field2: 1, Field3: true, Field4: 1.0},
				}

				for _, obj := range objects {
					if _, err := querySet.Create(obj).Exec(); err != nil {
						return err
					}
				}

				return nil
			},
			Expect: func(q *quest.Quest[*fakeT], t quest.Test[*fakeT], querySet *queries.QuerySet) error {
				var rows, err = querySet.All().Exec()
				if err != nil {
					return err
				}

				if rows == nil {
					return fmt.Errorf("expected rows, got nil")
				}

				if len(rows) != 3 {
					return fmt.Errorf("expected 3 rows, got %d", len(rows))
				}

				for i, row := range rows {
					var obj = row.Object.(*ModelA)
					if obj == nil {
						return fmt.Errorf("expected object, got nil")
					}

					if obj.Field1 != fmt.Sprintf("testListObjects %d", i+1) {
						return fmt.Errorf("expected Field1 %q, got %q", "test", obj.Field1)
					}

					if obj.Field2 != 1 {
						return fmt.Errorf("expected Field2 %d, got %d", 1, obj.Field2)
					}

					if obj.Field3 != true {
						return fmt.Errorf("expected Field3 %t, got %t", true, obj.Field3)
					}

					if obj.Field4 != 1.0 {
						return fmt.Errorf("expected Field4 %f, got %f", 1.0, obj.Field4)
					}
				}

				return nil
			},
		},
	}

	casesCreate = []testCase{
		testCreateObjectModelA_OK,
		testCreateObjectModelA_FAIL_ID,
		testCreateObjectModelA_FAIL_VALUES,
		testListObjects,
	}
)

func newQuestTest(tests ...quest.Test[*fakeT]) *quest.Quest[*fakeT] {
	return &quest.Quest[*fakeT]{
		T:         &fakeT{},
		DB:        db,
		TestCases: tests,
		CreateTables: []string{
			createTableModelA,
			createTableModelB,
			createTableModelC,
		},
	}
}

func TestQuestQueriesCreate(t *testing.T) {

	for _, test := range casesCreate {
		t.Run(test.test.Name(), func(t *testing.T) {
			var (
				q   = newQuestTest(test.test)
				err = q.Run()
			)
			if test.expectedErrs > 0 {
				if err == nil {
					t.Errorf("expected an error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
