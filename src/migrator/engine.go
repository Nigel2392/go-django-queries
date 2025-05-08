package migrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/elliotchance/orderedmap/v2"
	"github.com/pkg/errors"
)

const (
	MIGRATION_FILE_SUFFIX = ".mig"
)

type Dependency struct {
	AppName   string
	ModelName string
	Name      string
}

func (d *Dependency) MarshalJSON() ([]byte, error) {
	var s = strings.Join([]string{d.AppName, d.ModelName, d.Name}, ":")
	return json.Marshal(s)
}

func (d *Dependency) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return errors.Wrap(err, "failed to unmarshal dependency")
	}

	var parts = strings.SplitN(str, ":", 3)
	if len(parts) != 3 {
		return fmt.Errorf("invalid dependency format: %q", str)
	}

	d.AppName = parts[0]
	d.ModelName = parts[1]
	d.Name = parts[2]
	return nil
}

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

	// ContentType is the content type for the model of this migration.
	//
	// This is used to identify the model that the migration is for.
	ContentType *contenttypes.BaseContentType[attrs.Definer] `json:"-"`

	// Dependencies are the migration files that this migration depends on.
	//
	// This is used to ensure that the migrations are applied in the correct order.
	// If a migration file has dependencies, it will not be applied until all of its dependencies have been applied.
	Dependencies []Dependency `json:"dependencies,omitempty"`

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

func (m *MigrationFile) addDependency(appName, modelName, name string) {
	if m.Dependencies == nil {
		m.Dependencies = make([]Dependency, 0)
	}
	m.Dependencies = append(m.Dependencies, Dependency{
		AppName:   appName,
		ModelName: modelName,
		Name:      name,
	})
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

	// dependencies is a map of migration files used for dependency resolution.
	//
	// This is used to ensure that the migrations are applied in the correct order.
	dependencies map[string]map[string][]*MigrationFile

	// apps is an ordered map of applications used for dependency resolution and migrations.
	//
	// The apps contain a slice of models that are used to generate the migration files.
	apps *orderedmap.OrderedMap[string, django.AppConfig]
}

func NewMigrationEngine(path string, schemaEditor SchemaEditor, apps ...string) *MigrationEngine {
	var appMap = orderedmap.NewOrderedMap[string, django.AppConfig]()

	if len(apps) == 0 {
		appMap = django.Global.Apps
	} else {
		for _, app := range apps {
			var appConfig = django.GetApp[django.AppConfig](app)
			appMap.Set(app, appConfig)
		}
	}

	return &MigrationEngine{
		Path:         path,
		SchemaEditor: schemaEditor,
		apps:         appMap,
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

// GetLastMigration returns the last applied migration for the given app and model.
func (m *MigrationEngine) GetLastMigration(appName, modelName string) *MigrationFile {
	return latestFromMap(m.Migrations, appName, modelName)
}

func (m *MigrationEngine) Migrate() error {

	if err := m.SchemaEditor.Setup(); err != nil {
		return errors.Wrap(err, "failed to setup schema editor")
	}

	var migrations, err = m.ReadMigrations()
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
	m.dependencies = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	graph, err := buildDependencyGraph(unappliedMigrations)
	if err != nil {
		return err
	}

	for _, n := range graph {
		var defs = n.mig.Table.Object.FieldDefs()

		for _, action := range n.mig.Actions {
			var err error
			switch action.ActionType {
			case ActionCreateTable:
				err = m.SchemaEditor.CreateTable(n.mig.Table)
			case ActionDropTable:
				err = m.SchemaEditor.DropTable(action.Table.Old)
			case ActionRenameTable:
				err = m.SchemaEditor.RenameTable(action.Table.Old, action.Table.New.TableName())
			case ActionAddField:
				action.Field.New.Table = n.mig.Table
				action.Field.New.Field, _ = defs.Field(action.Field.New.Name)
				err = m.SchemaEditor.AddField(n.mig.Table, *action.Field.New)
			case ActionAlterField:
				action.Field.Old.Table = n.mig.Table
				action.Field.Old.Field, _ = defs.Field(action.Field.Old.Name)
				action.Field.New.Table = n.mig.Table
				action.Field.New.Field, _ = defs.Field(action.Field.New.Name)
				err = m.SchemaEditor.AlterField(n.mig.Table, *action.Field.Old, *action.Field.New)
			case ActionRemoveField:
				action.Field.Old.Table = n.mig.Table
				action.Field.Old.Field, _ = defs.Field(action.Field.Old.Name)
				err = m.SchemaEditor.RemoveField(n.mig.Table, *action.Field.Old)
			case ActionAddIndex:
				err = m.SchemaEditor.AddIndex(n.mig.Table, *action.Index.New)
			case ActionDropIndex:
				err = m.SchemaEditor.DropIndex(n.mig.Table, *action.Index.Old)
			case ActionRenameIndex:
				err = m.SchemaEditor.RenameIndex(n.mig.Table, action.Index.Old.Name, action.Index.New.Name)
			// case ActionAlterUniqueTogether:
			// 	err = m.SchemaEditor.AlterUniqueTogether(action.Table.New, action.Field.New.Unique)
			// case ActionAlterIndexTogether:
			// 	err = m.SchemaEditor.AlterIndexTogether(action.Table.New, action.Field.New.Index)
			default:
				return fmt.Errorf("unknown action type %d", action.ActionType)
			}

			if err != nil {
				return errors.Wrapf(
					err, "failed to apply migration %q", n.mig.Name,
				)
			}
		}
		err = m.SchemaEditor.StoreMigration(
			n.mig.AppName,
			n.mig.ModelName,
			n.mig.FileName(),
		)
		if err != nil {
			return errors.Wrapf(
				err, "failed to store migration %q", n.mig.Name,
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

	var migrations, err = m.ReadMigrations()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read migrations")
	}

	m.Migrations = make(map[string]map[string][]*MigrationFile)
	m.dependencies = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	var needsToMigrate = make([]*contenttypes.BaseContentType[attrs.Definer], 0)
	for head := m.apps.Front(); head != nil; head = head.Next() {
		var (
			def     = head.Value
			appName = head.Key
		)

		for _, model := range def.Models() {
			var cType = contenttypes.NewContentType(model)
			var modelName = cType.Model()

			// Build current table state
			var currTable = NewModelTable(cType.New())

			// Compare to last migration
			var mig, err = m.NewMigration(appName, modelName, currTable, cType)
			if err != nil {
				return nil, fmt.Errorf("MakeMigrations: failed to generate migration for %s: %w", modelName, err)
			}

			var last = m.GetLastMigration(
				mig.AppName, mig.ModelName,
			)
			var newMigrationNeeded bool = true
			newMigrationNeeded = m.makeMigrationDiff(
				mig, last, mig.Table,
			)

			if newMigrationNeeded {
				needsToMigrate = append(needsToMigrate, cType)
			}
		}
	}

	return needsToMigrate, nil
}

func (m *MigrationEngine) MakeMigrations() error {

	if err := m.SchemaEditor.Setup(); err != nil {
		return errors.Wrap(err, "failed to setup schema editor")
	}

	os.MkdirAll(m.Path, 0755)

	var migrations, err = m.ReadMigrations()
	if err != nil {
		return errors.Wrap(err, "failed to read migrations")
	}

	m.Migrations = make(map[string]map[string][]*MigrationFile)
	m.dependencies = make(map[string]map[string][]*MigrationFile)
	for _, migration := range migrations {
		m.storeMigration(migration)
	}

	var (
		migrationList = make([]*MigrationFile, 0)
		dependencies  = make(map[string]map[string]*MigrationFile)
	)
	for head := m.apps.Front(); head != nil; head = head.Next() {
		var (
			def     = head.Value
			appName = head.Key
		)

		for _, model := range def.Models() {
			var cType = contenttypes.NewContentType(model)
			var modelName = cType.Model()

			var appLabel = appName
			var model = modelName

			// Build current table state
			var currTable = NewModelTable(cType.New())

			// Compare to last migration
			var mig, err = m.NewMigration(appLabel, model, currTable, cType)
			if err != nil {
				return fmt.Errorf("MakeMigrations: failed to generate migration for %s: %w", modelName, err)
			}

			var last = m.GetLastMigration(mig.AppName, mig.ModelName)
			if !m.makeMigrationDiff(mig, last, mig.Table) {
				continue
			}

			mig.Name = generateMigrationFileName(mig)

			m.storeDependency(mig)

			migrationList = append(migrationList, mig)
			if dependencies[mig.AppName] == nil {
				dependencies[mig.AppName] = make(map[string]*MigrationFile)
			}
			dependencies[mig.AppName][mig.ModelName] = mig
		}
	}

	// Check for dependencies and write migration files
	for _, mig := range migrationList {

		// Check for dependencies
		for _, col := range mig.Table.Columns() {
			if col.Rel != nil {
				var (
					relApp     = getModelApp(col.Rel.TargetModel.New())
					relAppName = relApp.Name()
					relModel   = col.Rel.TargetModel.Model()
					depMig, ok = dependencies[relAppName][relModel]
				)
				if !ok {
					continue
				}

				mig.addDependency(relAppName, relModel, depMig.FileName())
			}
		}

		// Write the migration file
		if err := m.WriteMigration(mig); err != nil {
			return err
		}
	}

	return nil
}

type node struct {
	mig      *MigrationFile
	deps     []*node
	visited  bool
	visiting bool
}

func buildDependencyGraph(migrations []*MigrationFile) ([]*node, error) {
	var nodeMap = make(map[string]*node)

	// helper to create a unique key for lookup
	var key = func(m *MigrationFile) string {
		return fmt.Sprintf("%s:%s:%s", m.AppName, m.ModelName, m.FileName())
	}

	// Step 1: Create node for each migration
	for _, m := range migrations {
		nodeMap[key(m)] = &node{mig: m}
	}

	// Step 2: Link dependencies
	for _, n := range nodeMap {
		for _, dep := range n.mig.Dependencies {
			depKey := fmt.Sprintf("%s:%s:%s", dep.AppName, dep.ModelName, dep.Name)
			depNode, ok := nodeMap[depKey]
			if !ok {
				return nil, fmt.Errorf("missing dependency: %s (%v)", depKey, nodeMap)
			}
			n.deps = append(n.deps, depNode)
		}
	}

	// Step 3: Topological sort
	var ordered []*node
	var visit func(n *node) error
	visit = func(n *node) error {
		if n.visited {
			return nil
		}
		if n.visiting {
			return fmt.Errorf("cyclic dependency detected for migration: %s", key(n.mig))
		}
		n.visiting = true
		for _, dep := range n.deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		n.visited = true
		n.visiting = false
		ordered = append(ordered, n)
		return nil
	}

	for _, n := range nodeMap {
		if !n.visited {
			if err := visit(n); err != nil {
				return nil, err
			}
		}
	}

	return ordered, nil
}

// makeMigrationDiff diffs the last migration with the current table state and returns true if a migration is needed.
func (m *MigrationEngine) makeMigrationDiff(migration *MigrationFile, last *MigrationFile, table *ModelTable) (shouldMigrate bool) {
	if last == nil || last.Table == nil {
		migration.addAction(ActionCreateTable, nil, nil, nil)
		m.Log(ActionCreateTable, migration, unchanged(table), nil, nil)
		return true
	}

	var lastAppliedTable = last.Table
	if table == nil {
		migration.addAction(ActionDropTable, changed(lastAppliedTable, nil), nil, nil)
		m.Log(ActionDropTable, migration, unchanged(lastAppliedTable), nil, nil)
		return true
	}

	if lastAppliedTable.TableName() != table.TableName() {
		migration.addAction(ActionRenameTable, changed(lastAppliedTable, table), nil, nil)
		m.Log(ActionRenameTable, migration, changed(lastAppliedTable, table), nil, nil)
		shouldMigrate = true
	}

	var added, removed, diffs = table.Diff(lastAppliedTable)

	for _, col := range added {
		migration.addAction(ActionAddField, nil, unchanged(&col), nil)
		m.Log(ActionAddField, migration, unchanged(table), unchanged(&col), nil)
		shouldMigrate = true
	}

	for _, col := range removed {
		migration.addAction(ActionRemoveField, nil, changed(&col, nil), nil)
		m.Log(ActionRemoveField, migration, unchanged(table), changed(&col, nil), nil)
		shouldMigrate = true
	}

	for _, col := range diffs {
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
			migration.addAction(ActionDropIndex, nil, nil, changed(&oldIdx, nil))
			m.Log(ActionDropIndex, migration, unchanged(table), nil, changed(&oldIdx, nil))
			shouldMigrate = true
		}
	}

	// Add new or changed indexes
	for name, newIdx := range newMap {
		var oldIdx, exists = oldMap[name]
		if !exists || !indexesEqual(oldIdx, newIdx) {
			migration.addAction(ActionAddIndex, nil, nil, changed(nil, &newIdx))
			m.Log(ActionAddIndex, migration, unchanged(table), nil, unchanged(&newIdx))
			shouldMigrate = true
		}
	}

	// Detect and rename matching indexes with different names
	for oldName, oldIdx := range oldMap {
		for newName, candidate := range newMap {
			if indexesEqual(oldIdx, candidate) && oldName != newName {
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

func (e *MigrationEngine) NewMigration(appName, modelName string, newTable *ModelTable, def *contenttypes.BaseContentType[attrs.Definer]) (*MigrationFile, error) {
	// load latest applied migration if it exists
	var last = e.GetLastMigration(appName, modelName)

	// Get last order
	var nextOrder = 1
	if last != nil {
		nextOrder = last.Order + 1
	}

	// Name this migration something useful later on
	var name = "auto_generated"

	// Build tables map
	return &MigrationFile{
		AppName:     appName,
		ModelName:   modelName,
		ContentType: def,
		Name:        name,
		Order:       nextOrder,
		Table:       newTable,
	}, nil
}

// store a dependency in the migration map
//
// this is used to keep track of the dependencies between migration files
// so that they can be applied in the correct order
func (m *MigrationEngine) storeDependency(mig *MigrationFile) {
	if m.dependencies == nil {
		m.dependencies = make(map[string]map[string][]*MigrationFile)
	}

	storeInMap(m.dependencies, mig)
}

// storeMigration stores the migration file in the migration map.
//
// it will also automatically store a copy of the migration file in the dependencies
func (m *MigrationEngine) storeMigration(mig *MigrationFile) {
	if m.Migrations == nil {
		m.Migrations = make(map[string]map[string][]*MigrationFile)
	}

	storeInMap(m.Migrations, mig)

	m.storeDependency(mig)
}

func latestFromMap(m map[string]map[string][]*MigrationFile, appName, modelName string) *MigrationFile {
	if m == nil {
		return nil
	}

	var appMigrations, ok = m[appName]
	if !ok {
		return nil
	}

	modelMigrations, ok := appMigrations[modelName]
	if !ok || len(modelMigrations) == 0 {
		return nil
	}

	return modelMigrations[len(modelMigrations)-1]
}

func storeInMap(m map[string]map[string][]*MigrationFile, mig *MigrationFile) {
	var appMigrations, ok = m[mig.AppName]
	if !ok {
		appMigrations = make(map[string][]*MigrationFile)
		m[mig.AppName] = appMigrations
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
func (e *MigrationEngine) WriteMigration(migration *MigrationFile) error {
	var filePath = filepath.Join(e.Path, migration.AppName, migration.ModelName, migration.FileName())

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
func (e *MigrationEngine) ReadMigrations() ([]*MigrationFile, error) {

	var directories, err = os.ReadDir(e.Path)
	if err != nil {
		return nil, err
	}

	if _, err = os.Stat(e.Path); err != nil && os.IsNotExist(err) {
		return nil, errors.Wrapf(
			err, "failed to read migration directory %q", e.Path,
		)
	}

	var migrations = make([]*MigrationFile, 0)
	for _, appMigrationDir := range directories {
		if !appMigrationDir.IsDir() {
			continue
		}

		var workingPath = filepath.Join(
			e.Path, appMigrationDir.Name(),
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

		if _, ok := e.apps.Get(appMigrationDir.Name()); !ok {
			panic(fmt.Sprintf("app %q not found in migration engines' apps list", appMigrationDir.Name()))
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
					Name:         name,
					AppName:      appMigrationDir.Name(),
					ModelName:    modelMigrationDir.Name(),
					Order:        orderNum,
					Table:        migrationFile.Table,
					Actions:      migrationFile.Actions,
					Dependencies: migrationFile.Dependencies,
					ContentType:  contenttypes.NewContentType(migrationFile.Table.Object),
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
