package quest

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/Nigel2392/go-django-queries/src/migrator"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type DBTables struct {
	tables []*migrator.ModelTable
	schema migrator.SchemaEditor
	t      *testing.T
}

func Table(t *testing.T, model ...attrs.Definer) *DBTables {
	if len(model) == 0 {
		panic("No model provided to Table()")
	}

	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)

	var table = &DBTables{}
	var schemaEditor, err = migrator.GetSchemaEditor(db.Driver())
	if err != nil {
		table.fatalf("Failed setup SchemaEditor: %v", err)
		return nil
	}

	table.tables = make([]*migrator.ModelTable, len(model))
	for i, m := range model {
		attrs.RegisterModel(m)
		table.tables[i] = migrator.NewModelTable(m)
	}
	table.schema = schemaEditor
	table.t = t
	return table
}

func (t *DBTables) fatal(args ...interface{}) {
	if t.t == nil {
		panic(fmt.Sprint(args...))
	}
	t.t.Fatal(args...)
}

func (t *DBTables) fatalf(format string, args ...interface{}) {
	if t.t == nil {
		panic(fmt.Sprintf(format, args...))
	}
	t.t.Fatalf(format, args...)
}

func (t *DBTables) Create() {
	if t.schema == nil {
		t.fatal("SchemaEditor is not initialized")
		return
	}

	for _, table := range t.tables {

		if t.t != nil {
			t.t.Logf("Creating table: %s", table.TableName())
		} else {
			fmt.Printf("Creating table: %s\n", table.TableName())
		}

		err := t.schema.CreateTable(table)
		if err != nil {
			t.fatalf("Failed to create table (%s): %v", table.ModelName(), err)
			return
		}

	}
	return
}

func (t *DBTables) Drop() {
	if t.schema == nil {
		t.fatal("SchemaEditor is not initialized")
	}

	for _, table := range t.tables {
		err := t.schema.DropTable(table)
		if err != nil {
			t.fatalf("Failed to drop table (%s): %v", table.ModelName(), err)
		}
	}
	return
}
