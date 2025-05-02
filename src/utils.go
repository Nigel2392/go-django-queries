package queries

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/jmoiron/sqlx"
)

type SupportsReturning string

const (
	SupportsReturningNone         SupportsReturning = ""
	SupportsReturningLastInsertId SupportsReturning = "last_insert_id"
	SupportsReturningColumns      SupportsReturning = "columns"
)

// RegisterDriver registers a driver with the given database name.
//
// This is used to determine the database type when using sqlx.
//
// If your driver is not one of:
// - github.com/go-sql-driver/mysql.MySQLDriver
// - github.com/mattn/go-sqlite3.SQLiteDriver
// - github.com/jackc/pgx/v5/stdlib.Driver
//
// Then it explicitly needs to be registered here.
func RegisterDriver(driver driver.Driver, database string, supportsReturning ...SupportsReturning) {
	var s SupportsReturning
	if len(supportsReturning) > 0 {
		s = supportsReturning[0]
	}
	drivers[reflect.TypeOf(driver)] = driverData{
		name:              database,
		supportsReturning: s,
	}
}

type driverData struct {
	name              string
	supportsReturning SupportsReturning
}

var drivers = make(map[reflect.Type]driverData)

func sqlxDriverName(db *sql.DB) string {
	var driver = reflect.TypeOf(db.Driver())
	if driver == nil {
		return ""
	}
	if data, ok := drivers[driver]; ok {
		return data.name
	}
	return ""
}

func supportsReturning(db *sql.DB) SupportsReturning {
	var driver = reflect.TypeOf(db.Driver())
	if driver == nil {
		return SupportsReturningNone
	}
	if data, ok := drivers[driver]; ok {
		return data.supportsReturning
	}
	return SupportsReturningNone
}

func DefinerListToList[T attrs.Definer](list []attrs.Definer) []T {
	var result = make([]T, len(list))
	for i, obj := range list {
		result[i] = obj.(T)
	}
	return result
}

func newDefiner[T attrs.Definer]() T {
	return newObjectFromIface(*new(T)).(T)
}

func newObjectFromIface(obj attrs.Definer) attrs.Definer {
	var objTyp = reflect.TypeOf(obj)
	if objTyp.Kind() != reflect.Ptr {
		panic("newObjectFromIface: objTyp is not a pointer")
	}
	return reflect.New(objTyp.Elem()).Interface().(attrs.Definer)
}

// safer alias generator
func newJoinAlias(field attrs.Field, tableName string, chain []string) string {
	var l = len(chain)
	return fmt.Sprintf("%s_%s_%d", field.ColumnName(), tableName, l-1)
	//	if l > 1 {
	//}
	//return fmt.Sprintf("%s_%s", field.ColumnName(), tableName)
}

func walkFields(
	m attrs.Definer,
	column string,
) (
	definer attrs.Definer,
	parent attrs.Definer,
	f attrs.Field,
	chain []string,
	aliases []string,
	isRelated bool,
	err error,
) {
	var parts = strings.Split(column, ".")
	var current = m
	var field attrs.Field

	chain = make([]string, 0, len(parts)-1)
	aliases = make([]string, 0, len(parts)-1)

	for i, part := range parts {
		defs := current.FieldDefs()
		f, ok := defs.Field(part)
		if !ok {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q not found in %T", part, current)
		}
		field = f

		if i == len(parts)-1 {
			break
		}

		chain = append(chain, part)
		alias := newJoinAlias(f, defs.TableName(), chain)
		aliases = append(aliases, alias)
		parent = current

		switch {
		case f.ForeignKey() != nil:
			current = f.ForeignKey()
		case f.OneToOne() != nil:
			if through := f.OneToOne().Through(); through != nil {
				current = through
			} else {
				current = f.OneToOne().Model()
			}
		case f.ManyToMany() != nil:
			current = f.ManyToMany().Through()
		default:
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q is not a relation", part)
		}

		if current == nil {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q has no related model", part)
		}
		isRelated = true
	}

	return current, parent, field, chain, aliases, isRelated, nil
}

type queryInfo struct {
	db          *sql.DB
	dbx         *sqlx.DB
	sqlxDriver  string
	tableName   string
	definitions attrs.Definitions
	primary     attrs.Field
	fields      []attrs.Field
}

func getBaseQueryInfo(obj attrs.Definer) (*queryInfo, error) {
	var fieldDefs = obj.FieldDefs()
	var primary = fieldDefs.Primary()
	var tableName = fieldDefs.TableName()
	if tableName == "" {
		return nil, ErrNoTableName
	}

	return &queryInfo{
		definitions: fieldDefs,
		tableName:   tableName,
		primary:     primary,
		fields:      fieldDefs.Fields(),
	}, nil
}

func getQueryInfo(obj attrs.Definer, dbKey string) (*queryInfo, error) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		dbKey,
	)
	if db == nil {
		return nil, ErrNoDatabase
	}

	var sqlxDriver = sqlxDriverName(db)
	if sqlxDriver == "" {
		return nil, ErrUnknownDriver
	}

	var dbx = sqlx.NewDb(db, sqlxDriver)

	var queryInfo, err = getBaseQueryInfo(obj)
	if err != nil {
		return nil, err
	}

	queryInfo.db = db
	queryInfo.dbx = dbx
	queryInfo.sqlxDriver = sqlxDriver
	return queryInfo, nil
}
