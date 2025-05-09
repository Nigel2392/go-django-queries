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
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.String}, Type__string)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}, Type__int)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64}, Type__int)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Float32, reflect.Float64}, Type__float)
	migrator.RegisterColumnKind(&sqlite3.SQLiteDriver{}, []reflect.Kind{reflect.Bool}, Type__bool)

	// register types
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullString{}, Type__string)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullFloat64{}, Type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt64{}, Type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt32{}, Type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullInt16{}, Type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullBool{}, Type__bool)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullByte{}, Type__int)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, sql.NullTime{}, Type__datetime)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, time.Time{}, Type__datetime)
	migrator.RegisterColumnType(&sqlite3.SQLiteDriver{}, []byte{}, Type__string)
}

func Type__string(f attrs.Field) string {
	return "TEXT"
}

func Type__float(f attrs.Field) string {
	return "REAL"
}

func Type__int(f attrs.Field) string {
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

func Type__bool(f attrs.Field) string {
	return "BOOLEAN"
}

func Type__datetime(f attrs.Field) string {
	return "TIMESTAMP"
}
