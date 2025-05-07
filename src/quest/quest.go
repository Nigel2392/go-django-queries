package quest

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	queries "github.com/Nigel2392/go-django-queries/src"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type Test[Type T] interface {
	// Name returns the name of the test.
	// This is used to identify the test in the output.
	Name() string

	// Run runs the test and returns an error if it fails.
	// The test should use the provided Quest and TestCase to run the test.
	Run(q *Quest[Type]) error
}

type BaseTest[Type T] struct {
	TestName      string
	GetQuerySet   func(q *Quest[Type], t Test[Type]) *queries.QuerySet
	Setup         func(q *Quest[Type], t Test[Type], querySet *queries.QuerySet) error
	Expect        func(q *Quest[Type], t Test[Type], querySet *queries.QuerySet) error
	ExpectedError error

	querySet *queries.QuerySet
	q        *Quest[Type]
}

func (t *BaseTest[Type]) Name() string {
	return t.TestName
}

func (t *BaseTest[Type]) Run(q *Quest[Type]) error {
	t.q = q
	t.q.T.Helper()

	if t.GetQuerySet != nil {
		t.querySet = t.GetQuerySet(t.q, t)
	}

	if t.querySet == nil {
		return fmt.Errorf("no query set provided")
	}

	if t.Setup != nil {
		if err := t.Setup(t.q, t, t.querySet); err != nil {
			return err
		}
	}

	if t.Expect != nil {
		if err := t.Expect(q, t, t.querySet); err != nil {
			if t.ExpectedError != nil && errors.Is(err, t.ExpectedError) {
				return nil
			}
			return err
		}
	}

	return nil
}

type CreateTestConfig[TestT T, ModelT attrs.Definer] struct {
	ExpectedID     any
	ExpectedValues map[string]any
	ObjectValue    ModelT
	Execute        func(q *Quest[TestT], t Test[TestT], querySet *queries.QuerySet, obj ModelT) error
}

func NewCreateTest[TestT T, ModelT attrs.Definer](name string, config CreateTestConfig[TestT, ModelT]) *BaseTest[TestT] {
	var objT = reflect.TypeOf((*ModelT)(nil)).Elem()
	if objT.Kind() != reflect.Ptr {
		panic(fmt.Errorf("object type %T is not a pointer", objT))
	}
	var obj = reflect.New(objT.Elem()).Interface().(ModelT)
	return &BaseTest[TestT]{
		TestName: name,
		GetQuerySet: func(q *Quest[TestT], t Test[TestT]) *queries.QuerySet {
			return queries.Objects(obj)
		},
		Expect: func(q *Quest[TestT], t Test[TestT], querySet *queries.QuerySet) error {
			q.T.Helper()

			var obj = config.ObjectValue
			var newObj, err = querySet.Create(obj).Exec()
			if err != nil {
				return err
			}

			if config.Execute != nil {
				if err := config.Execute(q, t, querySet, obj); err != nil {
					return err
				}
			}

			var defs = newObj.FieldDefs()
			if config.ExpectedID != nil {
				var primary = defs.Primary()
				var id, err = primary.Value()
				if err != nil {
					return err
				}

				if id == nil {
					return fmt.Errorf("expected ID %v, got nil", config.ExpectedID)
				}

				var (
					idT  = reflect.TypeOf(id)
					idV  = reflect.ValueOf(id)
					expT = reflect.TypeOf(config.ExpectedID)
					expV = reflect.ValueOf(config.ExpectedID)
				)

				if idT != expT && !idT.ConvertibleTo(expT) {
					return fmt.Errorf("expected ID %v of type %T, got %v of type %T", config.ExpectedID, config.ExpectedID, id, id)
				}

				if idT != expT && idT.ConvertibleTo(expT) {
					var convertedID = idV.Convert(expV.Type()).Interface()
					if !reflect.DeepEqual(config.ExpectedID, convertedID) {
						return fmt.Errorf(
							"expected ID %v (%T), got %v (%T)",
							config.ExpectedID, config.ExpectedID, convertedID, convertedID,
						)
					}
				}
			}

			for name, expectedValue := range config.ExpectedValues {
				var field, ok = defs.Field(name)
				if !ok {
					return fmt.Errorf("field %q not found in model %T", name, obj)
				}

				var value, err = field.Value()
				if err != nil {
					return err
				}

				var (
					expectedValueT = reflect.TypeOf(expectedValue)
					expectedValueV = reflect.ValueOf(expectedValue)
					valueT         = reflect.TypeOf(value)
					valueV         = reflect.ValueOf(value)
				)

				if expectedValueT != valueT && !valueT.ConvertibleTo(expectedValueT) {
					return fmt.Errorf("expected value %v of type %T, got %v of type %T", expectedValue, expectedValue, value, value)
				}

				if expectedValueT != valueT && valueT.ConvertibleTo(expectedValueT) {
					var convertedValue = valueV.Convert(expectedValueV.Type()).Interface()
					if !reflect.DeepEqual(expectedValue, convertedValue) {
						return fmt.Errorf("expected value %v, got %v", expectedValue, convertedValue)
					}
				}

				if !reflect.DeepEqual(expectedValue, value) {
					return fmt.Errorf("expected value %v, got %v", expectedValue, value)
				}

				continue
			}

			return nil
		},
	}
}

type DB struct {
	Connect          func() (*sql.DB, error)
	DriverName       string
	ConnectionString string

	db *sql.DB
}

func (db *DB) Open() (*sql.DB, error) {
	if db.db != nil {
		return db.db, nil
	}
	var err error
	if db.Connect != nil {
		db.db, err = db.Connect()
	} else {
		db.db, err = sql.Open(
			db.DriverName,
			db.ConnectionString,
		)
	}
	return db.db, err
}

type T interface {
	Helper()
	Error(args ...any)
}

type Tester interface {
	T() T
}

type Quest[Type T] struct {
	T            Type
	DB           *DB
	CreateTables []string
	TestCases    []Test[Type]
}

func (q *Quest[T]) Run() error {
	q.T.Helper()

	if q.DB != nil {
		var db, err = q.DB.Open()
		if err != nil {
			return err
		}

		if django.Global == nil {
			var app = django.App(
				django.Configure(
					make(map[string]interface{}),
				),
				django.Flag(
					django.FlagSkipCmds,
					django.FlagSkipDepsCheck,
				),
			)
			if err := app.Initialize(); err != nil {
				return err
			}
		}

		django.Global.Settings.Set(
			django.APPVAR_DATABASE, db,
		)
	}

	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	if db == nil {
		return fmt.Errorf("no database connection found")
	}

	for _, table := range q.CreateTables {
		var _, err = db.Exec(table)
		if err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	for _, test := range q.TestCases {
		if err := test.Run(q); err != nil {
			return err
		}
	}

	return nil
}

func (q *Quest[T]) Assertf(b bool, format string, args ...any) (ok bool) {
	q.T.Helper()

	var message string
	if len(args) == 0 {
		message = format
	} else {
		message = fmt.Sprintf(format, args...)
	}

	return q.Assert(
		b, message,
	)
}

func (q *Quest[T]) Assert(b bool, args ...any) (ok bool) {
	if b {
		return true
	}

	if len(args) == 0 {
		args = []any{"assertion failed"}
	}

	q.T.Helper()
	q.T.Error(args...)
	return false
}

func (q *Quest[T]) AssertEqual(a, b any, format string, args ...any) (ok bool) {
	q.T.Helper()

	if a == nil && b == nil {
		return
	}

	if a == nil || b == nil {
		return q.Assertf(
			false,
			format,
			args...,
		)
	}

	return q.Assertf(
		reflect.DeepEqual(a, b),
		format, args...,
	)
}

func (q *Quest[T]) AssertNotEqual(a, b any, format string, args ...any) (ok bool) {
	q.T.Helper()

	return q.Assertf(
		!reflect.DeepEqual(a, b),
		format, args...,
	)
}

func (q *Quest[T]) AssertNil(a any, format string, args ...any) (ok bool) {
	q.T.Helper()

	return q.Assertf(
		a == nil,
		format,
		args...,
	)
}

func (q *Quest[T]) AssertNotNil(a any, format string, args ...any) (ok bool) {
	q.T.Helper()

	return q.Assertf(
		a != nil,
		format, args...,
	)
}

func (q *Quest[T]) AssertErr(err error, format string, args ...any) (ok bool) {
	q.T.Helper()

	return q.Assertf(
		err != nil,
		format, args...,
	)
}

func (q *Quest[T]) AssertErrNil(err error, format string, args ...any) (ok bool) {
	q.T.Helper()

	return q.Assertf(
		err == nil,
		format, args...,
	)
}

func (q *Quest[T]) AssertTypesEqual(a, b attrs.Definer, format string, args ...any) (ok bool) {
	q.T.Helper()

	var (
		aType = reflect.TypeOf(a)
		bType = reflect.TypeOf(b)
	)

	return q.AssertEqual(
		aType,
		bType,
		format,
		args...,
	)
}

func (q *Quest[T]) AssertModelsEqual(a, b attrs.Definer, excludeFromCmp ...string) (ok bool) {
	q.T.Helper()

	var (
		aDefs = a.FieldDefs()
		bDefs = b.FieldDefs()
	)

	ok = true

	ok = ok && q.AssertTypesEqual(
		a, b, "model types mismatch",
	)

	ok = ok && q.AssertEqual(
		aDefs.Len(),
		bDefs.Len(),
		"length of fields mismatch, %v != %v",
		aDefs.Len(), bDefs.Len(),
	)

	var exludes = make(map[string]struct{}, len(excludeFromCmp))
	for _, name := range excludeFromCmp {
		exludes[name] = struct{}{}
	}

	for _, aField := range aDefs.Fields() {

		var aName = aField.Name()
		if _, ok := exludes[aName]; ok {
			continue
		}

		var bField, hasField = bDefs.Field(aName)
		if !hasField {
			ok = ok && q.Assertf(
				false,
				"field %q not found in model %T",
				aName, b,
			)
			continue
		}

		var aValue = aField.GetValue()
		var bValue = bField.GetValue()
		if aValue == nil && bValue == nil {
			continue
		}

		if aValue == nil || bValue == nil {
			ok = ok && q.Assertf(
				false,
				"field %q value mismatch, %v != %v",
				aName, aValue, bValue,
			)
			continue
		}

		if reflect.TypeOf(aValue) != reflect.TypeOf(bValue) {
			ok = ok && q.Assertf(
				false,
				"field %q type mismatch, %T != %T",
				aName, aValue, bValue,
			)
			continue
		}

		if !reflect.DeepEqual(aValue, bValue) {
			ok = ok && q.Assertf(
				false,
				"field %q value mismatch, %v != %v",
				aName, aValue, bValue,
			)
			continue
		}
	}

	return ok
}
