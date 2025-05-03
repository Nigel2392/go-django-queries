package internal

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
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

var drivers = make(map[reflect.Type]driverData)

type driverData struct {
	name              string
	supportsReturning SupportsReturning
}

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

func SqlxDriverName(db *sql.DB) string {
	var driver = reflect.TypeOf(db.Driver())
	if driver == nil {
		return ""
	}
	if data, ok := drivers[driver]; ok {
		return data.name
	}
	return ""
}

func DBSupportsReturning(db *sql.DB) SupportsReturning {
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

func NewDefiner[T attrs.Definer]() T {
	return NewObjectFromIface(*new(T)).(T)
}

func NewObjectFromIface(obj attrs.Definer) attrs.Definer {
	var objTyp = reflect.TypeOf(obj)
	if objTyp.Kind() != reflect.Ptr {
		panic("newObjectFromIface: objTyp is not a pointer")
	}
	return reflect.New(objTyp.Elem()).Interface().(attrs.Definer)
}

// safer alias generator
func NewJoinAlias(field attrs.Field, tableName string, chain []string) string {
	var l = len(chain)
	return fmt.Sprintf("%s_%s_%d", field.ColumnName(), tableName, l-1)
	//	if l > 1 {
	//}
	//return fmt.Sprintf("%s_%s", field.ColumnName(), tableName)
}

func WalkFields(
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
		alias := NewJoinAlias(f, defs.TableName(), chain)
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

type RootFieldMeta struct {
	Root *FieldMeta
	Last *FieldMeta
}

type FieldMeta struct {
	Parent *FieldMeta
	Child  *FieldMeta
	Object attrs.Definer
	Field  attrs.Field
}

func (m *FieldMeta) String() string {
	var root = m
	for root.Parent != nil {
		root = root.Parent
	}
	var b = strings.Builder{}
	for root != nil {
		if root.Field != nil {
			b.WriteString(root.Field.Name())
		}
		if root.Child != nil {
			b.WriteString(".")
		}
		root = root.Child
	}
	return b.String()
}

func WalkFieldPath(m attrs.Definer, path string) (*RootFieldMeta, error) {
	var parts = strings.Split(path, ".")
	var root = &RootFieldMeta{
		Root: &FieldMeta{
			Object: m,
		},
	}
	var meta = root.Root
	for i, part := range parts {
		var defs = meta.Object.FieldDefs()
		var f, ok = defs.Field(part)
		if !ok {
			return nil, fmt.Errorf("field %q not found in %T", part, meta.Object)
		}

		meta.Field = f

		if i == len(parts)-1 {
			root.Last = meta
			break
		}

		var obj attrs.Definer
		switch {
		case f.ForeignKey() != nil:
			obj = f.ForeignKey()
		case f.OneToOne() != nil:
			if through := f.OneToOne().Through(); through != nil {
				obj = through
			} else {
				obj = f.OneToOne().Model()
			}
		case f.ManyToMany() != nil:
			obj = f.ManyToMany().Through()
		default:
			return nil, fmt.Errorf("field %q is not a relation", part)
		}

		var child = &FieldMeta{
			Object: obj,
			Parent: meta,
		}
		meta.Child = child
		meta = child
	}

	return root, nil
}

type QueryInfo struct {
	DB          *sql.DB
	DBX         interface{ Rebind(string) string }
	SqlxDriver  string
	TableName   string
	Definitions attrs.Definitions
	Primary     attrs.Field
	Fields      []attrs.Field
}

func GetBaseQueryInfo(obj attrs.Definer) (*QueryInfo, error) {
	var fieldDefs = obj.FieldDefs()
	var primary = fieldDefs.Primary()
	var tableName = fieldDefs.TableName()
	if tableName == "" {
		return nil, query_errors.ErrNoTableName
	}

	return &QueryInfo{
		Definitions: fieldDefs,
		TableName:   tableName,
		Primary:     primary,
		Fields:      fieldDefs.Fields(),
	}, nil
}

func GetQueryInfo(obj attrs.Definer, dbKey string) (*QueryInfo, error) {
	var db = django.ConfigGet[*sql.DB](
		django.Global.Settings,
		dbKey,
	)
	if db == nil {
		return nil, query_errors.ErrNoDatabase
	}

	var sqlxDriver = SqlxDriverName(db)
	if sqlxDriver == "" {
		return nil, query_errors.ErrUnknownDriver
	}

	var dbx = sqlx.NewDb(db, sqlxDriver)

	var queryInfo, err = GetBaseQueryInfo(obj)
	if err != nil {
		return nil, err
	}

	queryInfo.DB = db
	queryInfo.DBX = dbx
	queryInfo.SqlxDriver = sqlxDriver
	return queryInfo, nil
}
