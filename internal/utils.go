package internal

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/alias"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/jmoiron/sqlx"

	_ "unsafe"
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

const (
	CACHE_TRAVERSAL_RESULTS = false
)

//go:linkname getRelatedName github.com/Nigel2392/go-django/src/core/attrs.getRelatedName
func getRelatedName(f attrs.Field, default_ string) string

func GetRelatedName(f attrs.Field, default_ string) string {
	if isReverser, ok := f.(interface{ IsReverse() bool }); ok && isReverser.IsReverse() {
		return getRelatedName(f, default_)
	}

	return f.Name()
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

type walkFieldsResult struct {
	definer   attrs.Definer
	parent    attrs.Definer
	field     attrs.Field
	chain     []string
	aliases   []string
	isRelated bool
}

var walkFieldsCache = make(map[string]walkFieldsResult)

func WalkFields(
	m attrs.Definer,
	column string,
	aliasGen *alias.Generator,
) (
	definer attrs.Definer,
	parent attrs.Definer,
	f attrs.Field,
	chain []string,
	aliases []string,
	isRelated bool,
	err error,
) {

	var cacheKey = fmt.Sprintf("%T.%s", m, column)
	if CACHE_TRAVERSAL_RESULTS {
		if result, ok := walkFieldsCache[cacheKey]; ok {
			return result.definer, result.parent, result.field, result.chain, result.aliases, result.isRelated, nil
		}
	}

	var parts = strings.Split(column, ".")
	var current = m
	var field attrs.Field

	chain = make([]string, 0, len(parts)-1)
	aliases = make([]string, 0, len(parts)-1)

	defs := current.FieldDefs()
	for i, part := range parts {
		f, ok := defs.Field(part)
		if !ok {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q not found in %T", part, current)
		}
		field = f

		if i == len(parts)-1 {
			break
		}

		var rel = f.Rel()
		if rel == nil {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q is not a relation", part)
		}

		parent = current
		current = rel.Model()
		defs = current.FieldDefs()
		chain = append(chain, part)
		aliases = append(aliases, aliasGen.GetTableAlias(
			defs.TableName(), strings.Join(chain, "."),
		))

		if current == nil {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q has no related model", part)
		}

		isRelated = true
	}

	if CACHE_TRAVERSAL_RESULTS {
		walkFieldsCache[cacheKey] = walkFieldsResult{
			definer:   current,
			parent:    parent,
			field:     field,
			chain:     chain,
			aliases:   aliases,
			isRelated: isRelated,
		}
	}

	return current, parent, field, chain, aliases, isRelated, nil
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
