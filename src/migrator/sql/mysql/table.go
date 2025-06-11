// Package mysql provides a MySQL implementation of the migrator.SchemaEditor interface.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Nigel2392/go-django-queries/src/migrator"
	django "github.com/Nigel2392/go-django/src"
	"github.com/go-sql-driver/mysql"
)

var _ migrator.SchemaEditor = &MySQLSchemaEditor{}

func init() {
	migrator.RegisterSchemaEditor(&mysql.MySQLDriver{}, func() (migrator.SchemaEditor, error) {
		var db, ok = django.ConfigGetOK[*sql.DB](
			django.Global.Settings,
			django.APPVAR_DATABASE,
		)
		if !ok {
			return nil, fmt.Errorf("migrator: mysql: no database connection found")
		}
		return NewMySQLSchemaEditor(db), nil
	})
}

const (
	createTableMigrations = `CREATE TABLE IF NOT EXISTS migrations (
		id INT AUTO_INCREMENT PRIMARY KEY,
		app_name VARCHAR(255) NOT NULL,
		model_name VARCHAR(255) NOT NULL,
		migration_name VARCHAR(255) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY unique_migration (app_name, model_name, migration_name)
	);`
	insertTableMigrations = `INSERT INTO migrations (app_name, model_name, migration_name) VALUES (?, ?, ?);`
	deleteTableMigrations = `DELETE FROM migrations WHERE app_name = ? AND model_name = ? AND migration_name = ?;`
	selectTableMigrations = `SELECT COUNT(*) FROM migrations WHERE app_name = ? AND model_name = ? AND migration_name = ? LIMIT 1;`
)

type MySQLSchemaEditor struct {
	db            *sql.DB
	tablesCreated bool
}

func NewMySQLSchemaEditor(db *sql.DB) *MySQLSchemaEditor {
	return &MySQLSchemaEditor{db: db}
}

func (m *MySQLSchemaEditor) Setup() error {
	if m.tablesCreated {
		return nil
	}
	_, err := m.db.Exec(createTableMigrations)
	if err != nil {
		return err
	}
	m.tablesCreated = true
	return nil
}

func (m *MySQLSchemaEditor) StoreMigration(appName, modelName, migrationName string) error {
	_, err := m.db.Exec(insertTableMigrations, appName, modelName, migrationName)
	return err
}

func (m *MySQLSchemaEditor) HasMigration(appName, modelName, migrationName string) (bool, error) {
	var count int
	err := m.db.QueryRow(selectTableMigrations, appName, modelName, migrationName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *MySQLSchemaEditor) RemoveMigration(appName, modelName, migrationName string) error {
	_, err := m.db.Exec(deleteTableMigrations, appName, modelName, migrationName)
	return err
}

func (m *MySQLSchemaEditor) Execute(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return m.db.ExecContext(ctx, query, args...)
}

func (m *MySQLSchemaEditor) CreateTable(table migrator.Table) error {
	var w strings.Builder
	w.WriteString("CREATE TABLE `")
	w.WriteString(table.TableName())
	w.WriteString("` (")

	var written bool
	for _, col := range table.Columns() {
		if !col.UseInDB {
			continue
		}
		if written {
			w.WriteString(",\n")
		}
		w.WriteString("  ")
		WriteColumn(&w, *col)
		written = true
	}
	w.WriteString("\n);")
	_, err := m.db.Exec(w.String())
	return err
}

func (m *MySQLSchemaEditor) DropTable(table migrator.Table) error {
	query := fmt.Sprintf("DROP TABLE `%s`;", table.TableName())
	_, err := m.db.Exec(query)
	return err
}

func (m *MySQLSchemaEditor) RenameTable(table migrator.Table, newName string) error {
	query := fmt.Sprintf("RENAME TABLE `%s` TO `%s`;", table.TableName(), newName)
	_, err := m.db.Exec(query)
	return err
}

func (m *MySQLSchemaEditor) AddIndex(table migrator.Table, index migrator.Index) error {
	var w strings.Builder
	if index.Unique {
		w.WriteString("CREATE UNIQUE INDEX `")
	} else {
		w.WriteString("CREATE INDEX `")
	}
	w.WriteString(index.Name)
	w.WriteString("` ON `")
	w.WriteString(table.TableName())
	w.WriteString("` (")
	for i, col := range index.Columns {
		if i > 0 {
			w.WriteString(", ")
		}
		w.WriteString("`")
		w.WriteString(col)
		w.WriteString("`")
	}
	w.WriteString(");")
	_, err := m.db.Exec(w.String())
	return err
}

func (m *MySQLSchemaEditor) DropIndex(table migrator.Table, index migrator.Index) error {
	query := fmt.Sprintf("DROP INDEX `%s` ON `%s`;", index.Name, table.TableName())
	_, err := m.db.Exec(query)
	return err
}

func (m *MySQLSchemaEditor) RenameIndex(table migrator.Table, oldName, newName string) error {
	// MySQL does not support RENAME INDEX directly, workaround required
	return fmt.Errorf("mysql does not support RENAME INDEX directly, please drop and recreate")
}

func (m *MySQLSchemaEditor) AddField(table migrator.Table, col migrator.Column) error {
	var w strings.Builder
	w.WriteString("ALTER TABLE `")
	w.WriteString(table.TableName())
	w.WriteString("` ADD COLUMN ")
	WriteColumn(&w, col)
	_, err := m.db.Exec(w.String())
	return err
}

func (m *MySQLSchemaEditor) RemoveField(table migrator.Table, col migrator.Column) error {
	query := fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`;", table.TableName(), col.Column)
	_, err := m.db.Exec(query)
	return err
}

func (m *MySQLSchemaEditor) AlterField(table migrator.Table, oldCol, newCol migrator.Column) error {
	query := fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN ", table.TableName())
	var w strings.Builder
	w.WriteString(query)
	WriteColumn(&w, newCol)
	_, err := m.db.Exec(w.String())
	return err
}

func WriteColumn(w *strings.Builder, col migrator.Column) {
	w.WriteString("`")
	w.WriteString(col.Column)
	w.WriteString("` ")
	w.WriteString(migrator.GetFieldType(
		&mysql.MySQLDriver{}, &col,
	))
	if col.Nullable {
		w.WriteString(" NULL")
	} else {
		w.WriteString(" NOT NULL")
	}
	if col.Auto {
		w.WriteString(" AUTO_INCREMENT")
	}
	if col.Unique {
		w.WriteString(" UNIQUE")
	}
	if col.HasDefault() {
		w.WriteString(" DEFAULT ")
		switch v := col.Default.(type) {
		case string:
			w.WriteString("'")
			w.WriteString(v)
			w.WriteString("'")
		case int, int64, float32, float64:
			w.WriteString(fmt.Sprintf("%v", v))
		case bool:
			if v {
				w.WriteString("TRUE")
			} else {
				w.WriteString("FALSE")
			}
		case time.Time:
			if v.IsZero() {
				w.WriteString("CURRENT_TIMESTAMP")
			} else {
				w.WriteString("'")
				w.WriteString(v.Format("2006-01-02 15:04:05"))
				w.WriteString("'")
			}
		default:
			panic(fmt.Errorf("unsupported default type %T", v))
		}
	}
	if col.Rel != nil {
		relField := col.Rel.Field()
		if relField == nil {
			relField = col.Rel.Model().FieldDefs().Primary()
		}
		w.WriteString(" REFERENCES `")
		w.WriteString(col.Rel.Model().FieldDefs().TableName())
		w.WriteString("`(`")
		w.WriteString(relField.ColumnName())
		w.WriteString("`)")
		if col.Rel.OnDelete != 0 {
			w.WriteString(" ON DELETE ")
			w.WriteString(col.Rel.OnDelete.String())
		}
		if col.Rel.OnUpdate != 0 {
			w.WriteString(" ON UPDATE ")
			w.WriteString(col.Rel.OnUpdate.String())
		}
	}
}
