package sqlite

import (
	"database/sql"
	"reflect"
	"time"

	"github.com/Nigel2392/go-django-queries/src/migrator"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/mattn/go-sqlite3"
)

const (
	int16_max = 1 << 15
	int32_max = 1 << 31
)

// SQLITE TYPES
func init() {
	// register kinds
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.String}, type__string)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}, type__int)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Float32, reflect.Float64}, type__float)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Bool}, type__bool)

	// register types
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullString{}, type__string)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullFloat64{}, type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt64{}, type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt32{}, type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt16{}, type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullBool{}, type__bool)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullByte{}, type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullTime{}, type__datetime)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, time.Time{}, type__datetime)
}

func type__string(f attrs.Field) string {
	return "TEXT"
}

func type__float(f attrs.Field) string {
	return "REAL"
}

func type__int(f attrs.Field) string {
	var atts = f.Attrs()
	var max float64
	var max_val = atts[attrs.AttrMaxValueKey]
	if max_val != nil {
		max = max_val.(float64)
	}

	switch f.Type().Kind() {
	case reflect.Int8:
		return "SMALLINT"
	case reflect.Int16:
		return "INT"
	case reflect.Int32, reflect.Int:
		if max != 0 && max <= int32_max {
			return "INT"
		}
		return "BIGINT"
	case reflect.Int64:
		if max != 0 && max <= int32_max {
			return "INT"
		}
		return "BIGINT"
	}

	return "BIGINT"
}

func type__bool(f attrs.Field) string {
	return "BOOLEAN"
}

func type__datetime(f attrs.Field) string {
	return "TIMESTAMP"
}
