package mysql

import (
	"database/sql"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Nigel2392/go-django-queries/src/drivers"
	"github.com/Nigel2392/go-django-queries/src/migrator"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

const (
	int16_max = 1 << 15
	int32_max = 1 << 31
)

// MYSQL TYPES
func init() {
	// register kinds
	migrator.RegisterColumnKind(&drivers.DriverMySQL{}, []reflect.Kind{reflect.String}, Type__string)
	migrator.RegisterColumnKind(&drivers.DriverMySQL{}, []reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}, Type__int)
	migrator.RegisterColumnKind(&drivers.DriverMySQL{}, []reflect.Kind{reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64}, Type__int)
	migrator.RegisterColumnKind(&drivers.DriverMySQL{}, []reflect.Kind{reflect.Float32, reflect.Float64}, Type__float)
	migrator.RegisterColumnKind(&drivers.DriverMySQL{}, []reflect.Kind{reflect.Bool}, Type__bool)

	// register types
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullString{}, Type__string)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullFloat64{}, Type__int)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullInt64{}, Type__int)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullInt32{}, Type__int)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullInt16{}, Type__int)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullBool{}, Type__bool)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullByte{}, Type__int)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, sql.NullTime{}, Type__datetime)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, time.Time{}, Type__datetime)
	migrator.RegisterColumnType(&drivers.DriverMySQL{}, []byte{}, Type__string)
}

func Type__string(f attrs.Field) string {
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

func Type__float(f attrs.Field) string {
	switch f.Type().Kind() {
	case reflect.Float32:
		return "FLOAT"
	case reflect.Float64:
		return "DOUBLE"
	}
	return "DOUBLE"
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
