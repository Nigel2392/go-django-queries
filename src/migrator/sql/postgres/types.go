package postgres

import (
	"database/sql"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Nigel2392/go-django-queries/src/migrator"
	"github.com/Nigel2392/go-django/src/core/attrs"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"
)

const (
	int16_max = 1 << 15
	int32_max = 1 << 31
)

// POSTGRES TYPES
func init() {
	// register kinds
	migrator.RegisterColumnKind(&pg_stdlib.Driver{}, []reflect.Kind{reflect.String}, type__string)
	migrator.RegisterColumnKind(&pg_stdlib.Driver{}, []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}, type__int)
	migrator.RegisterColumnKind(&pg_stdlib.Driver{}, []reflect.Kind{reflect.Float32, reflect.Float64}, type__float)
	migrator.RegisterColumnKind(&pg_stdlib.Driver{}, []reflect.Kind{reflect.Bool}, type__bool)

	// register types
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullString{}, type__string)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullFloat64{}, type__int)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullInt64{}, type__int)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullInt32{}, type__int)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullInt16{}, type__int)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullBool{}, type__bool)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullByte{}, type__int)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, sql.NullTime{}, type__datetime)
	migrator.RegisterColumnType(&pg_stdlib.Driver{}, time.Time{}, type__datetime)
}

func type__string(f attrs.Field) string {
	var atts = f.Attrs()
	var max int64
	var max_len = atts[attrs.AttrMaxLengthKey]
	if max_len != nil {
		max = max_len.(int64)
	}

	if max == 0 {
		return "TEXT"
	}

	var sb = new(strings.Builder)
	sb.WriteString("VARCHAR(")
	sb.WriteString(strconv.FormatInt(max, 10))
	sb.WriteString(")")
	return sb.String()
}

func type__float(f attrs.Field) string {
	switch f.Type().Kind() {
	case reflect.Float32:
		return "REAL"
	case reflect.Float64:
		return "DOUBLE PRECISION"
	}
	return "DOUBLE PRECISION"
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
		return "INTEGER"
	case reflect.Int32, reflect.Int:
		if max != 0 && max <= int32_max {
			return "INTEGER"
		}
		return "BIGINT"
	case reflect.Int64:
		if max != 0 && max <= int32_max {
			return "INTEGER"
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
