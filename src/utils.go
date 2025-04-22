package queries

import (
	"database/sql"
	"database/sql/driver"
	"reflect"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/go-sql-driver/mysql"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
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
func RegisterDriver(driver driver.Driver, database string) {
	drivers[reflect.TypeOf(driver)] = database
}

var drivers = make(map[reflect.Type]string)

func init() {
	RegisterDriver(&mysql.MySQLDriver{}, "mysql")
	RegisterDriver(&sqlite3.SQLiteDriver{}, "sqlite3")
	RegisterDriver(&pg_stdlib.Driver{}, "postgres")
}

func sqlxDriverName(db *sql.DB) string {
	var driver = reflect.TypeOf(db.Driver())
	if driver == nil {
		return ""
	}
	if name, ok := drivers[driver]; ok {
		return name
	}
	return ""
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

type queryInfo[T attrs.Definer] struct {
	db          *sql.DB
	dbx         *sqlx.DB
	sqlxDriver  string
	tableName   string
	definitions attrs.Definitions
	primary     attrs.Field
	fields      []attrs.Field
}

func getQueryInfo[T attrs.Definer](obj T) (*queryInfo[T], error) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		django.APPVAR_DATABASE,
	)
	if db == nil {
		return nil, ErrNoDatabase
	}

	var sqlxDriver = sqlxDriverName(db)
	if sqlxDriver == "" {
		return nil, ErrUnknownDriver
	}

	var fieldDefs = obj.FieldDefs()
	var primary = fieldDefs.Primary()
	var tableName = fieldDefs.TableName()
	if tableName == "" {
		return nil, ErrNoTableName
	}

	var dbx = sqlx.NewDb(db, sqlxDriver)
	return &queryInfo[T]{
		db:          db,
		dbx:         dbx,
		sqlxDriver:  sqlxDriver,
		definitions: fieldDefs,
		tableName:   tableName,
		primary:     primary,
		fields:      fieldDefs.Fields(),
	}, nil
}
