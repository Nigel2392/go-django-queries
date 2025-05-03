package queries

import (
	"context"
	"database/sql/driver"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	django "github.com/Nigel2392/go-django/src"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/pkg/errors"

	_ "unsafe"
)

//go:linkname express github.com/Nigel2392/go-django-queries/src/expr.express
func express(key interface{}, vals ...interface{}) []expr.Expression

// -----------------------------------------------------------------------------
// QuerySet
// -----------------------------------------------------------------------------

const MAX_GET_RESULTS = 21

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

func (f *FieldInfo) WriteFields(sb *strings.Builder, d driver.Driver, m attrs.Definer, quote string) []any {
	var args = make([]any, 0, len(f.Fields))
	for i, field := range f.Fields {
		if i > 0 {
			sb.WriteString(", ")
		}

		var tableAlias string
		if f.Table.Alias == "" {
			tableAlias = f.Table.Name
		} else {
			tableAlias = f.Table.Alias
		}
		if ve, ok := field.(VirtualField); ok && m != nil {
			var alias = fmt.Sprintf(
				"%s_%s", tableAlias, ve.Alias(),
			)
			var sql, a = ve.SQL(d, m, quote)
			if sql == "" {
				// SQL is empty, we don't need to add it to the query
				continue
			}

			sb.WriteString(sql)

			if alias != "" {
				sb.WriteString(" AS ")
				sb.WriteString(quote)
				sb.WriteString(alias)
				sb.WriteString(quote)
			}

			args = append(args, a...)
			continue
		}

		sb.WriteString(quote)

		if f.Table.Alias == "" {
			sb.WriteString(f.Table.Name)
		} else {
			sb.WriteString(f.Table.Alias)
		}

		sb.WriteString(quote)
		sb.WriteString(".")
		sb.WriteString(quote)
		sb.WriteString(field.ColumnName())
		sb.WriteString(quote)
	}

	return args
}

type QuerySetInternals struct {
	Annotations map[string]*queryField[any]
	Fields      []FieldInfo
	Where       []expr.Expression
	Having      []expr.Expression
	Joins       []JoinDef
	GroupBy     []FieldInfo
	OrderBy     []OrderBy
	Limit       int
	Offset      int
	ForUpdate   bool
	Distinct    bool
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
type QuerySet struct {
	queryInfo    *internal.QueryInfo
	internals    *QuerySetInternals
	model        attrs.Definer
	compiler     QueryCompiler
	explicitSave bool
	useCache     bool
	cached       any
}

// Objects creates a new QuerySet for the given model.
//
// If the model implements the QueryDefiner interface,
// it will use the GetQuerySet method to get the initial QuerySet.
//
// It panics if:
// - the model is nil
// - the base query info cannot be retrieved
//
// It returns a pointer to a new QuerySet.
//
// The model must implement the Definer interface.
func Objects(model attrs.Definer, database ...string) *QuerySet {

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

	if m, ok := model.(QuerySetDefiner); ok {
		var qs = m.GetQuerySet()
		return qs.Clone()
	}

	var queryInfo, err = internal.GetBaseQueryInfo(model)
	if err != nil {
		panic(fmt.Errorf("QuerySet: %w", err))
	}

	if queryInfo == nil {
		panic("QuerySet: queryInfo is nil")
	}

	var qs = &QuerySet{
		model:     model,
		queryInfo: queryInfo,
		internals: &QuerySetInternals{
			Annotations: make(map[string]*queryField[any]),
			Where:       make([]expr.Expression, 0),
			Having:      make([]expr.Expression, 0),
			Joins:       make([]JoinDef, 0),
			GroupBy:     make([]FieldInfo, 0),
			OrderBy:     make([]OrderBy, 0),
			Limit:       1000,
			Offset:      0,
		},

		// enable queryset caching by default
		// this can result in race conditions in some rare edge cases
		// but is generally safe to use
		useCache: true,
	}
	qs.compiler = Compiler(model, defaultDb)
	return qs
}

// Return the underlying database which the compiler is using.
func (qs *QuerySet) DB() DB {
	return qs.compiler.DB()
}

// Return the model which the queryset is for.
func (qs *QuerySet) Model() attrs.Definer {
	return qs.model
}

// Return the compiler which the queryset is using.
func (qs *QuerySet) Compiler() QueryCompiler {
	return qs.compiler
}

// StartTransaction starts a transaction on the underlying database.
//
// It returns a transaction object which can be used to commit or rollback the transaction.
func (qs *QuerySet) StartTransaction(ctx context.Context) (Transaction, error) {
	return qs.compiler.StartTransaction(ctx)
}

// Clone creates a new QuerySet with the same parameters as the original one.
//
// It is used to create a new QuerySet with the same parameters as the original one, so that the original one is not modified.
//
// It is a shallow clone, underlying values like `*queries.Expr` are not cloned and have built- in immutability.
func (qs *QuerySet) Clone() *QuerySet {
	return &QuerySet{
		model:     qs.model,
		queryInfo: qs.queryInfo,
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
		},
		explicitSave: qs.explicitSave,
		useCache:     qs.useCache,
		compiler:     qs.compiler,
	}
}

// Return the string representation of the QuerySet.
func (w *QuerySet) String() string {
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
func (qs *QuerySet) GoString() string {
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
func (qs *QuerySet) unpackFields(fields ...string) (infos []FieldInfo, hasRelated bool) {
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

	for _, field := range fields {

		var onlyPrimary = false
		if strings.HasSuffix(strings.ToLower(field), "__pk") {
			field = field[:len(field)-4]
			onlyPrimary = true
		}

		var current, _, field, chain, aliases, isRelated, err = internal.WalkFields(qs.model, field)
		if err != nil {
			panic(err)
		}

		if isRelated && ((!onlyPrimary && len(chain) == 1) || len(chain) > 1) {
			hasRelated = true

			var relDefs = current.FieldDefs()
			var tableName = relDefs.TableName()
			infos = append(infos, FieldInfo{
				SourceField: field,
				Model:       current,
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

func (qs *QuerySet) attrFields(obj attrs.Definer) []attrs.Field {
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

func addJoinForFK(qs *QuerySet, foreignKey attrs.Definer, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) (*FieldInfo, []JoinDef) {
	var defs = foreignKey.FieldDefs()
	var tableName = defs.TableName()
	var relField attrs.Field

	if relFieldGetter, ok := field.(RelatedField); ok {
		relField = relFieldGetter.GetTargetField()
	} else {
		relField = defs.Primary()
	}

	var front, back = qs.compiler.Quote()

	var (
		condA_Alias = parentDefs.TableName()
		condB_Alias = tableName
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
		var fields = defs.Fields()
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

	var info = &FieldInfo{
		SourceField: field,
		Table: Table{
			Name:  tableName,
			Alias: aliases[len(aliases)-1],
		},
		Model:  foreignKey,
		Fields: includedFields,
		Chain:  chain,
	}

	var key = fmt.Sprintf("%s.%s", condA, condB)
	if _, ok := joinM[key]; ok {
		return info, nil
	}

	joinM[key] = true
	var join = JoinDef{
		TypeJoin: "LEFT JOIN",
		Table: Table{
			Name:  tableName,
			Alias: aliases[len(aliases)-1],
		},
		ConditionA: condA,
		Logic:      "=",
		ConditionB: condB,
	}

	return info, []JoinDef{join}
}

func addJoinForM2M(qs *QuerySet, manyToMany attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) (*FieldInfo, []JoinDef) {
	// TBA
	return nil, nil
}

func addJoinForO2O(qs *QuerySet, oneToOne attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) (*FieldInfo, []JoinDef) {
	var through = oneToOne.Through()
	if through == nil {
		return addJoinForFK(qs, oneToOne.Model(), parentDefs, parentField, field, chain, aliases, all, joinM)
	}
	return addJoinForM2M(qs, oneToOne, parentDefs, parentField, field, chain, aliases, all, joinM)
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
func (qs *QuerySet) Select(fields ...string) *QuerySet {
	qs = qs.Clone()

	var (
		fieldInfos = make([]FieldInfo, 0)
		joins      = make([]JoinDef, 0)
		joinM      = make(map[string]bool)
	)

	if len(fields) == 0 {
		fields = make([]string, 0, len(qs.queryInfo.Fields))
		for _, field := range qs.queryInfo.Fields {
			if ForSelectAll(field) {
				fields = append(fields, field.Name())
			}
		}
	} else if len(fields) > 0 && fields[0] == "*" {
		var f = make([]string, 0, len(qs.queryInfo.Fields)+(len(fields)-1))
		for _, field := range qs.queryInfo.Fields {
			if ForSelectAll(field) {
				f = append(f, field.Name())
			}
		}
		fields = append(f, fields[1:]...)
	}

	for _, selectedField := range fields {

		var allFields bool
		if strings.HasSuffix(strings.ToLower(selectedField), ".*") {
			selectedField = selectedField[:len(selectedField)-2]
			allFields = true
		}

		var current, parent, field, chain, aliases, isRelated, err = internal.WalkFields(
			qs.model, selectedField,
		)
		if err != nil {
			field, ok := qs.internals.Annotations[selectedField]
			if ok {
				fieldInfos = append(fieldInfos, FieldInfo{
					Table: Table{
						Name: qs.queryInfo.TableName,
					},
					Fields: []attrs.Field{field},
				})
				continue
			}

			panic(err)
		}

		//if !ForSelectAll(field) {
		//	panic(fmt.Errorf("field %q is not for use in queries", field.Name()))
		//}

		// The field might be a relation
		var (
			rel        attrs.Definer
			foreignKey = field.ForeignKey()
			oneToOne   = field.OneToOne()
			manyToMany = field.ManyToMany()
		)

		// If all fields of the relation are requested, we need to add the relation
		// to the join list. We also need to add the parent field to the chain.
		if (foreignKey != nil || oneToOne != nil || manyToMany != nil) && allFields {
			chain = append(chain, field.Name())
			aliases = append(aliases, internal.NewJoinAlias(
				field, current.FieldDefs().TableName(), chain,
			))
			parent = current
			isRelated = true

			switch {
			case foreignKey != nil:
				rel = foreignKey
			case oneToOne != nil:
				rel = oneToOne.Through()
				if rel == nil {
					rel = oneToOne.Model()
				}
			case manyToMany != nil:
				rel = manyToMany.Through()
			}
		}

		var defs = current.FieldDefs()
		var tableName = defs.TableName()
		if len(chain) > 0 && isRelated {

			var (
				info *FieldInfo
				join []JoinDef
			)

			/*
				This works fine for fetching related fields, I.E. Select("Parent.Parent.*"), but
				am unsure if this is the best way to do it. It looks messy, but does behave how it should.

			*/
			//	var (
			//		joinChainModel = qs.model
			//	)
			//
			//	for i := 0; i < len(chain); i++ {
			//		fieldName := chain[i]
			//		defs := joinChainModel.FieldDefs()
			//
			//		cField, ok := defs.Field(fieldName)
			//		if !ok {
			//			panic(fmt.Errorf("field %q not found in %T", fieldName, joinChainModel))
			//		}
			//
			//		nextModel := cField.ForeignKey()
			//		if nextModel == nil && cField.OneToOne() != nil {
			//			nextModel = cField.OneToOne().Model()
			//			if nextModel == nil {
			//				nextModel = cField.OneToOne().Through()
			//			}
			//		} else if nextModel == nil && cField.ManyToMany() != nil {
			//			nextModel = cField.ManyToMany().Through()
			//		}
			//		if nextModel == nil {
			//			panic(fmt.Errorf("field %q in %T is not a relation", fieldName, joinChainModel))
			//		}
			//
			//		alias := aliases[i]
			//		if joinM[alias] {
			//			joinChainModel = nextModel
			//			continue // already joined
			//		}
			//
			//		// This is the join we need to add
			//		switch {
			//		case cField.ForeignKey() != nil:
			//			_, j := addJoinForFK(qs, cField.ForeignKey(), defs, cField, nil, chain[:i+1], aliases[:i+1], true, joinM)
			//			joins = append(joins, j...)
			//		case cField.OneToOne() != nil:
			//			_, j := addJoinForO2O(qs, cField.OneToOne(), defs, cField, nil, chain[:i+1], aliases[:i+1], true, joinM)
			//			joins = append(joins, j...)
			//		case cField.ManyToMany() != nil:
			//			_, j := addJoinForM2M(qs, cField.ManyToMany(), defs, cField, nil, chain[:i+1], aliases[:i+1], true, joinM)
			//			joins = append(joins, j...)
			//		default:
			//			panic(fmt.Errorf("field %q is not a relation", cField.Name()))
			//		}
			//
			//		joinM[alias] = true
			//		joinChainModel = nextModel
			//	}

			var parentDefs = parent.FieldDefs()
			var parentField, ok = parentDefs.Field(chain[len(chain)-1])
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", chain[len(chain)-1], parent))
			}

			if rel == nil {
				foreignKey = parentField.ForeignKey()
				oneToOne = parentField.OneToOne()
				manyToMany = parentField.ManyToMany()
			}

			switch {
			case foreignKey != nil:
				info, join = addJoinForFK(qs, foreignKey, parentDefs, parentField, field, chain, aliases, allFields, joinM)
			case oneToOne != nil:
				info, join = addJoinForO2O(qs, oneToOne, parentDefs, parentField, field, chain, aliases, allFields, joinM)
			case manyToMany != nil:
				info, join = addJoinForM2M(qs, manyToMany, parentDefs, parentField, field, chain, aliases, allFields, joinM)
			default:
				panic(fmt.Errorf("field %q is not a relation", field.Name()))
			}

			if info != nil {
				fieldInfos = append(fieldInfos, *info)
				joins = append(joins, join...)
			}

			continue
		}

		fieldInfos = append(fieldInfos, FieldInfo{
			Model: current,
			Table: Table{
				Name: tableName,
			},
			Fields: []attrs.Field{field},
			Chain:  chain,
		})
	}

	qs.internals.Joins = joins
	qs.internals.Fields = fieldInfos

	return qs
}

// Filter is used to filter the results of a query.
//
// It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.
//
// The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.
//
// By default the `__exact` (=) operator is used, each where clause is separated by `AND`.
func (qs *QuerySet) Filter(key interface{}, vals ...interface{}) *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.Where = append(qs.internals.Where, express(key, vals...)...)
	return nqs
}

// Having is used to filter the results of a query after aggregation.
//
// It takes a key and a list of values as arguments and returns a new QuerySet with the filtered results.
//
// The key can be a field name (string), an expr.Expression (expr.Expression) or a map of field names to values.
func (qs *QuerySet) Having(key interface{}, vals ...interface{}) *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.Having = append(qs.internals.Having, express(key, vals...)...)
	return nqs
}

// GroupBy is used to group the results of a query.
//
// It takes a list of field names as arguments and returns a new QuerySet with the grouped results.
func (qs *QuerySet) GroupBy(fields ...string) *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.GroupBy, _ = qs.unpackFields(fields...)
	return nqs
}

// OrderBy is used to order the results of a query.
//
// It takes a list of field names as arguments and returns a new QuerySet with the ordered results.
//
// The field names can be prefixed with a minus sign (-) to indicate descending order.
func (qs *QuerySet) OrderBy(fields ...string) *QuerySet {
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
			qs.model, ord,
		)

		if err != nil {
			panic(err)
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
			alias = fmt.Sprintf(
				"%s_%s", tableAlias, vF.Alias(),
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
func (qs *QuerySet) Reverse() *QuerySet {
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
func (qs *QuerySet) Limit(n int) *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.Limit = n
	return nqs
}

// Offset is used to set the offset of the results returned by a query.
func (qs *QuerySet) Offset(n int) *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.Offset = n
	return nqs
}

// ForUpdate is used to lock the rows returned by a query for update.
//
// It is used to prevent other transactions from modifying the rows until the current transaction is committed or rolled back.
func (qs *QuerySet) ForUpdate() *QuerySet {
	var nqs = qs.Clone()
	nqs.internals.ForUpdate = true
	return nqs
}

// Distinct is used to select distinct rows from the results of a query.
//
// It is used to remove duplicate rows from the results.
func (qs *QuerySet) Distinct() *QuerySet {
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
func (qs *QuerySet) ExplicitSave() *QuerySet {
	var nqs = qs.Clone()
	nqs.explicitSave = true
	return nqs
}

func (qs *QuerySet) annotate(alias string, expr expr.Expression) {
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
func (qs *QuerySet) Annotate(aliasOrAliasMap interface{}, exprs ...expr.Expression) *QuerySet {
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

type Row struct {
	Object      attrs.Definer
	Annotations map[string]any
}

// All is used to retrieve all rows from the database.
//
// It returns a Query that can be executed to get the results, which is a slice of Row objects.
//
// Each Row object contains the model object and a map of annotations.
//
// If no fields are provided, it selects all fields from the model, see `Select()` for more details.
func (qs *QuerySet) All() Query[[]*Row] {
	if len(qs.internals.Fields) == 0 {
		qs = qs.Select("*")
	}

	var resultQuery = qs.compiler.BuildSelectQuery(
		context.Background(),
		qs,
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

	return &wrappedQuery[[][]interface{}, []*Row]{
		query: resultQuery,
		exec: func(q Query[[][]interface{}]) ([]*Row, error) {
			if qs.cached != nil && qs.useCache {
				return qs.cached.([]*Row), nil
			}

			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}

			var list = make([]*Row, len(results))

			for i, row := range results {
				obj := internal.NewObjectFromIface(qs.model)
				scannables := getScannableFields(qs.internals.Fields, obj)

				annotations := make(map[string]any)
				var annotator, _ = obj.(DataModel)

				for j, field := range scannables {
					f := field.(attrs.Field)
					val := row[j]

					if err := f.Scan(val); err != nil {
						return nil, errors.Wrapf(err, "failed to scan field %q in %T", f.Name(), obj)
					}

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

						if annotator != nil {
							annotator.SetQueryValue(alias, val)
						}
					}
				}

				list[i] = &Row{
					Object:      obj,
					Annotations: annotations,
				}
			}

			if qs.useCache {
				qs.cached = list
			}

			return list, nil
		},
	}
}

// ValuesList is used to retrieve a list of values from the database.
//
// It takes a list of field names as arguments and returns a ValuesListQuery.
func (qs *QuerySet) ValuesList(fields ...string) ValuesListQuery {

	qs = qs.Select(fields...)

	var resultQuery = qs.compiler.BuildSelectQuery(
		context.Background(),
		qs,
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

	return &wrappedQuery[[][]interface{}, [][]any]{
		query: resultQuery,
		exec: func(q Query[[][]interface{}]) ([][]any, error) {
			if qs.cached != nil && qs.useCache {
				return qs.cached.([][]any), nil
			}

			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}

			var list = make([][]any, len(results))
			for i, row := range results {
				var obj = internal.NewObjectFromIface(qs.model)
				var fields = getScannableFields(qs.internals.Fields, obj)
				var values = make([]any, len(fields))
				for j, field := range fields {
					var f = field.(attrs.Field)
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
		},
	}
}

// Aggregate is used to perform aggregation on the results of a query.
//
// It takes a map of field names to expr.Expressions as arguments and returns a Query that can be executed to get the results.
func (qs *QuerySet) Aggregate(annotations map[string]expr.Expression) Query[map[string]any] {
	qs = qs.Clone()
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

	query := qs.compiler.BuildSelectQuery(
		context.Background(),
		qs,
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

	return &wrappedQuery[[][]interface{}, map[string]any]{
		query: query,
		exec: func(q Query[[][]interface{}]) (map[string]any, error) {
			if qs.cached != nil && qs.useCache {
				return qs.cached.(map[string]any), nil
			}

			results, err := q.Exec()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return map[string]any{}, nil
			}

			scannables := getScannableFields(qs.internals.Fields, internal.NewObjectFromIface(qs.model))
			row := results[0]
			out := make(map[string]any)

			for i, field := range scannables {
				if vf, ok := field.(VirtualField); ok {
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
		},
	}
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
func (qs *QuerySet) Get() Query[*Row] {
	if len(qs.internals.Where) == 0 {
		panic(query_errors.ErrNoWhereClause)
	}

	qs = qs.Limit(MAX_GET_RESULTS)
	q := qs.All()

	return &wrappedQuery[[]*Row, *Row]{
		query: q,
		exec: func(q Query[[]*Row]) (*Row, error) {
			if qs.cached != nil && qs.useCache {
				return qs.cached.(*Row), nil
			}

			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}
			var resCnt = len(results)
			if resCnt == 0 {
				return nil, query_errors.ErrNoRows
			}
			if resCnt > 1 {
				var errResCnt string
				if MAX_GET_RESULTS == 0 || resCnt < MAX_GET_RESULTS {
					errResCnt = strconv.Itoa(resCnt)
				} else {
					errResCnt = strconv.Itoa(MAX_GET_RESULTS-1) + "+"
				}

				return nil, errors.Wrapf(
					query_errors.ErrMultipleRows,
					"multiple rows returned for %T: %s rows",
					qs.model, errResCnt,
				)
			}

			if qs.useCache {
				qs.cached = results[0]
			}

			return results[0], nil
		},
	}
}

// GetOrCreate is used to retrieve a single row from the database or create it if it does not exist.
//
// It returns the definer object and an error if any occurred.
//
// This method executes a transaction to ensure that the object is created only once.
//
// It panics if the queryset has no where clause.
func (qs *QuerySet) GetOrCreate(value attrs.Definer) (attrs.Definer, error) {

	if len(qs.internals.Where) == 0 {
		panic(query_errors.ErrNoWhereClause)
	}

	// Create a new transaction
	var ctx = context.Background()
	var transaction, err = qs.compiler.StartTransaction(ctx)
	if err != nil {
		return nil, err
	}

	defer transaction.Rollback()

	// Check if the object already exists
	qs.useCache = false
	var resultQuery = qs.Get()
	row, err := resultQuery.Exec()
	if err != nil {
		if errors.Is(err, query_errors.ErrNoRows) {
			goto create
		} else {
			return nil, err
		}
	}

	// Object already exists, return it and commit the transaction
	if row != nil {
		return row.Object, transaction.Commit()
	}

	// Object does not exist, create it
create:
	var createQuery = qs.Create(value)
	obj, err := createQuery.Exec()
	if err != nil {
		return nil, err
	}

	// Object was created successfully, commit the transaction
	return obj, transaction.Commit()
}

// First is used to retrieve the first row from the database.
//
// It returns a Query that can be executed to get the result, which is a Row object
// that contains the model object and a map of annotations.
func (qs *QuerySet) First() Query[*Row] {
	qs = qs.Limit(1)
	q := qs.All()

	return &wrappedQuery[[]*Row, *Row]{
		query: q,
		exec: func(q Query[[]*Row]) (*Row, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, query_errors.ErrNoRows
			}
			return results[0], nil
		},
	}
}

// Last is used to retrieve the last row from the database.
//
// It reverses the order of the results and then calls First to get the last row.
//
// It returns a Query that can be executed to get the result, which is a Row object
// that contains the model object and a map of annotations.
func (qs *QuerySet) Last() Query[*Row] {
	var nqs = qs.Reverse()
	return nqs.First()
}

// Exists is used to check if any rows exist in the database.
//
// It returns a Query that can be executed to get the result,
// which is a boolean indicating if any rows exist.
func (qs *QuerySet) Exists() ExistsQuery {
	qs = qs.Clone()

	var resultQuery = qs.compiler.BuildCountQuery(
		context.Background(),
		qs,
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		1,
		0,
	)

	return &wrappedQuery[int64, bool]{
		query: resultQuery,
		exec: func(q Query[int64]) (bool, error) {
			var exists, err = q.Exec()
			if err != nil {
				return false, err
			}
			return exists > 0, nil
		},
	}
}

// Count is used to count the number of rows in the database.
//
// It returns a CountQuery that can be executed to get the result, which is an int64 indicating the number of rows.
func (qs *QuerySet) Count() CountQuery {

	qs = qs.Clone()

	return qs.compiler.BuildCountQuery(
		context.Background(),
		qs,
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		qs.internals.Limit,
		qs.internals.Offset,
	)
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
func (qs *QuerySet) Create(value attrs.Definer) Query[attrs.Definer] {

	// Check if the object is a saver
	// If it is, we can use the Save method to save the object
	if saver, ok := value.(models.Saver); ok && !qs.explicitSave {
		return &QueryObject[attrs.Definer]{
			model:    value,
			compiler: qs.compiler,
			exec: func(sql string, args ...any) (attrs.Definer, error) {
				if err := sendSignal(SignalPreModelSave, value, qs.compiler); err != nil {
					return nil, err
				}

				var err = saver.Save(context.Background())
				if err != nil {
					return nil, err
				}

				if err := sendSignal(SignalPostModelSave, value, qs.compiler); err != nil {
					return nil, err
				}
				return saver.(attrs.Definer), nil
			},
		}
	}

	qs = qs.Clone()

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
		qs,
		info,
		defs.Primary(),
		values,
	)

	return &wrappedQuery[[]interface{}, attrs.Definer]{
		query: resultQuery,
		exec: func(q Query[[]interface{}]) (attrs.Definer, error) {

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
					return nil, errors.Wrapf(
						err,
						"failed to set field %q in %T",
						f.Name(), newObj,
					)
				}
			}

			// Execute the create query
			var results, err = q.Exec()
			if err != nil {
				return nil, err
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
					return nil, errors.Wrapf(
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
					return nil, errors.Wrapf(
						query_errors.ErrLastInsertId,
						"expected %d results returned after insert, got %d",
						len(scannables), len(results),
					)
				}

				var idx = 0
				if prim != nil {
					var id = results[0].(int64)
					if err := prim.Scan(id); err != nil {
						return nil, errors.Wrapf(
							err, "failed to scan primary key %q in %T",
							prim.Name(), newObj,
						)
					}
					idx++
				}

				for i, field := range scannables {
					var f = field.(attrs.Field)
					var val = results[i+idx]

					if err := f.Scan(val); err != nil {
						return nil, errors.Wrapf(
							err,
							"failed to scan field %q in %T",
							f.Name(), newObj,
						)
					}
				}
			}

			return newObj, nil
		},
	}
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
func (qs *QuerySet) Update(value attrs.Definer) CountQuery {
	qs = qs.Clone()

	if len(qs.internals.Where) == 0 && !qs.explicitSave {
		var (
			defs            = value.FieldDefs()
			primary         = defs.Primary()
			primaryVal, err = primary.Value()
		)

		if err != nil {
			panic(fmt.Errorf("failed to get value for field %q: %w", primary.Name(), err))
		}

		if _, ok := value.(models.Saver); ok && !fields.IsZero(primaryVal) {
			return &QueryObject[int64]{
				model:    value,
				compiler: qs.compiler,
				exec: func(sql string, args ...any) (int64, error) {
					if err := sendSignal(SignalPreModelSave, value, qs.compiler); err != nil {
						return 0, err
					}

					var err = value.(models.Saver).Save(context.Background())
					if err != nil {
						return 0, err
					}

					if err := sendSignal(SignalPostModelSave, value, qs.compiler); err != nil {
						return 0, err
					}
					return 1, nil
				},
			}
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

		info.Fields = append(info.Fields, field)
		values = append(values, value)
	}

	var resultQuery = qs.compiler.BuildUpdateQuery(
		context.Background(),
		qs,
		info,
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
		values,
	)

	return resultQuery
}

// Delete is used to delete an object from the database.
//
// It returns a CountQuery that can be executed to get the result, which is the number of rows affected.
func (qs *QuerySet) Delete() CountQuery {
	qs = qs.Clone()

	var resultQuery = qs.compiler.BuildDeleteQuery(
		context.Background(),
		qs,
		qs.internals.Where,
		qs.internals.Joins,
		qs.internals.GroupBy,
	)

	return resultQuery
}

func getScannableFields(fields []FieldInfo, root attrs.Definer) []any {
	var listSize = 0
	for _, info := range fields {
		listSize += len(info.Fields)
	}

	var scannables = make([]any, 0, listSize)
	var instances = map[string]attrs.Definer{"": root}
	for _, info := range fields {
		if info.SourceField == nil {
			defs := root.FieldDefs()
			for _, f := range info.Fields {

				if _, ok := f.(VirtualField); ok && info.Model == nil {
					// If field is virtual and not bound to a model, just scan it directly
					scannables = append(scannables, f)
					continue
				}

				field, ok := defs.Field(f.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", f.Name(), root))
				}
				scannables = append(scannables, field)
			}
			continue
		}

		// Walk chain
		var parentKey string
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
				if i == len(info.Chain)-1 {
					obj = internal.NewObjectFromIface(info.Model)
				} else {
					obj = internal.NewObjectFromIface(field.Rel())
				}
				if err := field.SetValue(obj, true); err != nil {
					panic(fmt.Errorf("failed to set relation %q: %w", field.Name(), err))
				}
				instances[key] = obj
			}

			parentKey = key
		}

		var final = instances[parentKey]
		var finalDefs = final.FieldDefs()
		for _, f := range info.Fields {
			field, ok := finalDefs.Field(f.Name())
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", f.Name(), final))
			}
			scannables = append(scannables, field)
		}
	}

	return scannables
}
