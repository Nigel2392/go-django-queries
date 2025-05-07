package migrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/pkg/errors"
)

const (
	MIGRATION_FILE_SUFFIX = ".mig"
)

type MigrationFile struct {
	// The name of the application for this migration.
	//
	// This is used to identify the application that the migration is for.
	AppName string `json:"-"`

	// The name of the model for this migration.
	//
	// This is used to identify the model that the migration is for.
	ModelName string `json:"-"`

	// The name of the migration file.
	//
	// This is used to identify the migration and apply it in the correct order.
	Name string `json:"-"`

	// The order of the migration file.
	//
	// This is used to ensure that the migrations are applied in the correct order.
	Order int `json:"-"`

	// TODO: add dependency chaining
	//	// Dependencies are the migration files that this migration depends on.
	//	//
	//	// This is used to ensure that the migrations are applied in the correct order.
	//	// If a migration file has dependencies, it will not be applied until all of its dependencies have been applied.
	//	Dependencies []Dependency `json:"dependencies"`

	// The SQL commands to be executed in the
	// migration file.
	//
	// This is used to apply the migration to the database.
	Table *ModelTable `json:"table"`

	// Actions are the actions that have been taken in this migration file.
	//
	// This is used to keep track of the actions that have been taken in the migration file.
	// These actions are used to generate the migration file name, and can be used to
	// migrate the database to a different state.
	Actions []MigrationAction `json:"actions"`
}

func (m *MigrationFile) addAction(actionType ActionType, table *Changed[*ModelTable], column *Changed[*Column], index *Changed[*Index]) {
	if m.Actions == nil {
		m.Actions = make([]MigrationAction, 0)
	}
	m.Actions = append(m.Actions, MigrationAction{
		ActionType: actionType,
		Table:      table,
		Field:      column,
		Index:      index,
	})
}

func (m *MigrationFile) FileName() string {
	return generateMigrationFileName(m)
}

type MigrationLog interface {
	Log(action ActionType, file *MigrationFile, table *Changed[*ModelTable], column *Changed[*Column], index *Changed[*Index])
}

type MigrationEngine struct {
	// The path to the directory where the migration files are stored.
	//
	// This is used to load the migration files and apply them to the database.
	Path string

	// SchemaEditor is the schema editor used to apply the migrations to the database.
	//
	// This is used to execute SQL commands for creating, modifying, and deleting tables and columns.
	SchemaEditor SchemaEditor

	// Migrations is the list of migration files that have been applied to the database.
	//
	// This is used to keep track of the migrations that have been applied and ensure that they are not applied again.
	Migrations map[string]map[string][]*MigrationFile

	// MigrationLog is the migration log used to log the actions taken by the migration engine.
	//
	// This is used to log the actions taken by the migration engine for debugging and auditing purposes.
	MigrationLog MigrationLog
}

func NewMigrationEngine(path string) *MigrationEngine {
	return &MigrationEngine{
		Path: path,
	}
}

func (m *MigrationEngine) Log(action ActionType, file *MigrationFile, table *Changed[*ModelTable], column *Changed[*Column], index *Changed[*Index]) {
	if m.MigrationLog == nil {
		return
	}
	file.Actions = append(file.Actions, MigrationAction{
		ActionType: action,
		Table:      table,
		Field:      column,
		Index:      index,
	})
	m.MigrationLog.Log(action, file, table, column, index)
}

// GetLastAppliedMigration returns the last applied migration for the given app and model.
func (m *MigrationEngine) GetLastAppliedMigration(appName, modelName string) *MigrationFile {
	migrations, ok := m.Migrations[appName]
	if !ok {
		return nil
	}

	modelMigrations, ok := migrations[modelName]
	if !ok {
		return nil
	}

	if len(modelMigrations) == 0 {
		return nil
	}

	return modelMigrations[len(modelMigrations)-1]
}

func (m *MigrationEngine) Migrate() error {

	if err := m.SchemaEditor.Setup(); err != nil {
		return errors.Wrap(err, "failed to setup schema editor")
	}

	var migrations, err = ReadMigrations(m.Path)
	if err != nil {
		return errors.Wrap(err, "failed to read migrations")
	}

	var unappliedMigrations = make([]*MigrationFile, 0)
	for _, migration := range migrations {
		var hasApplied, err = m.SchemaEditor.HasMigration(
			migration.AppName,
			migration.ModelName,
			migration.FileName(),
		)

		if err != nil {
			return errors.Wrapf(
				err, "failed to check if migration %q has been applied", migration.Name,
			)
		}

		if hasApplied {
			continue
		}

		unappliedMigrations = append(unappliedMigrations, migration)
	}

	m.Migrations = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	for _, migration := range unappliedMigrations {
		var defs = migration.Table.Object.FieldDefs()
		for _, action := range migration.Actions {
			var err error
			switch action.ActionType {
			case ActionCreateTable:
				err = m.SchemaEditor.CreateTable(migration.Table)
			case ActionDropTable:
				err = m.SchemaEditor.DropTable(action.Table.Old)
			case ActionRenameTable:
				err = m.SchemaEditor.RenameTable(action.Table.Old, action.Table.New.TableName())
			case ActionAddField:
				action.Field.New.Table = migration.Table
				action.Field.New.Field, _ = defs.Field(action.Field.New.Name)
				err = m.SchemaEditor.AddField(migration.Table, *action.Field.New)
			case ActionAlterField:
				action.Field.Old.Table = migration.Table
				action.Field.Old.Field, _ = defs.Field(action.Field.Old.Name)
				action.Field.New.Table = migration.Table
				action.Field.New.Field, _ = defs.Field(action.Field.New.Name)
				err = m.SchemaEditor.AlterField(migration.Table, *action.Field.Old, *action.Field.New)
			case ActionRemoveField:
				action.Field.Old.Table = migration.Table
				action.Field.Old.Field, _ = defs.Field(action.Field.Old.Name)
				err = m.SchemaEditor.RemoveField(migration.Table, *action.Field.Old)
			case ActionAddIndex:
				err = m.SchemaEditor.AddIndex(migration.Table, *action.Index.New)
			case ActionDropIndex:
				err = m.SchemaEditor.DropIndex(migration.Table, *action.Index.Old)
			case ActionRenameIndex:
				err = m.SchemaEditor.RenameIndex(migration.Table, action.Index.Old.Name, action.Index.New.Name)
			// case ActionAlterUniqueTogether:
			// 	err = m.SchemaEditor.AlterUniqueTogether(action.Table.New, action.Field.New.Unique)
			// case ActionAlterIndexTogether:
			// 	err = m.SchemaEditor.AlterIndexTogether(action.Table.New, action.Field.New.Index)
			default:
				return fmt.Errorf("unknown action type %d", action.ActionType)
			}

			if err != nil {
				return errors.Wrapf(
					err, "failed to apply migration %q", migration.Name,
				)
			}
		}
		err = m.SchemaEditor.StoreMigration(
			migration.AppName,
			migration.ModelName,
			migration.FileName(),
		)
		if err != nil {
			return errors.Wrapf(
				err, "failed to store migration %q", migration.Name,
			)
		}
	}

	return nil
}

func (m *MigrationEngine) NeedsToMigrate() ([]*contenttypes.BaseContentType[attrs.Definer], error) {

	if err := m.SchemaEditor.Setup(); err != nil {
		return nil, errors.Wrap(err, "failed to setup schema editor")
	}

	os.MkdirAll(m.Path, 0755)

	var migrations, err = ReadMigrations(m.Path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read migrations")
	}

	m.Migrations = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	var needsToMigrate = make([]*contenttypes.BaseContentType[attrs.Definer], 0)
	for head := registeredModels.Front(); head != nil; head = head.Next() {
		var (
			modelName = head.Key
			def       = head.Value
			appLabel  = def.AppLabel()
			model     = def.Model()
		)

		// Skip if already applied
		var last = m.GetLastAppliedMigration(appLabel, model)

		// Build current table state
		var currTable = NewModelTable(def.New())

		// Compare to last migration
		var mig, err = m.NewMigration(appLabel, model, currTable)
		if err != nil {
			return nil, fmt.Errorf("MakeMigrations: failed to generate migration for %s: %w", modelName, err)
		}

		var newMigrationNeeded bool = true
		newMigrationNeeded = m.makeMigrationDiff(
			mig, last, currTable,
		)

		if newMigrationNeeded {
			needsToMigrate = append(needsToMigrate, def)
		}
	}

	return needsToMigrate, nil
}

func (m *MigrationEngine) MakeMigrations() error {

	if err := m.SchemaEditor.Setup(); err != nil {
		return errors.Wrap(err, "failed to setup schema editor")
	}

	os.MkdirAll(m.Path, 0755)

	var migrations, err = ReadMigrations(m.Path)
	if err != nil {
		return errors.Wrap(err, "failed to read migrations")
	}

	m.Migrations = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	for head := registeredModels.Front(); head != nil; head = head.Next() {
		var (
			modelName = head.Key
			def       = head.Value
		)

		var appLabel = def.AppLabel()
		var model = def.Model()

		// Skip if already applied
		var last = m.GetLastAppliedMigration(appLabel, model)

		// Build current table state
		var currTable = NewModelTable(def.New())

		// Compare to last migration
		var mig, err = m.NewMigration(appLabel, model, currTable)
		if err != nil {
			return fmt.Errorf("MakeMigrations: failed to generate migration for %s: %w", modelName, err)
		}

		var newMigrationNeeded bool = true
		newMigrationNeeded = m.makeMigrationDiff(
			mig, last, currTable,
		)

		if !newMigrationNeeded {
			continue
		}

		mig.Name = generateMigrationFileName(mig)

		if err := WriteMigration(m.Path, mig); err != nil {
			return fmt.Errorf("MakeMigrations: failed to write migration for %s: %w", modelName, err)
		}
	}

	return nil
}

// ApplyTableMigrationDiff applies the migration to the database.
func (m *MigrationEngine) makeMigrationDiff(migration *MigrationFile, last *MigrationFile, table *ModelTable) (shouldMigrate bool) {
	if last == nil || last.Table == nil {
		// return m.SchemaEditor.CreateTable(table)
		migration.addAction(ActionCreateTable, nil, nil, nil)
		m.Log(ActionCreateTable, migration, unchanged(table), nil, nil)
		return true
	}

	var lastAppliedTable = last.Table
	if table == nil {
		// return m.SchemaEditor.DropTable(lastAppliedTable)
		migration.addAction(ActionDropTable, changed(lastAppliedTable, nil), nil, nil)
		m.Log(ActionDropTable, migration, unchanged(lastAppliedTable), nil, nil)
		return true
	}

	if lastAppliedTable.TableName() != table.TableName() {
		// m.SchemaEditor.RenameTable(lastAppliedTable, table.TableName())
		migration.addAction(ActionRenameTable, changed(lastAppliedTable, table), nil, nil)
		m.Log(ActionRenameTable, migration, changed(lastAppliedTable, table), nil, nil)
		shouldMigrate = true
	}

	var added, removed, diffs = table.Diff(lastAppliedTable)

	for _, col := range added {
		//var err = m.SchemaEditor.AddField(table, col)
		//if err != nil {
		//	return errors.Wrapf(
		//		err, "failed to add column %q to table %q", col.Name, table.TableName(),
		//	)
		//}
		migration.addAction(ActionAddField, nil, unchanged(&col), nil)
		m.Log(ActionAddField, migration, unchanged(table), unchanged(&col), nil)
		shouldMigrate = true
	}

	for _, col := range removed {
		//var err = m.SchemaEditor.RemoveField(table, col)
		//if err != nil {
		//	return errors.Wrapf(
		//		err, "failed to remove column %q from table %q", col.Name, table.TableName(),
		//	)
		//}
		migration.addAction(ActionRemoveField, nil, unchanged(&col), nil)
		m.Log(ActionRemoveField, migration, unchanged(table), changed(&col, nil), nil)
		shouldMigrate = true
	}

	for _, col := range diffs {
		//var err = m.SchemaEditor.AlterField(table, col.Old, col.New)
		//if err != nil {
		//	return errors.Wrapf(
		//		err, "failed to alter column %q in table %q", col.Old.Name, table.TableName(),
		//	)
		//}
		migration.addAction(ActionAlterField, nil, changed(&col.Old, &col.New), nil)
		m.Log(ActionAlterField, migration, unchanged(table), changed(&col.Old, &col.New), nil)
		shouldMigrate = true
	}

	var (
		oldIndexes = lastAppliedTable.Indexes()
		newIndexes = table.Indexes()
		oldMap     = make(map[string]Index, len(oldIndexes))
		newMap     = make(map[string]Index, len(newIndexes))
	)

	for _, idx := range oldIndexes {
		oldMap[idx.Name] = idx
	}
	for _, idx := range newIndexes {
		newMap[idx.Name] = idx
	}

	// Drop removed or changed indexes
	for name, oldIdx := range oldMap {
		var newIdx, exists = newMap[name]
		if !exists || !indexesEqual(oldIdx, newIdx) {
			//if err := m.SchemaEditor.DropIndex(table, oldIdx); err != nil {
			//	return errors.Wrapf(err, "failed to drop index %q", oldIdx.Name)
			//}
			migration.addAction(ActionDropIndex, nil, nil, changed(&oldIdx, nil))
			m.Log(ActionDropIndex, migration, unchanged(table), nil, changed(&oldIdx, nil))
			shouldMigrate = true
		}
	}

	// Add new or changed indexes
	for name, newIdx := range newMap {
		var oldIdx, exists = oldMap[name]
		if !exists || !indexesEqual(oldIdx, newIdx) {
			//if err := m.SchemaEditor.AddIndex(table, newIdx); err != nil {
			//	return errors.Wrapf(err, "failed to add index %q", newIdx.Name)
			//}
			migration.addAction(ActionAddIndex, nil, nil, changed(nil, &newIdx))
			m.Log(ActionAddIndex, migration, unchanged(table), nil, unchanged(&newIdx))
			shouldMigrate = true
		}
	}

	// Detect and rename matching indexes with different names
	for oldName, oldIdx := range oldMap {
		for newName, candidate := range newMap {
			if indexesEqual(oldIdx, candidate) && oldName != newName {
				//if err := m.SchemaEditor.RenameIndex(table, oldName, newName); err != nil {
				//	return errors.Wrapf(err, "failed to rename index %q to %q", oldName, newName)
				//}
				delete(oldMap, oldName)
				delete(newMap, newName)
				migration.addAction(ActionRenameIndex, nil, nil, changed(&oldIdx, &candidate))
				m.Log(ActionRenameIndex, migration, unchanged(table), nil, changed(&oldIdx, &candidate))
				shouldMigrate = true
				break
			}
		}
	}

	return shouldMigrate
}

func (e *MigrationEngine) NewMigration(appName, modelName string, newTable *ModelTable) (*MigrationFile, error) {
	// load latest applied migration if it exists
	var last = e.GetLastAppliedMigration(appName, modelName)

	// Get last order
	var nextOrder = 1
	if last != nil {
		nextOrder = last.Order + 1
	}

	// Name this migration something useful later on
	var name = "auto_generated"

	// Build tables map
	return &MigrationFile{
		AppName:   appName,
		ModelName: modelName,
		Name:      name,
		Order:     nextOrder,
		Table:     newTable,
	}, nil
}

func (m *MigrationEngine) storeMigration(mig *MigrationFile) {
	if m.Migrations == nil {
		m.Migrations = make(map[string]map[string][]*MigrationFile)
	}

	var appMigrations, ok = m.Migrations[mig.AppName]
	if !ok {
		appMigrations = make(map[string][]*MigrationFile)
		m.Migrations[mig.AppName] = appMigrations
	}

	modelMigrations, ok := appMigrations[mig.ModelName]
	if !ok {
		modelMigrations = make([]*MigrationFile, 0)
		appMigrations[mig.ModelName] = modelMigrations
	}

	modelMigrations = append(modelMigrations, mig)
	appMigrations[mig.ModelName] = modelMigrations
}

func indexesEqual(a, b Index) bool {
	if a.Name != b.Name || a.Unique != b.Unique || a.Type != b.Type {
		return false
	}

	if len(a.Columns) != len(b.Columns) {
		return false
	}

	for i := range a.Columns {
		if a.Columns[i] != b.Columns[i] {
			return false
		}
	}

	return true
}

// WriteMigration writes the migration file to the specified path.
//
// The migration file is used to apply the migrations to the database.
func WriteMigration(path string, migration *MigrationFile) error {
	var filePath = filepath.Join(path, migration.AppName, migration.ModelName, migration.FileName())

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Errorf("migration file %q already exists", filePath)
	}

	var data, err = json.MarshalIndent(migration, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshal migration file %q", filePath)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", filepath.Dir(filePath))
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return errors.Wrapf(err, "failed to write migration file %q", filePath)
	}

	return nil
}

// ReadMigrations reads the migration files from the specified path and returns a list of migration files.
//
// These migration files are used to apply the migrations to the database.
func ReadMigrations(path string) ([]*MigrationFile, error) {

	var directories, err = os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	if _, err = os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil, errors.Wrapf(
			err, "failed to read migration directory %q", path,
		)
	}

	var migrations = make([]*MigrationFile, 0)
	for _, appMigrationDir := range directories {
		if !appMigrationDir.IsDir() {
			continue
		}

		var workingPath = filepath.Join(
			path, appMigrationDir.Name(),
		)

		if _, err = os.Stat(workingPath); err != nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(
				err, "failed to read migration directory %q", workingPath,
			)
		}

		var files, err = os.ReadDir(workingPath)
		if err != nil {
			return nil, errors.Wrapf(
				err, "failed to read migration directory %q", workingPath,
			)
		}

		for _, modelMigrationDir := range files {
			if !modelMigrationDir.IsDir() {
				continue
			}

			var filesDir = filepath.Join(workingPath, modelMigrationDir.Name())

			if _, err = os.Stat(filesDir); err != nil && os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, errors.Wrapf(
					err, "failed to read migration directory %q", filesDir,
				)
			}

			var files, err = os.ReadDir(filesDir)
			if err != nil {
				return nil, errors.Wrapf(
					err, "failed to read migration directory %q", filesDir,
				)
			}

			for _, file := range files {
				var filePath = filepath.Join(
					filesDir, file.Name(),
				)

				if file.IsDir() || filepath.Ext(file.Name()) != MIGRATION_FILE_SUFFIX {
					continue
				}

				if _, err = os.Stat(filePath); err != nil && os.IsNotExist(err) {
					continue
				}

				var migrationFileBytes, err = os.ReadFile(filePath)
				if err != nil {
					return nil, errors.Wrapf(
						err, "failed to read migration file %q", filePath,
					)
				}

				var migrationFile = new(MigrationFile)
				if err := json.Unmarshal(migrationFileBytes, &migrationFile); err != nil {
					return nil, errors.Wrapf(
						err, "failed to unmarshal migration file %q", filePath,
					)
				}

				orderNum, name, err := parseMigrationFileName(file.Name())
				if err != nil {
					return nil, errors.Wrapf(
						err, "failed to parse migration file name %q", file.Name(),
					)
				}

				migrations = append(migrations, &MigrationFile{
					Name:      name,
					AppName:   appMigrationDir.Name(),
					ModelName: modelMigrationDir.Name(),
					Order:     orderNum,
					Table:     migrationFile.Table,
					Actions:   migrationFile.Actions,
				})
			}
		}
	}

	slices.SortStableFunc(migrations, func(a, b *MigrationFile) int {
		if a.Order < b.Order {
			return -1
		}
		if a.Order > b.Order {
			return 1
		}
		return 0
	})

	return migrations, nil
}
