package testsql

import (
	"context"
	"database/sql"

	"github.com/Nigel2392/go-django-queries/src/migrator"
)

type SQL struct {
	SQL    string
	Params []any
}

type Action struct {
	Type  migrator.ActionType
	Table migrator.Table
	Field migrator.Column
	Index migrator.Index
}

type TestMigrationEngine struct {
	SetupCalled      bool
	StoredMigrations map[string]map[string]map[string]struct{}
	RawSQL           []SQL
	Actions          []Action
}

func NewTestMigrationEngine() *TestMigrationEngine {
	return &TestMigrationEngine{
		SetupCalled:      false,
		RawSQL:           make([]SQL, 0),
		Actions:          make([]Action, 0),
		StoredMigrations: make(map[string]map[string]map[string]struct{}),
	}
}

func (t *TestMigrationEngine) Setup() error {
	t.SetupCalled = true
	return nil
}

func (t *TestMigrationEngine) StoreMigration(appName string, modelName string, migrationName string) error {
	if t.StoredMigrations == nil {
		t.StoredMigrations = make(map[string]map[string]map[string]struct{})
	}
	if _, ok := t.StoredMigrations[appName]; !ok {
		t.StoredMigrations[appName] = make(map[string]map[string]struct{})
	}
	if _, ok := t.StoredMigrations[appName][modelName]; !ok {
		t.StoredMigrations[appName][modelName] = make(map[string]struct{})
	}
	t.StoredMigrations[appName][modelName][migrationName] = struct{}{}
	return nil
}

func (t *TestMigrationEngine) HasMigration(appName string, modelName string, migrationName string) (bool, error) {
	if t.StoredMigrations == nil {
		return false, nil
	}
	if _, ok := t.StoredMigrations[appName]; !ok {
		return false, nil
	}
	if _, ok := t.StoredMigrations[appName][modelName]; !ok {
		return false, nil
	}
	if _, ok := t.StoredMigrations[appName][modelName][migrationName]; !ok {
		return false, nil
	}
	return true, nil
}

func (t *TestMigrationEngine) RemoveMigration(appName string, modelName string, migrationName string) error {
	if t.StoredMigrations == nil {
		return nil
	}
	if _, ok := t.StoredMigrations[appName]; !ok {
		return nil
	}
	if _, ok := t.StoredMigrations[appName][modelName]; !ok {
		return nil
	}
	if _, ok := t.StoredMigrations[appName][modelName][migrationName]; !ok {
		return nil
	}
	delete(t.StoredMigrations[appName][modelName], migrationName)
	if len(t.StoredMigrations[appName][modelName]) == 0 {
		delete(t.StoredMigrations[appName], modelName)
	}
	if len(t.StoredMigrations[appName]) == 0 {
		delete(t.StoredMigrations, appName)
	}
	return nil
}

func (t *TestMigrationEngine) Execute(ctx context.Context, query string, args ...any) (sql.Result, error) {
	t.RawSQL = append(t.RawSQL, SQL{SQL: query, Params: args})
	return nil, nil
}
func (t *TestMigrationEngine) CreateTable(table migrator.Table) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionCreateTable, Table: table})
	return nil
}
func (t *TestMigrationEngine) DropTable(table migrator.Table) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionDropTable, Table: table})
	return nil
}
func (t *TestMigrationEngine) RenameTable(table migrator.Table, newName string) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionRenameTable, Table: table})
	return nil
}
func (t *TestMigrationEngine) AddIndex(table migrator.Table, index migrator.Index) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionAddIndex, Table: table, Index: index})
	return nil
}
func (t *TestMigrationEngine) DropIndex(table migrator.Table, index migrator.Index) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionDropIndex, Table: table, Index: index})
	return nil
}
func (t *TestMigrationEngine) RenameIndex(table migrator.Table, oldName string, newName string) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionRenameIndex, Table: table})
	return nil
}

//	func (t *TestMigrationEngine) AlterUniqueTogether(table migrator.Table, unique bool) error {
//		t.Actions = append(t.Actions, Action{Type: migrator.ActionAlterUniqueTogether, Table: table})
//		return nil
//	}
//
//	func (t *TestMigrationEngine) AlterIndexTogether(table migrator.Table, unique bool) error {
//		t.Actions = append(t.Actions, Action{Type: migrator.ActionAlterIndexTogether, Table: table})
//		return nil
//	}
func (t *TestMigrationEngine) AddField(table migrator.Table, col migrator.Column) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionAddField, Table: table, Field: col})
	return nil
}
func (t *TestMigrationEngine) AlterField(table migrator.Table, old migrator.Column, newCol migrator.Column) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionAlterField, Table: table, Field: newCol})
	return nil
}
func (t *TestMigrationEngine) RemoveField(table migrator.Table, col migrator.Column) error {
	t.Actions = append(t.Actions, Action{Type: migrator.ActionRemoveField, Table: table, Field: col})
	return nil
}
