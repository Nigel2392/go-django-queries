package queries

import (
	"database/sql"
	"reflect"

	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/go-sql-driver/mysql"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
)

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

func sqlxDriverName(db *sql.DB) string {
	switch db.Driver().(type) {
	case *mysql.MySQLDriver:
		return "mysql"
	case *sqlite3.SQLiteDriver:
		return "sqlite3"
	case *pg_stdlib.Driver:
		return "postgres"
	default:
		return ""
	}
}

type queryInfo[T attrs.Definer] struct {
	db          *sql.DB
	dbx         *sqlx.DB
	sqlxDriver  string
	tableName   string
	definitions attrs.Definitions
	primary     attrs.Field
	fields      []attrs.Field
	fields_map  map[string]attrs.Field
}

func (qi *queryInfo[T]) isValidField(field string) bool {
	if field == "" {
		return false
	}
	if _, ok := qi.fields_map[field]; !ok {
		return false
	}
	return true
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

	var fields_map = make(map[string]attrs.Field)
	var fields = fieldDefs.Fields()
	for _, field := range fieldDefs.Fields() {
		fields_map[field.ColumnName()] = field
	}

	var dbx = sqlx.NewDb(db, sqlxDriver)
	return &queryInfo[T]{
		db:          db,
		dbx:         dbx,
		sqlxDriver:  sqlxDriver,
		definitions: fieldDefs,
		tableName:   tableName,
		primary:     primary,
		fields:      fields,
		fields_map:  fields_map,
	}, nil
}
