package queries

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/alias"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/pkg/errors"

	_ "unsafe"
)

// -----------------------------------------------------------------------------
// QuerySet
// -----------------------------------------------------------------------------

const MAX_GET_RESULTS = 21

var QUERYSET_USE_CACHE_DEFAULT = true

type Table struct {
	Name  string
	Alias string
}

type JoinDef struct {
	Table      Table
	TypeJoin   string
	ConditionA string
	Logic      string
	ConditionB string
}

type FieldInfo struct {
	SourceField attrs.Field
	Model       attrs.Definer
	RelType     attrs.RelationType
	Table       Table
	Chain       []string
	Fields      []attrs.Field
}

type OrderBy struct {
	Table Table
	Field string
	Alias string
	Desc  bool
}

func (f *FieldInfo) WriteFields(sb *strings.Builder, inf *expr.ExpressionInfo) []any {
	var args = make([]any, 0, len(f.Fields))
	var written bool
	for _, field := range f.Fields {
		if written {
			sb.WriteString(", ")
		}

		var a, _, ok = f.WriteField(sb, inf, field, false)
		written = ok || written
		if !ok {
			continue
		}

		args = append(args, a...)
	}

	return args
}

func (f *FieldInfo) WriteUpdateFields(sb *strings.Builder, inf *expr.ExpressionInfo) []any {
	var args = make([]any, 0, len(f.Fields))
	var written bool
	for _, field := range f.Fields {
		if written {
			sb.WriteString(", ")
		}

		var a, _, ok = f.WriteField(sb, inf, field, true)
		written = ok || written
		if !ok {
			continue
		}

		args = append(args, a...)
	}

	return args
}

func (f *FieldInfo) WriteField(sb *strings.Builder, inf *expr.ExpressionInfo, field attrs.Field, forUpdate bool) (args []any, isSQL, written bool) {
	var fieldAlias string
	if ve, ok := field.(AliasField); ok && !forUpdate {
		fieldAlias = ve.Alias()
	}

	var tableAlias string
	if f.Table.Alias == "" {
		tableAlias = f.Table.Name
	} else {
		tableAlias = f.Table.Alias
	}

	if ve, ok := field.(VirtualField); ok && inf.Model != nil {
		var sql, a = ve.SQL(inf)
		if sql == "" {
			return nil, true, false
		}

		sb.WriteString(sql)

		if fieldAlias != "" && !forUpdate {
			sb.WriteString(" AS ")
			sb.WriteString(inf.Quote)
			sb.WriteString(inf.AliasGen.GetFieldAlias(
				tableAlias, fieldAlias,
			))
			sb.WriteString(inf.Quote)
		}

		args = append(args, a...)
		return args, true, true
	}

	if !forUpdate {
		sb.WriteString(inf.Quote)

		if f.Table.Alias == "" {
			sb.WriteString(f.Table.Name)
		} else {
			sb.WriteString(f.Table.Alias)
		}

		sb.WriteString(inf.Quote)
		sb.WriteString(".")
	}

	sb.WriteString(inf.Quote)
	sb.WriteString(field.ColumnName())
	sb.WriteString(inf.Quote)

	if forUpdate {
		sb.WriteString(" = ?")
	}

	return []any{}, false, true
}

type QuerySetInternals struct {
	Annotations map[string]*queryField[any]
	Fields      []FieldInfo
	Where       []expr.LogicalExpression
	Having      []expr.LogicalExpression
	Joins       []JoinDef
	GroupBy     []FieldInfo
	OrderBy     []OrderBy
	Limit       int
	Offset      int
	ForUpdate   bool
	Distinct    bool

	joinsMap map[string]bool
}

// QuerySet is a struct that represents a query set in the database.
//
// It contains methods to filter, order, and limit the results of a query.
//
// It is used to build and execute queries against the database.
//
// Every method on the queryset returns a new queryset, so that the original queryset is not modified.
//
// It also has a chainable api, so that you can easily build complex queries by chaining methods together.
//
// Queries are built internally with the help of the QueryCompiler interface, which is responsible for generating the SQL queries for the database.
type QuerySet[T attrs.Definer] struct {
	queryInfo    *internal.QueryInfo
	internals    *QuerySetInternals
	model        attrs.Definer
	compiler     QueryCompiler
	AliasGen     *alias.Generator
	explicitSave bool
	useCache     bool
	latestQuery  QueryInfo
	cached       any
}

// QuerySet is a struct that represents a query set in the database.
//
// It contains methods to filter, order, and limit the results of a query.
//
// It is used to build and execute queries against the database.
//
// Every method on the queryset returns a new queryset, so that the original queryset is not modified.
//
// It also has a chainable api, so that you can easily build complex queries by chaining methods together.
//
// Queries are built internally with the help of the QueryCompiler interface, which is responsible for generating the SQL queries for the database.
type GenericQuerySet = QuerySet[attrs.Definer]

// GetQuerySet creates a new QuerySet for the given model.
//
// If the model implements the QuerySetDefiner interface,
// it will use the GetQuerySet method to get the initial QuerySet.
//
// A model should use Objects[T](model) to get the default QuerySet inside of it's
// GetQuerySet method. If not, it will recursively call itself.
//
// See [Objects] for more details.
func GetQuerySet[T attrs.Definer](model attrs.Definer) *QuerySet[T] {
	if m, ok := model.(QuerySetDefiner); ok {
		var qs = m.GetQuerySet()
		qs = qs.Clone()
		return ChangeObjectsType[attrs.Definer, T](qs)
	}

	return Objects[T](model)
}

// Objects creates a new QuerySet for the given model.
//
// This function should only be called in a model's GetQuerySet method.
//
// In other places the [GetQuerySet] function should be used instead.
//
// It panics if:
// - the model is nil
// - the base query info cannot be retrieved
//
// It returns a pointer to a new QuerySet.
//
// The model must implement the Definer interface.
func Objects[T attrs.Definer](model attrs.Definer, database ...string) *QuerySet[T] {

	if model == nil {
		panic("QuerySet: model is nil")
	}

	var defaultDb = django.APPVAR_DATABASE
	if len(database) > 1 {
		panic("QuerySet: too many databases provided")
	}

	// If the model implements the QuerySetDatabaseDefiner interface,
	// it will use the QuerySetDatabase method to get the default database.
	// Function arguments still take precedence however.
	if m, ok := model.(QuerySetDatabaseDefiner); ok && len(database) == 0 {
		defaultDb = m.QuerySetDatabase()
	}

	// Arguments take precedence over the default database
	if len(database) == 1 {
		defaultDb = database[0]
	}

	var queryInfo, err = internal.GetBaseQueryInfo(model)
	if err != nil {
		panic(fmt.Errorf("QuerySet: %w", err))
	}

	if queryInfo == nil {
		panic("QuerySet: queryInfo is nil")
	}

	var qs = &QuerySet[T]{
		model:     model,
		queryInfo: queryInfo,
		AliasGen:  alias.NewGenerator(),
		internals: &QuerySetInternals{
			Annotations: make(map[string]*queryField[any]),
			Where:       make([]expr.LogicalExpression, 0),
			Having:      make([]expr.LogicalExpression, 0),
			Joins:       make([]JoinDef, 0),
			GroupBy:     make([]FieldInfo, 0),
			OrderBy:     make([]OrderBy, 0),
			Limit:       1000,
			Offset:      0,
		},

		// enable queryset caching by default
		// this can result in race conditions in some rare edge cases
		// but is generally safe to use
		useCache: QUERYSET_USE_CACHE_DEFAULT,
	}
	qs.compiler = Compiler(model, defaultDb)
	return qs
}

func ChangeObjectsType[OldT, NewT attrs.Definer](qs *QuerySet[OldT]) *QuerySet[NewT] {
	return &QuerySet[NewT]{
		AliasGen:     qs.AliasGen,
		queryInfo:    qs.queryInfo,
		model:        qs.model,
		compiler:     qs.compiler,
		explicitSave: qs.explicitSave,
		useCache:     qs.useCache,
		cached:       qs.cached,
		internals:    qs.internals,
	}
}

// Return the underlying database which the compiler is using.
func (qs *QuerySet[T]) DB() DB {
	return qs.compiler.DB()
}

// Return the model which the queryset is for.
func (qs *QuerySet[T]) Model() attrs.Definer {
	return qs.model
}

// Return the compiler which the queryset is using.
func (qs *QuerySet[T]) Compiler() QueryCompiler {
	return qs.compiler
}

// LatestQuery returns the latest query that was executed on the queryset.
func (qs *QuerySet[T]) LatestQuery() QueryInfo {
	return qs.latestQuery
}

// StartTransaction starts a transaction on the underlying database.
//
// It returns a transaction object which can be used to commit or rollback the transaction.
func (qs *QuerySet[T]) StartTransaction(ctx context.Context) (Transaction, error) {
	return qs.compiler.StartTransaction(ctx)
}

// Clone creates a new QuerySet with the same parameters as the original one.
//
// It is used to create a new QuerySet with the same parameters as the original one, so that the original one is not modified.
//
// It is a shallow clone, underlying values like `*queries.Expr` are not cloned and have built- in immutability.
func (qs *QuerySet[T]) Clone() *QuerySet[T] {
	return &QuerySet[T]{
		model:     qs.model,
		queryInfo: qs.queryInfo,
		AliasGen:  qs.AliasGen.Clone(),
		internals: &QuerySetInternals{
			Annotations: maps.Clone(qs.internals.Annotations),
			Fields:      slices.Clone(qs.internals.Fields),
			Where:       slices.Clone(qs.internals.Where),
			Having:      slices.Clone(qs.internals.Having),
			Joins:       slices.Clone(qs.internals.Joins),
			GroupBy:     slices.Clone(qs.internals.GroupBy),
			OrderBy:     slices.Clone(qs.internals.OrderBy),
			Limit:       qs.internals.Limit,
			Offset:      qs.internals.Offset,
			ForUpdate:   qs.internals.ForUpdate,
			Distinct:    qs.internals.Distinct,
			joinsMap:    maps.Clone(qs.internals.joinsMap),
		},
		explicitSave: qs.explicitSave,
		useCache:     qs.useCache,
		compiler:     qs.compiler,
	}
}

// Prefix sets the prefix for the alias generator
func (qs *QuerySet[T]) Prefix(prefix string) *QuerySet[T] {
	qs.AliasGen.Prefix = prefix
	return qs
}

// Return the string representation of the QuerySet.
func (w *QuerySet[T]) String() string {
	var sb = strings.Builder{}
	sb.WriteString("QuerySet{")
	sb.WriteString("model: ")
	sb.WriteString(fmt.Sprintf("%T", w.model))
	sb.WriteString(", fields: [")
	var written bool
	for _, field := range w.internals.Fields {
		for _, f := range field.Fields {
			if written {
				sb.WriteString(", ")
			}

			if len(field.Chain) > 0 {
				sb.WriteString(strings.Join(
					field.Chain, ".",
				))
				sb.WriteString(".")
			}

			sb.WriteString(f.Name())
			written = true
		}
	}
	sb.WriteString("]}")
	return sb.String()
}

// Return a detailed string representation of the QuerySet.
func (qs *QuerySet[T]) GoString() string {
	var sb = strings.Builder{}
	sb.WriteString("QuerySet{")
	sb.WriteString("\n\tmodel: ")
	sb.WriteString(fmt.Sprintf("%T", qs.model))
	sb.WriteString(",\n\tfields: [")
	var written bool
	for _, field := range qs.internals.Fields {
		for _, f := range field.Fields {
			if written {
				sb.WriteString(", ")
			}

			sb.WriteString("\n\t\t")
			if len(field.Chain) > 0 {
				sb.WriteString(strings.Join(
					field.Chain, ".",
				))
				sb.WriteString(".")
			}

			sb.WriteString(f.Name())
			written = true
		}
	}
	sb.WriteString("\n\t],")

	if len(qs.internals.Where) > 0 {
		sb.WriteString("\n\twhere: [")
		for _, expr := range qs.internals.Where {
			fmt.Fprintf(&sb, "\n\t\t%T: %#v", expr, expr)
		}
		sb.WriteString("\n\t],")
	}

	if len(qs.internals.Joins) > 0 {
		sb.WriteString("\n\tjoins: [")
		for _, join := range qs.internals.Joins {
			sb.WriteString("\n\t\t")
			sb.WriteString(join.TypeJoin)
			sb.WriteString(" ")
			if join.Table.Alias == "" {
				sb.WriteString(join.Table.Name)
			} else {
				sb.WriteString(join.Table.Alias)
			}
			sb.WriteString(" ON ")
			sb.WriteString(join.ConditionA)
			sb.WriteString(" ")
			sb.WriteString(join.Logic)
			sb.WriteString(" ")
			sb.WriteString(join.ConditionB)
		}
		sb.WriteString("\n\t],")
	}

	sb.WriteString("\n}")
	return sb.String()
}

// The core function used to convert a list of fields to a list of FieldInfo.
//
// This function will make sure to map each provided field name to a model field.
//
// Relations are also respected, joins are automatically added to the query.
func (qs *QuerySet[T]) unpackFields(fields ...string) (infos []FieldInfo, hasRelated bool) {
	infos = make([]FieldInfo, 0, len(qs.internals.Fields))
	var info = FieldInfo{
		Table: Table{
			Name: qs.queryInfo.TableName,
		},
		Fields: make([]attrs.Field, 0),
	}

	if len(fields) == 0 || len(fields) == 1 && fields[0] == "*" {
		fields = make([]string, 0, len(qs.queryInfo.Fields))
		for _, field := range qs.queryInfo.Fields {
			fields = append(fields, field.Name())
		}
	}

	for _, selectedField := range fields {
		var current, parent, field, chain, aliases, isRelated, err = internal.WalkFields(
			qs.model, selectedField, qs.AliasGen,
		)
		if err != nil {
			field, ok := qs.internals.Annotations[selectedField]
			if ok {
				infos = append(infos, FieldInfo{
					Table: Table{
						Name: qs.queryInfo.TableName,
					},
					Fields: []attrs.Field{field},
				})
				continue
			}

			panic(err)
		}

		// The field might be a relation
		var rel = field.Rel()

		if (rel != nil) || (len(chain) > 0 || isRelated) {
			var relType attrs.RelationType
			if rel != nil {
				relType = rel.Type()
			} else {
				var parentMeta = attrs.GetModelMeta(parent)
				var parentDefs = parentMeta.Definitions()
				var parentField, ok = parentDefs.Field(chain[len(chain)-1])
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", chain[len(chain)-1], parent))
				}
				relType = parentField.Rel().Type()
			}

			var relDefs = current.FieldDefs()
			var tableName = relDefs.TableName()
			infos = append(infos, FieldInfo{
				SourceField: field,
				Model:       current,
				RelType:     relType,
				Table: Table{
					Name:  tableName,
					Alias: aliases[len(aliases)-1],
				},
				Fields: relDefs.Fields(),
				Chain:  chain,
			})

			continue
		}

		info.Fields = append(info.Fields, field)
	}

	if len(info.Fields) > 0 {
		infos = append(infos, info)
	}

	return infos, hasRelated
}

func (qs *QuerySet[T]) attrFields(obj attrs.Definer) []attrs.Field {
	var defs = obj.FieldDefs()
	var fields []attrs.Field
	if len(qs.internals.Fields) > 0 {
		fields = make([]attrs.Field, 0, len(qs.internals.Fields))
		for _, info := range qs.internals.Fields {
			for _, field := range info.Fields {
				var f, ok = defs.Field(field.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", field.Name(), obj))
				}
				fields = append(fields, f)
			}
		}
	} else {
		var all = defs.Fields()
		fields = make([]attrs.Field, 0, len(all))
		for _, field := range all {
			var val = field.GetValue()
			var rVal = reflect.ValueOf(val)
			if rVal.IsValid() && rVal.IsZero() {
				continue
			}
			fields = append(fields, field)
		}
	}
	return fields
}

func (qs *QuerySet[T]) addJoinForFK(foreignKey attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]FieldInfo, []JoinDef) {
	var target = foreignKey.Model()
	var relField = foreignKey.Field()
	// var relField attrs.Field
	var targetDefs = target.FieldDefs()
	var targetTable = targetDefs.TableName()
	// if relFieldGetter, ok := field.(RelatedField); ok {
	// relField = relFieldGetter.GetTargetField()
	// } else {
	// if relField == nil {
	// relField = targetDefs.Primary()
	// }

	var front, back = qs.compiler.Quote()

	var (
		condA_Alias = parentDefs.TableName()
		condB_Alias = targetTable
	)

	if len(aliases) == 1 {
		condB_Alias = aliases[0]
	} else if len(aliases) > 1 {
		condA_Alias = aliases[len(aliases)-2]
		condB_Alias = aliases[len(aliases)-1]
	}

	var condA = fmt.Sprintf(
		"%s%s%s.%s%s%s",
		front, condA_Alias, back,
		front, parentField.ColumnName(), back,
	)
	var condB = fmt.Sprintf(
		"%s%s%s.%s%s%s",
		front, condB_Alias, back,
		front, relField.ColumnName(), back,
	)

	var includedFields []attrs.Field
	if all {
		var fields = targetDefs.Fields()
		includedFields = make([]attrs.Field, 0, len(fields))
		for _, f := range fields {
			if !ForSelectAll(f) {
				continue
			}
			includedFields = append(includedFields, f)
		}
	} else {
		includedFields = []attrs.Field{field}
	}

	var info = FieldInfo{
		RelType:     foreignKey.Type(),
		SourceField: field,
		Table: Table{
			Name:  targetTable,
			Alias: aliases[len(aliases)-1],
		},
		Model:  target,
		Fields: includedFields,
		Chain:  chain,
	}

	var key = fmt.Sprintf("%s.%s", condA, condB)
	if _, ok := joinM[key]; ok {
		return []FieldInfo{info}, nil
	}

	joinM[key] = true
	var join = JoinDef{
		TypeJoin: "LEFT JOIN",
		Table: Table{
			Name:  targetTable,
			Alias: aliases[len(aliases)-1],
		},
		ConditionA: condA,
		Logic:      "=",
		ConditionB: condB,
	}

	return []FieldInfo{info}, []JoinDef{join}
}

func (qs *QuerySet[T]) addJoinForM2M(manyToMany attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]FieldInfo, []JoinDef) {
	var through = manyToMany.Through()

	if through == nil {
		panic(fmt.Errorf("manyToMany relation %T.%s does not have a through table", manyToMany.Model(), field.Name()))
	}

	// through model info
	var throughModel = through.Model()
	var throughDefs = throughModel.FieldDefs()
	var throughTable = throughDefs.TableName()

	sourceField, ok := throughDefs.Field(through.SourceField())
	if !ok {
		panic(fmt.Errorf("field %q not found in %T", through.SourceField(), throughModel))
	}
	targetField, ok := throughDefs.Field(through.TargetField())
	if !ok {
		panic(fmt.Errorf("field %q not found in %T", through.TargetField(), throughModel))
	}

	var target = manyToMany.Model()
	var targetDefs = target.FieldDefs()
	var targetTable = targetDefs.TableName()

	front, back := qs.compiler.Quote()
	var parentAlias string
	if len(aliases) > 1 {
		parentAlias = aliases[len(aliases)-2]
	} else {
		parentAlias = parentDefs.TableName()
	}
	alias := aliases[len(aliases)-1]
	aliasThrough := fmt.Sprintf("%s_through", alias)

	// JOIN through table
	join1 := JoinDef{
		TypeJoin: "LEFT JOIN",
		Table: Table{
			Name:  throughTable,
			Alias: aliasThrough,
		},
		ConditionA: fmt.Sprintf(
			"%s%s%s.%s%s%s",
			front, parentAlias, back,
			front, parentField.ColumnName(), back,
		),
		Logic: "=",
		ConditionB: fmt.Sprintf(
			"%s%s%s.%s%s%s",
			front, aliasThrough, back,
			front, sourceField.ColumnName(), back,
		),
	}

	// JOIN target table
	join2 := JoinDef{
		TypeJoin: "LEFT JOIN",
		Table: Table{
			Name:  targetTable,
			Alias: alias,
		},
		ConditionA: fmt.Sprintf(
			"%s%s%s.%s%s%s",
			front, aliasThrough, back,
			front, targetField.ColumnName(), back,
		),
		Logic: "=",
		ConditionB: fmt.Sprintf(
			"%s%s%s.%s%s%s",
			front, alias, back,
			front, targetDefs.Primary().ColumnName(), back,
		),
	}

	// Prevent duplicate joins
	joins := make([]JoinDef, 0, 2)
	if _, ok := joinM[join1.ConditionA+"."+join1.ConditionB]; !ok {
		joins = append(joins, join1)
		joinM[join1.ConditionA+"."+join1.ConditionB] = true
	}
	if _, ok := joinM[join2.ConditionA+"."+join2.ConditionB]; !ok {
		joins = append(joins, join2)
		joinM[join2.ConditionA+"."+join2.ConditionB] = true
	}

	includedFields := []attrs.Field{field}
	if all {
		includedFields = nil
		for _, f := range targetDefs.Fields() {
			if ForSelectAll(f) {
				includedFields = append(includedFields, f)
			}
		}
	}

	return []FieldInfo{{
		RelType:     manyToMany.Type(),
		SourceField: field,
		Model:       target,
		Table: Table{
			Name:  targetTable,
			Alias: alias,
		},
		Fields: includedFields,
		Chain:  chain,
	}}, joins

}

func (qs *QuerySet[T]) addJoinForO2O(oneToOne attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]FieldInfo, []JoinDef) {
	var through = oneToOne.Through()
	if through == nil {
		return qs.addJoinForFK(oneToOne, parentDefs, parentField, field, chain, aliases, all, joinM)
	}
	return qs.addJoinForM2M(oneToOne, parentDefs, parentField, field, chain, aliases, all, joinM)
}

// Select is used to select specific fields from the model.
//
// It takes a list of field names as arguments and returns a new QuerySet with the selected fields.
//
// If no fields are provided, it selects all fields from the model.
//
// If the first field is "*", it selects all fields from the model,
// extra fields (i.e. relations) can be provided thereafter - these will also be added to the selection.
//
// How to call Select:
//
// `Select("*")`
// `Select("Field1", "Field2")`
// `Select("Field1", "Field2", "Relation.*")`
// `Select("*", "Relation.*")`
// `Select("Relation.*")`
// `Select("*", "Relation.Field1", "Relation.Field2", "Relation.Nested.*")`
func (qs *QuerySet[T]) Select(fields ...any) *QuerySet[T] {
	qs = qs.Clone()

	qs.internals.Fields = make([]FieldInfo, 0)
	if qs.internals.joinsMap == nil {
		qs.internals.joinsMap = make(map[string]bool, len(qs.internals.Joins))
	}

	if len(fields) == 0 {
		fields = make([]any, 0, len(qs.queryInfo.Fields))
		for _, field := range qs.queryInfo.Fields {
			if ForSelectAll(field) {
				fields = append(fields, field.Name())
			}
		}
	} else if len(fields) > 0 && fields[0] == "*" {
		var f = make([]any, 0, len(qs.queryInfo.Fields)+(len(fields)-1))
		for _, field := range qs.queryInfo.Fields {
			if ForSelectAll(field) {
				f = append(f, field.Name())
			}
		}
		fields = append(f, fields[1:]...)
	}

	var exprMap = make(map[string]expr.NamedExpression, len(fields))
	for _, selectedFieldObj := range fields {

		var selectedField string
		switch v := selectedFieldObj.(type) {
		case string:
			selectedField = v
		case expr.NamedExpression:
			selectedField = v.FieldName()

			if selectedField == "" {
				panic(fmt.Errorf("Select: empty field name for %T", v))
			}

			exprMap[selectedField] = v
		default:
			panic(fmt.Errorf("Select: invalid field type %T, can be one of [string, NamedExpression]", v))
		}

		var allFields bool
		if strings.HasSuffix(strings.ToLower(selectedField), ".*") {
			selectedField = selectedField[:len(selectedField)-2]
			allFields = true
		}

		var current, parent, field, chain, aliases, isRelated, err = internal.WalkFields(
			qs.model, selectedField, qs.AliasGen,
		)
		if err != nil {
			field, ok := qs.internals.Annotations[selectedField]
			if ok {
				qs.internals.Fields = append(qs.internals.Fields, FieldInfo{
					Table: Table{
						Name: qs.queryInfo.TableName,
					},
					Fields: []attrs.Field{field},
				})
				continue
			}

			panic(err)
		}

		// Check if expression, wrap it in exprField
		if expr, ok := selectedFieldObj.(expr.NamedExpression); ok {
			field = &exprField{
				Field: field,
				expr:  expr,
			}
		}

		// The field might be a relation
		var rel = field.Rel()

		// If all fields of the relation are requested, we need to add the relation
		// to the join list. We also need to add the parent field to the chain.
		//
		// this must be in line with alias generation in internal.WalkFields!!!
		if (rel != nil) && allFields {
			chain = append(chain, field.Name())
			var meta = attrs.GetModelMeta(rel.Model())
			var defs = meta.Definitions()
			aliases = append(aliases, qs.AliasGen.GetTableAlias(
				defs.TableName(), selectedField,
			))
			parent = current
			isRelated = true
		}

		var defs = current.FieldDefs()
		var tableName = defs.TableName()
		if len(chain) > 0 && isRelated {

			var (
				infos []FieldInfo
				join  []JoinDef
			)

			var parentDefs = parent.FieldDefs()
			var parentField, ok = parentDefs.Field(chain[len(chain)-1])
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", chain[len(chain)-1], parent))
			}

			if rel == nil {
				rel = parentField.Rel()
			}

			switch rel.Type() {
			case attrs.RelManyToOne:
				infos, join = qs.addJoinForFK(rel, parentDefs, parentField, field, chain, aliases, allFields, qs.internals.joinsMap)
			case attrs.RelOneToOne:
				infos, join = qs.addJoinForO2O(rel, parentDefs, parentField, field, chain, aliases, allFields, qs.internals.joinsMap)
			case attrs.RelManyToMany:
				infos, join = qs.addJoinForM2M(rel, parentDefs, parentField, field, chain, aliases, allFields, qs.internals.joinsMap)
			case attrs.RelOneToMany:
				infos, join = qs.addJoinForFK(rel, parentDefs, parentField, field, chain, aliases, allFields, qs.internals.joinsMap)
			default:
				panic(fmt.Errorf("field %q (%T) is not a relation %s", field.Name(), field, rel.Type()))
			}

			if len(infos) > 0 {
				qs.internals.Fields = append(qs.internals.Fields, infos...)
				qs.internals.Joins = append(qs.internals.Joins, join...)
			}

			continue
		}

		qs.internals.Fields = append(qs.internals.Fields, FieldInfo{
			Model: current,
			Table: Table{
				Name: tableName,
			},
			Fields: []attrs.Field{field},
			Chain:  chain,
		})
	}

	return qs
}

// Filter is used to filter the results of a query.
//
// It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.
//
// The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.
//
// By default the `__exact` (=) operator is used, each where clause is separated by `AND`.
func (qs *QuerySet[T]) Filter(key interface{}, vals ...interface{}) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.Where = append(qs.internals.Where, expr.Express(key, vals...)...)
	return nqs
}

// Having is used to filter the results of a query after aggregation.
//
// It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.
//
// The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.
func (qs *QuerySet[T]) Having(key interface{}, vals ...interface{}) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.Having = append(qs.internals.Having, expr.Express(key, vals...)...)
	return nqs
}

// GroupBy is used to group the results of a query.
//
// It takes a list of field names as arguments and returns a new QuerySet with the grouped results.
func (qs *QuerySet[T]) GroupBy(fields ...string) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.GroupBy, _ = qs.unpackFields(fields...)
	return nqs
}

// OrderBy is used to order the results of a query.
//
// It takes a list of field names as arguments and returns a new QuerySet with the ordered results.
//
// The field names can be prefixed with a minus sign (-) to indicate descending order.
func (qs *QuerySet[T]) OrderBy(fields ...string) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.OrderBy = make([]OrderBy, 0, len(fields))

	for _, field := range fields {
		var ord = strings.TrimSpace(field)
		var desc = false
		if strings.HasPrefix(ord, "-") {
			desc = true
			ord = strings.TrimPrefix(ord, "-")
		}

		var obj, _, field, _, aliases, _, err = internal.WalkFields(
			qs.model, ord, qs.AliasGen,
		)

		if err != nil {
			var ok bool
			field, ok = qs.internals.Annotations[ord]
			if !ok {
				panic(err)
			}
		}

		var defs = obj.FieldDefs()
		var tableAlias string
		if len(aliases) > 0 {
			tableAlias = aliases[len(aliases)-1]
		} else {
			tableAlias = defs.TableName()
		}

		var alias string
		if vF, ok := field.(AliasField); ok {
			alias = qs.AliasGen.GetFieldAlias(
				tableAlias, vF.Alias(),
			)
		}

		nqs.internals.OrderBy = append(nqs.internals.OrderBy, OrderBy{
			Table: Table{
				Name:  defs.TableName(),
				Alias: tableAlias,
			},
			Field: field.ColumnName(),
			Alias: alias,
			Desc:  desc,
		})
	}

	return nqs
}

// Reverse is used to reverse the order of the results of a query.
//
// It returns a new QuerySet with the reversed order.
func (qs *QuerySet[T]) Reverse() *QuerySet[T] {
	var ordBy = make([]OrderBy, 0, len(qs.internals.OrderBy))
	for _, ord := range qs.internals.OrderBy {
		ordBy = append(ordBy, OrderBy{
			Table: ord.Table,
			Field: ord.Field,
			Alias: ord.Alias,
			Desc:  !ord.Desc,
		})
	}
	var nqs = qs.Clone()
	nqs.internals.OrderBy = ordBy
	return nqs
}

// Limit is used to limit the number of results returned by a query.
func (qs *QuerySet[T]) Limit(n int) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.Limit = n
	return nqs
}

// Offset is used to set the offset of the results returned by a query.
func (qs *QuerySet[T]) Offset(n int) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.Offset = n
	return nqs
}

// ForUpdate is used to lock the rows returned by a query for update.
//
// It is used to prevent other transactions from modifying the rows until the current transaction is committed or rolled back.
func (qs *QuerySet[T]) ForUpdate() *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.ForUpdate = true
	return nqs
}

// Distinct is used to select distinct rows from the results of a query.
//
// It is used to remove duplicate rows from the results.
func (qs *QuerySet[T]) Distinct() *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.internals.Distinct = true
	return nqs
}

// ExplicitSave is used to indicate that the save operation should be explicit.
//
// It is used to prevent the automatic save operation from being performed on the model.
//
// I.E. when using the `Create` method after calling `qs.ExplicitSave()`, it will **not** automatically
// save the model to the database using the model's own `Save` method.
func (qs *QuerySet[T]) ExplicitSave() *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.explicitSave = true
	return nqs
}

func (qs *QuerySet[T]) annotate(alias string, expr expr.Expression) {
	field := newQueryField[any](alias, expr)
	qs.internals.Annotations[alias] = field

	qs.internals.Fields = append(qs.internals.Fields, FieldInfo{
		Model: nil,
		Table: Table{
			Name: qs.queryInfo.TableName,
		},
		Fields: []attrs.Field{field},
	})
}

// Annotate is used to add annotations to the results of a query.
//
// It takes a string or a map of strings to expr.Expressions as arguments and returns a new QuerySet with the annotations.
//
// If a string is provided, it is used as the alias for the expr.Expression.
//
// If a map is provided, the keys are used as aliases for the expr.Expressions.
func (qs *QuerySet[T]) Annotate(aliasOrAliasMap interface{}, exprs ...expr.Expression) *QuerySet[T] {
	qs = qs.Clone()

	switch aliasOrAliasMap := aliasOrAliasMap.(type) {
	case string:
		if len(exprs) == 0 {
			panic("QuerySet: no expr.Expression provided")
		}
		qs.annotate(aliasOrAliasMap, exprs[0])
	case map[string]expr.Expression:
		if len(exprs) > 0 {
			panic("QuerySet: map and expr.Expressions both provided")
		}
		for alias, e := range aliasOrAliasMap {
			qs.annotate(alias, e)
		}
	case map[string]any:
		if len(exprs) > 0 {
			panic("QuerySet: map and expr.Expressions both provided")
		}
		for alias, e := range aliasOrAliasMap {
			if exprs, ok := e.(expr.Expression); ok {
				qs.annotate(alias, exprs)
			} else {
				panic(fmt.Errorf(
					"QuerySet: %q is not an expr.Expression (%T)", alias, e,
				))
			}
		}
	}

	return qs
}

type Row[T attrs.Definer] struct {
	Object      T
	Annotations map[string]any
	QuerySet    *QuerySet[T]
}

func (qs *QuerySet[T]) queryAll(fields ...any) CompiledQuery[[][]interface{}] {
	// Select all fields if no fields are provided
	//
	// Override the pointer to the original QuerySet with the Select("*") QuerySet
	if len(qs.internals.Fields) == 0 && len(fields) == 0 {
		*qs = *qs.Select("*")
	}

	if len(fields) > 0 {
		*qs = *qs.Select(fields...)
	}

	var query = qs.compiler.BuildSelectQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals.Fields,
		qs.internals.Where,
		qs.internals.Having,
		qs.internals.Joins,
		qs.internals.GroupBy,
		qs.internals.OrderBy,
		qs.internals.Limit,
		qs.internals.Offset,
		qs.internals.ForUpdate,
		qs.internals.Distinct,
	)
	qs.latestQuery = query

	return query
}

func (qs *QuerySet[T]) queryAggregate() CompiledQuery[[][]interface{}] {
	var query = qs.compiler.BuildSelectQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals.Fields,
		qs.internals.Where,
		qs.internals.Having,
		qs.internals.Joins,
		qs.internals.GroupBy,
		nil,   // no order
		1,     // just one row
		0,     // no offset
		false, // not for update
		false, // not distinct
	)
	qs.latestQuery = query
	return query
}

func (qs *QuerySet[T]) queryCount() CompiledQuery[int64] {
	var q = qs.compiler.BuildCountQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		qs.internals.Limit,
		qs.internals.Offset,
	)
	qs.latestQuery = q
	return q
}

//type dedupeObject struct {
//	parent   attrs.Definer
//	children *orderedmap.OrderedMap[any, attrs.Definer]
//}
//
//type dedupeBuilder struct {
//	root               *scannableField
//	possibleDuplicates []*scannableField
//	// map[chain key] -> map[parent PK] -> object(parent, [](child PK -> child))
//	parents map[string]map[any]*dedupeObject
//	objects *orderedmap.OrderedMap[any, attrs.Definer]
//}
//
//func newDedupeBuilder(root *scannableField, possibleDuplicates []*scannableField) *dedupeBuilder {
//	var parents = make(map[string]map[any]*dedupeObject, len(possibleDuplicates))
//	for _, scannable := range possibleDuplicates {
//		parents[scannable.chainPart] = make(map[any]*dedupeObject)
//	}
//
//	return &dedupeBuilder{
//		root:               root,
//		possibleDuplicates: possibleDuplicates,
//		parents:            parents,
//		objects:            orderedmap.NewOrderedMap[any, attrs.Definer](),
//	}
//}
//
//func (d *dedupeBuilder) addRow(scannables []*scannableField) {
//	var (
//		rootField = scannables[d.root.idx]
//		rootObj   = rootField.object
//		rootPk    = rootField.field.GetValue()
//	)
//
//	// Check if we already have this object
//	if _, ok := d.objects.Get(rootPk); !ok {
//		d.objects.Set(rootPk, rootObj)
//	}
//
//	// Loop through parents and add them to the appropriate parents
//	for _, dup := range d.possibleDuplicates {
//		var (
//			objectField     = scannables[dup.idx]
//			parentScannable = objectField.srcField
//			parentInstance  = parentScannable.object
//			parentDefs      = parentInstance.FieldDefs()
//			parentPrimary   = parentDefs.Primary()
//			parentPk        = parentPrimary.GetValue()
//			childMap        = d.parents[objectField.chainPart]
//		)
//
//		var (
//			dedupeObj *dedupeObject
//			ok        bool
//		)
//
//		if dedupeObj, ok = childMap[parentPk]; !ok {
//			dedupeObj = &dedupeObject{
//				parent:   parentInstance,
//				children: orderedmap.NewOrderedMap[any, attrs.Definer](),
//			}
//			childMap[parentPk] = dedupeObj
//		}
//
//		var objectPk = objectField.field.GetValue()
//		if _, has := dedupeObj.children.Get(objectPk); !has {
//			dedupeObj.children.Set(
//				objectField.field.GetValue(), objectField.object,
//			)
//		}
//	}
//}
//
//func (d *dedupeBuilder) build() []*Row[attrs.Definer] {
//	var out = make([]*Row[attrs.Definer], 0, d.objects.Len())
//	for head := d.objects.Front(); head != nil; head = head.Next() {
//		for chainKey, parentMap := range d.parents {
//			for parentPk, parentObj := range parentMap {
//
//			}
//		}
//	}
//
//	return out
//}

type dedupeNode struct {
	children map[string]map[any]*dedupeNode // chain name -> PK -> next node
	objects  map[any]struct{}               // Only for leaves: PKs we've already seen at this level
}

func newDedupeNode() *dedupeNode {
	return &dedupeNode{
		children: make(map[string]map[any]*dedupeNode),
		objects:  make(map[any]struct{}),
	}
}

type chainPart struct {
	chain  string
	pk     any
	object attrs.Definer
}

func (n *dedupeNode) Has(keyParts []chainPart) bool {
	return n.has(keyParts, 0)
}

func (n *dedupeNode) Add(keyParts []chainPart) {
	n.add(keyParts, 0)
}

func (n *dedupeNode) has(keyParts []chainPart, partsIdx int) bool {
	part := keyParts[partsIdx]
	if partsIdx == len(keyParts)-1 {
		_, ok := n.objects[part.pk]
		return ok
	}
	nextMap, ok := n.children[part.chain]
	if !ok {
		return false
	}
	child, ok := nextMap[part.pk]
	if !ok {
		return false
	}
	return child.has(keyParts, partsIdx+1)
}

func (n *dedupeNode) add(keyParts []chainPart, partsIdx int) {
	part := keyParts[partsIdx]
	if partsIdx == len(keyParts)-1 {
		n.objects[part.pk] = struct{}{}
		return
	}
	nextMap, ok := n.children[part.chain]
	if !ok {
		nextMap = make(map[any]*dedupeNode)
		n.children[part.chain] = nextMap
	}
	child, ok := nextMap[part.pk]
	if !ok {
		child = newDedupeNode()
		nextMap[part.pk] = child

	}
	child.add(keyParts, partsIdx+1)
}

func buildChainParts(actualField *scannableField) []chainPart {
	// Get the stack of fields from target to parent
	var stack = make([]chainPart, 0)
	for cur := actualField; cur != nil; cur = cur.srcField {
		var (
			inst    = cur.field.Instance()
			defs    = inst.FieldDefs()
			primary = defs.Primary()
		)

		stack = append(stack, chainPart{
			chain:  cur.chainPart,
			pk:     primary.GetValue(),
			object: inst,
		})
	}

	// Reverse the stack to get the fields in the correct order
	// i.e. parent to target
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}

	return stack
}

// All is used to retrieve all rows from the database.
//
// It returns a Query that can be executed to get the results, which is a slice of Row objects.
//
// Each Row object contains the model object and a map of annotations.
//
// If no fields are provided, it selects all fields from the model, see `Select()` for more details.
func (qs *QuerySet[T]) All() ([]*Row[T], error) {
	if qs.cached != nil && qs.useCache {
		return qs.cached.([]*Row[T]), nil
	}

	var resultQuery = qs.queryAll()
	var results, err = resultQuery.Exec()
	if err != nil {
		return nil, err
	}

	var (
		possibleDuplicates = make([]*scannableField, 0)
		relationFields     = make(map[int]*scannableField)
	)
	var scannables = getScannableFields(
		qs.internals.Fields, internal.NewObjectFromIface(qs.model),
	)

	var rootScannable *scannableField
	for _, scannable := range scannables {
		if (scannable.relType != -1 && scannable.relType != attrs.RelNone) && scannable.field.IsPrimary() {
			relationFields[scannable.idx] = scannable
		}
		if (scannable.relType == attrs.RelManyToMany || scannable.relType == attrs.RelOneToMany) && scannable.field.IsPrimary() {
			possibleDuplicates = append(possibleDuplicates, scannable)
		}
		if scannable.relType == -1 && scannable.field.IsPrimary() && rootScannable == nil {
			rootScannable = scannable
		}
	}

	if rootScannable != nil && len(possibleDuplicates) > 0 {
		possibleDuplicates = append(possibleDuplicates, rootScannable)
	}

	var (
		dedupe = newDedupeNode()
		list   = make([]*Row[T], 0, len(results))
		// parents = make(map[string]map[any]attrs.Definer) // chain name -> PK -> next node
		prev []*scannableField
	)

	for _, row := range results {
		var (
			obj        = internal.NewObjectFromIface(qs.model)
			scannables = getScannableFields(qs.internals.Fields, obj)
		)

		var (
			annotator, _ = obj.(DataModel)
			annotations  = make(map[string]any)
			datastore    ModelDataStore
		)

		if annotator != nil {
			datastore = annotator.ModelDataStore()
		}

		for j, field := range scannables {
			f := field.field
			val := row[j]

			if err := f.Scan(val); err != nil {
				return nil, errors.Wrapf(err, "failed to scan field %q in %T", f.Name(), obj)
			}

			//	if _, ok := relationFields[field.idx]; ok {
			//		var cur = field
			//		for {
			//			parentMap, ok := parents[cur.chainKey]
			//			if !ok {
			//				parentMap = make(map[any]attrs.Definer)
			//				parents[cur.chainKey] = parentMap
			//			}
			//
			//			var primary = cur.object.FieldDefs().Primary()
			//			if _, ok := parentMap[primary.GetValue()]; ok {
			//				break
			//			}
			//
			//			parentMap[primary] = cur.object
			//
			//			if cur.srcField == nil {
			//				break
			//			}
			//
			//			cur = cur.srcField
			//		}
			//	}

			// If it's a virtual field not in the model, store as annotation
			if vf, ok := f.(AliasField); ok {
				var (
					alias = vf.Alias()
					val   = vf.GetValue()
				)
				if alias == "" {
					alias = f.Name()
				}

				annotations[alias] = val

				if datastore != nil {
					datastore.SetValue(alias, val)
				}
			}
		}

		var newRow = true
		for _, possibleDuplicate := range possibleDuplicates {
			var (
				workingObj attrs.Definer

				actualField = scannables[possibleDuplicate.idx]
				chainParts  = buildChainParts(
					actualField,
				)
				has = dedupe.Has(chainParts)
			)

			if len(actualField.chainPart) > 0 {
				var f *scannableField
				if prev != nil && prev[rootScannable.idx].field.GetValue() == scannables[rootScannable.idx].field.GetValue() {
					f = prev[possibleDuplicate.idx]
				} else {
					f = actualField
				}
				fmt.Printf(
					"Has: %v, %d, Setting %v, %v, %p, %T on %T %v\n",
					has, possibleDuplicate.idx,

					actualField.chainPart,
					scannables[possibleDuplicate.idx].field.GetValue(),
					scannables[possibleDuplicate.idx].field.Instance(),
					scannables[possibleDuplicate.idx].field.Instance(),

					f.srcField.field.Instance(),
					f.srcField.field.Instance().FieldDefs().Primary().GetValue(),
				)
			} else {
				fmt.Printf(
					"Has: %v, %d, Working on %T %v\n",
					has, possibleDuplicate.idx,
					actualField.field.Instance(), actualField.field.GetValue(),
				)
			}

			if has {
				newRow = false
				continue
			}

			dedupe.Add(chainParts)

			// this is eithe root value (safe to skip)
			if actualField.idx == rootScannable.idx {
				fmt.Printf("continuing %T %v\n", actualField.field.Instance(), actualField.field.GetValue())
				continue
			}

			//parentMap := parents[actualField.srcField.chainKey]
			//workingObj = parentMap[actualField.srcField.object.FieldDefs().Primary().GetValue()]

			// retrieve the panret object from the last row (if any)
			if prev != nil && prev[rootScannable.idx].field.GetValue() == scannables[rootScannable.idx].field.GetValue() {
				workingObj = prev[possibleDuplicate.idx].srcField.field.Instance()
			} else {
				workingObj = actualField.srcField.field.Instance()
			}

			fmt.Printf("workingObj %T %v %v\n", workingObj, workingObj.FieldDefs().Primary().GetValue(), actualField.srcField)

			// set the value on the [DataModel] as a slice of related objects
			var dataModel = workingObj.(DataModel)
			var dataStore = dataModel.ModelDataStore()
			existing, _ := dataStore.GetValue(actualField.chainPart)

			var slice []attrs.Definer
			switch v := existing.(type) {
			case nil:
				slice = []attrs.Definer{}
			case attrs.Definer:
				// no need to add, if the object is new it will be added
				// as actualField.object automatically below.
				slice = make([]attrs.Definer, 0)
			case []attrs.Definer:
				slice = v
			default:
				panic(fmt.Errorf("unexpected type for relatedName %s: %T", actualField.chainPart, existing))
			}

			slice = append(
				slice, actualField.object,
			)

			if err := dataStore.SetValue(actualField.chainPart, slice); err != nil {
				panic(err)
			}
			fmt.Println("----------------------------------------------")
		}

		if !newRow {
			continue
		}

		prev = scannables
		list = append(list, &Row[T]{
			QuerySet:    qs,
			Object:      obj.(T),
			Annotations: annotations,
		})

		if len(possibleDuplicates) > 0 {
			fmt.Println("_____________________________________________")
		}
	}

	if len(possibleDuplicates) > 0 {
		fmt.Println()

		var sb = &strings.Builder{}
		printDedupe(sb, dedupe, 0)
		fmt.Println(sb.String())

		fmt.Println()
	}

	if qs.useCache {
		qs.cached = list
	}

	return list, nil
}

func printDedupe(sb *strings.Builder, dedupe *dedupeNode, depth int) {
	if dedupe.children != nil {
		for path, childMap := range dedupe.children {
			for k, dedupe := range childMap {
				sb.WriteString(strings.Repeat("\t", depth+1))
				sb.WriteString(fmt.Sprintf("'%v' (%s): %v\n", k, path, dedupe))
				printDedupe(sb, dedupe, depth+1)
			}
		}
	}
}

// ValuesList is used to retrieve a list of values from the database.
//
// It takes a list of field names as arguments and returns a ValuesListQuery.
func (qs *QuerySet[T]) ValuesList(fields ...any) ([][]interface{}, error) {
	if qs.cached != nil && qs.useCache {
		return qs.cached.([][]any), nil
	}

	var resultQuery = qs.queryAll(fields...)
	var results, err = resultQuery.Exec()
	if err != nil {
		return nil, err
	}

	var list = make([][]any, len(results))
	for i, row := range results {
		var obj = internal.NewObjectFromIface(qs.model)
		var fields = getScannableFields(qs.internals.Fields, obj)
		var values = make([]any, len(fields))
		for j, field := range fields {
			var f = field.field
			var val = row[j]

			if err = f.Scan(val); err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to scan field %q in %T",
					f.Name(), row,
				)
			}

			var v = f.GetValue()
			values[j] = v
		}

		list[i] = values
	}

	if qs.useCache {
		qs.cached = list
	}

	return list, nil
}

// Aggregate is used to perform aggregation on the results of a query.
//
// It takes a map of field names to expr.Expressions as arguments and returns a Query that can be executed to get the results.
func (qs *QuerySet[T]) Aggregate(annotations map[string]expr.Expression) (map[string]any, error) {
	if qs.cached != nil && qs.useCache {
		return qs.cached.(map[string]any), nil
	}

	qs.internals.Fields = make([]FieldInfo, 0, len(annotations))

	for alias, expr := range annotations {
		qs.internals.Fields = append(qs.internals.Fields, FieldInfo{
			Model: nil,
			Table: Table{
				Name: qs.queryInfo.TableName,
			},
			Fields: []attrs.Field{newQueryField[any](alias, expr)},
		})
	}

	var query = qs.queryAggregate()
	var results, err = query.Exec()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return map[string]any{}, nil
	}

	var scannables = getScannableFields(qs.internals.Fields, internal.NewObjectFromIface(qs.model))
	var row = results[0]
	var out = make(map[string]any)

	for i, field := range scannables {
		if vf, ok := field.field.(AliasField); ok {
			if err := vf.Scan(row[i]); err != nil {
				return nil, err
			}
			out[vf.Alias()] = vf.GetValue()
		}
	}

	if qs.useCache {
		qs.cached = out
	}

	return out, nil

}

// Get is used to retrieve a single row from the database.
//
// It returns a Query that can be executed to get the result, which is a Row object
// that contains the model object and a map of annotations.
//
// It panics if the queryset has no where clause.
//
// If no rows are found, it returns queries.query_errors.ErrNoRows.
//
// If multiple rows are found, it returns queries.query_errors.ErrMultipleRows.
func (qs *QuerySet[T]) Get() (*Row[T], error) {
	if len(qs.internals.Where) == 0 {
		panic(query_errors.ErrNoWhereClause)
	}

	if qs.cached != nil && qs.useCache {
		return qs.cached.(*Row[T]), nil
	}

	var nillRow = &Row[T]{
		QuerySet: qs,
	}

	*qs = *qs.Limit(MAX_GET_RESULTS)
	var results, err = qs.All()
	if err != nil {
		return nillRow, err
	}

	var resCnt = len(results)
	if resCnt == 0 {
		return nillRow, query_errors.ErrNoRows
	}

	if resCnt > 1 {
		var errResCnt string
		if MAX_GET_RESULTS == 0 || resCnt < MAX_GET_RESULTS {
			errResCnt = strconv.Itoa(resCnt)
		} else {
			errResCnt = strconv.Itoa(MAX_GET_RESULTS-1) + "+"
		}

		return nillRow, errors.Wrapf(
			query_errors.ErrMultipleRows,
			"multiple rows returned for %T: %s rows",
			qs.model, errResCnt,
		)
	}

	if qs.useCache {
		qs.cached = results[0]
	}

	return results[0], nil

}

// GetOrCreate is used to retrieve a single row from the database or create it if it does not exist.
//
// It returns the definer object and an error if any occurred.
//
// This method executes a transaction to ensure that the object is created only once.
//
// It panics if the queryset has no where clause.
func (qs *QuerySet[T]) GetOrCreate(value T) (T, error) {

	if len(qs.internals.Where) == 0 {
		panic(query_errors.ErrNoWhereClause)
	}

	// Create a new transaction
	var ctx = context.Background()
	var transaction, err = qs.compiler.StartTransaction(ctx)
	if err != nil {
		return *new(T), err
	}

	defer transaction.Rollback()

	// Check if the object already exists
	qs.useCache = false
	row, err := qs.Get()
	if err != nil {
		if errors.Is(err, query_errors.ErrNoRows) {
			goto create
		} else {
			return *new(T), err
		}
	}

	// Object already exists, return it and commit the transaction
	if row != nil {
		return row.Object, transaction.Commit()
	}

	// Object does not exist, create it
create:
	obj, err := qs.Create(value)
	if err != nil {
		return *new(T), err
	}

	// Object was created successfully, commit the transaction
	return obj, transaction.Commit()
}

// First is used to retrieve the first row from the database.
//
// It returns a Query that can be executed to get the result, which is a Row object
// that contains the model object and a map of annotations.
func (qs *QuerySet[T]) First() (*Row[T], error) {
	*qs = *qs.Limit(1)
	var results, err = qs.All()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, query_errors.ErrNoRows
	}
	return results[0], nil

}

// Last is used to retrieve the last row from the database.
//
// It reverses the order of the results and then calls First to get the last row.
//
// It returns a Query that can be executed to get the result, which is a Row object
// that contains the model object and a map of annotations.
func (qs *QuerySet[T]) Last() (*Row[T], error) {
	*qs = *qs.Reverse()
	return qs.First()
}

// Exists is used to check if any rows exist in the database.
//
// It returns a Query that can be executed to get the result,
// which is a boolean indicating if any rows exist.
func (qs *QuerySet[T]) Exists() (bool, error) {
	var resultQuery = qs.compiler.BuildCountQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		1,
		0,
	)
	qs.latestQuery = resultQuery

	var exists, err = resultQuery.Exec()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// Count is used to count the number of rows in the database.
//
// It returns a CountQuery that can be executed to get the result, which is an int64 indicating the number of rows.
func (qs *QuerySet[T]) Count() (int64, error) {
	var q = qs.queryCount()
	var count, err = q.Exec()
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Create is used to create a new object in the database.
//
// It takes a definer object as an argument and returns a Query that can be executed
// to get the result, which is the created object.
//
// It panics if a non- nullable field is null or if the field is not found in the model.
//
// The model can adhere to django's `models.Saver` interface, in which case the `Save()` method will be called
// unless `ExplicitSave()` was called on the queryset.
//
// If `ExplicitSave()` was called, the `Create()` method will return a query that can be executed to create the object
// without calling the `Save()` method on the model.
func (qs *QuerySet[T]) Create(value T) (T, error) {

	// Check if the object is a saver
	// If it is, we can use the Save method to save the object
	if saver, ok := any(value).(models.ContextSaver); ok && !qs.explicitSave {
		if err := sendSignal(SignalPreModelSave, value, qs.compiler); err != nil {
			return *new(T), err
		}

		var err = saver.Save(context.Background())
		if err != nil {
			return *new(T), err
		}

		if err := sendSignal(SignalPostModelSave, value, qs.compiler); err != nil {
			return *new(T), err
		}

		return saver.(T), nil
	}

	var defs = value.FieldDefs()
	var fields = defs.Fields()
	var values = make([]any, 0, len(fields))
	var infoFields = make([]attrs.Field, 0, len(fields))
	var info = FieldInfo{
		Model: value,
		Table: Table{
			Name: defs.TableName(),
		},
		Chain: make([]string, 0),
	}

	for _, field := range fields {
		var atts = field.Attrs()
		var v, ok = atts[attrs.AttrAutoIncrementKey]
		if ok && v.(bool) {
			continue
		}

		if field.IsPrimary() || !field.AllowEdit() {
			continue
		}

		var value, err = field.Value()
		if err != nil {
			panic(fmt.Errorf("failed to get value for field %q: %w", field.Name(), err))
		}

		if value == nil && !field.AllowNull() {
			panic(errors.Wrapf(
				query_errors.ErrFieldNull,
				"field %q cannot be null",
				field.Name(),
			))
		}

		infoFields = append(infoFields, field)
		values = append(values, value)
	}

	// Copy all the fields from the model to the info fields
	info.Fields = slices.Clone(infoFields)

	var support = qs.compiler.SupportsReturning()
	var resultQuery = qs.compiler.BuildCreateQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		info,
		defs.Primary(),
		values,
	)
	qs.latestQuery = resultQuery

	var newObj = internal.NewObjectFromIface(qs.model)
	var newDefs = newObj.FieldDefs()

	// Set the old values on the new object
	for _, field := range infoFields {
		var (
			n     = field.Name()
			f, ok = newDefs.Field(n)
		)
		if !ok {
			panic(fmt.Errorf("field %q not found in %T", n, newObj))
		}

		var val = field.GetValue()
		if err := f.SetValue(val, true); err != nil {
			return *new(T), errors.Wrapf(
				err,
				"failed to set field %q in %T",
				f.Name(), newObj,
			)
		}
	}

	// Execute the create query
	var results, err = resultQuery.Exec()
	if err != nil {
		return *new(T), err
	}

	// Check results & which returning method to use
	switch {
	case len(results) == 0 && support == SupportsReturningNone:
		// Do nothing

	case len(results) > 0 && support == SupportsReturningLastInsertId:
		var (
			id   = results[0].(int64)
			prim = newDefs.Primary()
		)
		if err := prim.SetValue(id, true); err != nil {
			return *new(T), errors.Wrapf(
				err,
				"failed to set primary key %q in %T",
				prim.Name(), newObj,
			)
		}

	case len(results) > 0 && support == SupportsReturningColumns:
		var (
			scannables = getScannableFields([]FieldInfo{info}, newObj)
			resLen     = len(results)
			prim       = newDefs.Primary()
		)
		if prim != nil {
			resLen--
		}

		if len(scannables) != resLen {
			return *new(T), errors.Wrapf(
				query_errors.ErrLastInsertId,
				"expected %d results returned after insert, got %d",
				len(scannables), len(results),
			)
		}

		var idx = 0
		if prim != nil {
			var id = results[0].(int64)
			if err := prim.Scan(id); err != nil {
				return *new(T), errors.Wrapf(
					err, "failed to scan primary key %q in %T",
					prim.Name(), newObj,
				)
			}
			idx++
		}

		for i, field := range scannables {
			var f = field.field
			var val = results[i+idx]

			if err := f.Scan(val); err != nil {
				return *new(T), errors.Wrapf(
					err,
					"failed to scan field %q in %T",
					f.Name(), newObj,
				)
			}
		}
	}

	var rVal = reflect.ValueOf(value)
	if rVal.Kind() == reflect.Ptr {
		rVal.Elem().Set(reflect.ValueOf(newObj).Elem())
	}

	return newObj.(T), nil
}

// Update is used to update an object in the database.
//
// It takes a definer object as an argument and returns a CountQuery that can be executed
// to get the result, which is the number of rows affected.
//
// It panics if a non- nullable field is null or if the field is not found in the model.
//
// If the model adheres to django's `models.Saver` interface, no where clause is provided
// and ExplicitSave() was not called, the `Save()` method will be called on the model
func (qs *QuerySet[T]) Update(value T, expressions ...expr.NamedExpression) (int64, error) {
	if len(qs.internals.Where) == 0 && !qs.explicitSave {
		var (
			defs            = value.FieldDefs()
			primary         = defs.Primary()
			primaryVal, err = primary.Value()
		)

		if err != nil {
			panic(fmt.Errorf("failed to get value for field %q: %w", primary.Name(), err))
		}

		if saver, ok := any(value).(models.ContextSaver); ok && !fields.IsZero(primaryVal) {
			if err := sendSignal(SignalPreModelSave, value, qs.compiler); err != nil {
				return 0, err
			}

			var err = saver.Save(context.Background())
			if err != nil {
				return 0, err
			}

			if err := sendSignal(SignalPostModelSave, value, qs.compiler); err != nil {
				return 0, err
			}
			return 1, nil
		}
	}

	var defs = value.FieldDefs()
	var fields []attrs.Field = qs.attrFields(value)
	var values = make([]any, 0, len(fields))
	var info = FieldInfo{
		Model: value,
		Table: Table{
			Name: defs.TableName(),
		},
		Fields: make([]attrs.Field, 0),
		Chain:  make([]string, 0),
	}

	var exprMap = make(map[string]expr.NamedExpression, len(expressions))
	for _, expr := range expressions {
		var fieldName = expr.FieldName()
		if fieldName == "" {
			panic(fmt.Errorf("expression %q has no field name", expr))
		}

		if _, ok := exprMap[fieldName]; ok {
			panic(fmt.Errorf("duplicate field %q in update expression", fieldName))
		}

		exprMap[fieldName] = expr
	}

	for _, field := range fields {
		var atts = field.Attrs()
		var v, ok = atts[attrs.AttrAutoIncrementKey]
		if ok && v.(bool) {
			continue
		}

		if field.IsPrimary() || !field.AllowEdit() {
			continue
		}

		var value, err = field.Value()
		if err != nil {
			panic(fmt.Errorf("failed to get value for field %q: %w", field.Name(), err))
		}

		if value == nil && !field.AllowNull() {
			panic(errors.Wrapf(
				query_errors.ErrFieldNull,
				"field %q cannot be null",
				field.Name(),
			))
		}

		if expr, ok := exprMap[field.Name()]; ok {
			info.Fields = append(info.Fields, &exprField{
				Field: field,
				expr:  expr,
			})
			continue
		}

		info.Fields = append(info.Fields, field)
		values = append(values, value)
	}

	var resultQuery = qs.compiler.BuildUpdateQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		info,
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		values,
	)
	qs.latestQuery = resultQuery

	return resultQuery.Exec()
}

// Delete is used to delete an object from the database.
//
// It returns a CountQuery that can be executed to get the result, which is the number of rows affected.
func (qs *QuerySet[T]) Delete() (int64, error) {
	var resultQuery = qs.compiler.BuildDeleteQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
	)
	qs.latestQuery = resultQuery

	return resultQuery.Exec()
}

type scannableField struct {
	idx       int
	object    attrs.Definer
	field     attrs.Field
	srcField  *scannableField
	relType   attrs.RelationType
	chainPart string // name of the field in the chain
	chainKey  string // the chain up to this point, joined by "."
}

func getScannableFields(fields []FieldInfo, root attrs.Definer) []*scannableField {
	var listSize = 0
	for _, info := range fields {
		listSize += len(info.Fields)
	}

	var (
		scannables    = make([]*scannableField, 0, listSize)
		instances     = make(map[string]attrs.Definer)
		parentFields  = make(map[string]*scannableField) // NEW: store parent scannableFields by chain
		rootScannable *scannableField
		idx           = 0
	)
	for _, info := range fields {
		if info.SourceField == nil {
			defs := root.FieldDefs()
			for _, f := range info.Fields {
				if _, ok := f.(VirtualField); ok && info.Model == nil {
					scannables = append(scannables, &scannableField{
						idx:     idx,
						field:   f,
						relType: -1,
					})
					idx++
					continue
				}
				field, ok := defs.Field(f.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", f.Name(), root))
				}
				var sf = &scannableField{
					idx:     idx,
					field:   field,
					object:  root,
					relType: -1,
				}

				if field.IsPrimary() && rootScannable == nil {
					rootScannable = sf
				}

				scannables = append(scannables, sf)
				idx++
			}
			continue
		}

		instances[""] = root
		parentFields[""] = rootScannable

		// Walk chain
		var parentKey string
		var parentScannable = rootScannable
		var parentObj = root
		for i, name := range info.Chain {
			key := strings.Join(info.Chain[:i+1], ".")
			parent := instances[parentKey]
			defs := parent.FieldDefs()
			field, ok := defs.Field(name)
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", name, parent))
			}
			if _, exists := instances[key]; !exists {
				var obj attrs.Definer
				var rel = field.Rel()
				if i == len(info.Chain)-1 {
					obj = internal.NewObjectFromIface(info.Model)
				} else {
					obj = internal.NewObjectFromIface(rel.Model())
				}
				if err := field.SetValue(obj, true); err != nil {
					panic(fmt.Errorf("failed to set relation %q: %w", field.Name(), err))
				}
				instances[key] = obj

				// Make the scannableField node for this relation link to its parent
				newParent := &scannableField{
					relType:   rel.Type(),
					chainPart: name,
					chainKey:  key,
					object:    obj,
					field:     obj.FieldDefs().Primary(),
					idx:       -1,                      // Not a leaf
					srcField:  parentFields[parentKey], // link to parent in the chain
				}
				parentFields[key] = newParent
			}

			parentScannable = parentFields[key]
			parentObj = instances[key]
			parentKey = key
		}

		var final = parentObj
		var finalDefs = final.FieldDefs()
		for _, f := range info.Fields {
			field, ok := finalDefs.Field(f.Name())
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", f.Name(), final))
			}

			var cpy = *parentScannable
			cpy.idx = idx
			cpy.object = final
			cpy.field = field
			scannables = append(scannables, &cpy)

			idx++
		}
	}

	return scannables
}
