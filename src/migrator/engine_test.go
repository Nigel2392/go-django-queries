package migrator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nigel2392/go-django-queries/src/migrator"
	testsql "github.com/Nigel2392/go-django-queries/src/migrator/sql/test_sql"
)

func init() {
	migrator.Register(&testsql.User{})
	migrator.Register(&testsql.Todo{})
	migrator.Register(&testsql.Profile{})
}

func TestMigrator(t *testing.T) {

	var (
		// db, _ = sql.Open("sqlite3", "file:./migrator_test.db")
		// tmpDir = t.TempDir()
		tmpDir = "./migrations"
		engine = migrator.NewMigrationEngine(tmpDir)
		editor = testsql.NewTestMigrationEngine()
		// editor = sqlite.NewSQLiteSchemaEditor(db)
	)
	engine.SchemaEditor = editor

	os.RemoveAll(tmpDir)

	// MakeMigrations
	if err := engine.MakeMigrations(); err != nil {
		t.Fatalf("MakeMigrations failed: %v", err)
	}

	t.Logf("Migrations created in %q", tmpDir)

	// Ensure migration files exist
	var files = 0
	var err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == migrator.MIGRATION_FILE_SUFFIX {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if files == 0 {
		t.Fatalf("expected migration files, got none")
	}

	// Migrate
	if err := engine.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify stored migrations
	if len(engine.Migrations["test_sql"]) == 0 {
		t.Fatalf("expected engine to track stored migrations for app 'test_sql' %v", engine.Migrations)
	}

	for model, migs := range engine.Migrations["test_sql"] {
		if len(migs) == 0 {
			t.Errorf("expected at least one migration stored for model %q", model)
		}
	}

	if len(editor.Actions) == 0 {
		t.Fatalf("expected actions, got none")
	}

	// Verify actions were logged (at least CreateTable)
	found := false
	for _, a := range editor.Actions {
		if a.Type == migrator.ActionCreateTable {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CreateTable action")
	}

	testsql.ExtendedDefinitions = true

	needsToMigrate, err := engine.NeedsToMigrate()
	if err != nil {
		t.Fatalf("NeedsToMigrate failed: %v", err)
	}

	if len(needsToMigrate) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(needsToMigrate))
	}

	if err := engine.MakeMigrations(); err != nil {
		t.Fatalf("MakeMigrations failed: %v", err)
	}

	t.Logf("Migrations created in %q", tmpDir)

	// Ensure migration files exist
	files = 0
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == migrator.MIGRATION_FILE_SUFFIX {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	if files == 0 {
		t.Fatalf("expected migration files, got none")
	}

	// Migrate
	if err := engine.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify stored migrations
	if len(engine.Migrations["test_sql"]) == 0 {
		t.Fatalf("expected engine to track stored migrations for app 'test_sql' %v", engine.Migrations)
	}

	if len(engine.Migrations["test_sql"]) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(engine.Migrations["test_sql"]))
	}

	if len(engine.Migrations["test_sql"]["Profile"]) != 2 {
		t.Fatalf("expected 2 migrations for Profile, got %d", len(engine.Migrations["test_sql"]["Profile"]))
	}

	if len(engine.Migrations["test_sql"]["Todo"]) != 2 {
		t.Fatalf("expected 2 migration for Todo, got %d", len(engine.Migrations["test_sql"]["Todo"]))
	}

	if len(engine.Migrations["test_sql"]["User"]) != 2 {
		t.Fatalf("expected 2 migration for User, got %d", len(engine.Migrations["test_sql"]["User"]))
	}

	testsql.ExtendedDefinitions = false

	needsToMigrate, err = engine.NeedsToMigrate()
	if err != nil {
		t.Fatalf("NeedsToMigrate failed: %v", err)
	}

	if len(needsToMigrate) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(needsToMigrate))
	}

	if err := engine.MakeMigrations(); err != nil {
		t.Fatalf("MakeMigrations failed: %v", err)
	}

	t.Logf("Migrations created in %q", tmpDir)

	// Ensure migration files exist
	files = 0
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == migrator.MIGRATION_FILE_SUFFIX {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	if files == 0 {
		t.Fatalf("expected migration files, got none")
	}

	// Migrate
	if err := engine.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var (
		latestMigrationProfile = engine.Migrations["test_sql"]["Profile"][len(engine.Migrations["test_sql"]["Profile"])-1]
		latestMigrationTodo    = engine.Migrations["test_sql"]["Todo"][len(engine.Migrations["test_sql"]["Todo"])-1]
		latestMigrationUser    = engine.Migrations["test_sql"]["User"][len(engine.Migrations["test_sql"]["User"])-1]
	)

	// Verify stored migrations
	if len(engine.Migrations["test_sql"]) == 0 {
		t.Fatalf("expected engine to track stored migrations for app 'test_sql' %v", engine.Migrations)
	}

	if len(engine.Migrations["test_sql"]) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(engine.Migrations["test_sql"]))
	}

	if len(engine.Migrations["test_sql"]["Profile"]) != 3 {
		t.Fatalf("expected 2 migrations for Profile, got %d", len(engine.Migrations["test_sql"]["Profile"]))
	}

	if len(engine.Migrations["test_sql"]["Todo"]) != 3 {
		t.Fatalf("expected 2 migration for Todo, got %d", len(engine.Migrations["test_sql"]["Todo"]))
	}

	if len(engine.Migrations["test_sql"]["User"]) != 3 {
		t.Fatalf("expected 2 migration for User, got %d", len(engine.Migrations["test_sql"]["User"]))
	}

	if latestMigrationProfile.Actions[len(latestMigrationProfile.Actions)-1].ActionType != migrator.ActionRemoveField {
		t.Fatalf("expected last action to be AddField, got %s", latestMigrationProfile.Actions[len(latestMigrationProfile.Actions)-1].ActionType)
	}

	if latestMigrationTodo.Actions[len(latestMigrationTodo.Actions)-1].ActionType != migrator.ActionRemoveField {
		t.Fatalf("expected last action to be AddField, got %s", latestMigrationTodo.Actions[len(latestMigrationTodo.Actions)-1].ActionType)
	}

	if latestMigrationUser.Actions[len(latestMigrationUser.Actions)-1].ActionType != migrator.ActionRemoveField {
		t.Fatalf("expected last action to be AddField, got %s", latestMigrationUser.Actions[len(latestMigrationUser.Actions)-1].ActionType)
	}
}
