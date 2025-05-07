package internal

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
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

const (
	CACHE_TRAVERSAL_RESULTS = false
)

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
	if field == nil {
		return tableName
	}
	var l = len(chain)
	return fmt.Sprintf("%s_%s_%d", field.ColumnName(), tableName, l-1)
	//	if l > 1 {
	//}
	//return fmt.Sprintf("%s_%s", field.ColumnName(), tableName)
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

		var rel = f.Rel()
		if rel == nil {
			return nil, nil, nil, nil, nil, false, fmt.Errorf("field %q is not a relation", part)
		}

		current = rel.Model()
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

type PathMetaChain []*PathMeta

func (c PathMetaChain) First() *PathMeta {
	if len(c) == 0 {
		return nil
	}
	return c[0]
}

func (c PathMetaChain) Last() *PathMeta {
	if len(c) == 0 {
		return nil
	}
	return c[len(c)-1]
}

type PathMeta struct {
	idx         int
	root        PathMetaChain
	Object      attrs.Definer
	Definitions attrs.Definitions
	Field       attrs.Field
	Relation    attrs.Relation
	TableAlias  string
}

func (m *PathMeta) String() string {
	var sb strings.Builder
	for i, part := range m.root[:m.idx+1] {
		if i > 0 {
			sb.WriteString(".")
		}
		sb.WriteString(part.Field.Name())
	}
	return sb.String()
}

func pathMetaTableAlias(m *PathMeta) string {
	if len(m.root) == 1 {
		return m.Definitions.TableName()
	}
	var c = m.CutAt()
	var s = make([]string, len(c))
	for i, part := range c {
		s[i] = part.Field.Name()
	}

	var field attrs.Field
	if m.idx > 0 {
		field = m.root[m.idx-1].Field
	}

	return NewJoinAlias(field, m.Definitions.TableName(), s)
}

func (m *PathMeta) Parent() *PathMeta {
	if m.idx == 0 {
		return nil
	}
	return m.root[m.idx-1]
}

func (m *PathMeta) Child() *PathMeta {
	if m.idx >= len(m.root)-1 {
		return nil
	}
	return m.root[m.idx+1]
}

func (m *PathMeta) CutAt() []*PathMeta {
	return slices.Clone(m.root)[:m.idx+1]
}

var walkFieldPathsCache = make(map[string]PathMetaChain)

func WalkFieldPath(m attrs.Definer, path string) (PathMetaChain, error) {
	var cacheKey = fmt.Sprintf("%T.%s", m, path)
	if CACHE_TRAVERSAL_RESULTS {
		if result, ok := walkFieldPathsCache[cacheKey]; ok {
			return result, nil
		}
	}

	var parts = strings.Split(path, ".")
	var root = make(PathMetaChain, len(parts))
	var current = m
	for i, part := range parts {
		var defs = current.FieldDefs()
		var meta = &PathMeta{
			Object:      current,
			Definitions: defs,
		}

		var f, ok = defs.Field(part)
		if !ok {
			return nil, fmt.Errorf("field %q not found in %T", part, meta.Object)
		}

		relation, ok := attrs.GetRelationMeta(meta.Object, part)
		if !ok && i != len(parts)-1 {
			return nil, fmt.Errorf("field %q is not a relation in %T", part, meta.Object)
		}

		meta.idx = i
		meta.root = root
		meta.Field = f
		meta.Relation = relation

		root[i] = meta

		meta.TableAlias = pathMetaTableAlias(meta)

		if i == len(parts)-1 {
			break
		}

		// This is required to avoid FieldNotFound errors - some objects might cache
		// their field definitions, meaning any dynamic changes to the field will not be reflected
		// in the field definitions. This is a workaround to avoid that issue.
		var newTyp = reflect.TypeOf(relation.Model())
		var newObj = reflect.New(newTyp.Elem())
		current = newObj.Interface().(attrs.Definer)
	}

	if CACHE_TRAVERSAL_RESULTS {
		walkFieldPathsCache[cacheKey] = root
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
