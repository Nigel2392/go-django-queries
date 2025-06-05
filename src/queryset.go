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
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/pkg/errors"

	_ "unsafe"
)

const (
	// Maximum number of results to return when using the `Get` method.
	//
	// Also the maximum number of results to return when querying the database
	// inside of the `String` method.
	MAX_GET_RESULTS = 21

	// Maximum default number of results to return when using the `All` method.
	//
	// This can be overridden by the `Limit` method.
	MAX_DEFAULT_RESULTS = 1000
)

// QUERYSET_USE_CACHE_DEFAULT is the default value for the useCache field in the QuerySet.
//
// It is used to determine whether the QuerySet should cache the results of the
// latest query until the QuerySet is modified.
var QUERYSET_USE_CACHE_DEFAULT = true

// Basic information about the model used in the QuerySet.
// It contains the model's meta information, primary key field, all fields,
// and the table name.
type modelInfo struct {
	Meta      attrs.ModelMeta
	Primary   attrs.FieldDefinition
	Fields    []attrs.FieldDefinition
	TableName string
}

// Internals contains the internal state of the QuerySet.
//
// It includes all nescessary information for
// the compiler to build a query out of.
type QuerySetInternals struct {
	Model       modelInfo
	Annotations map[string]*queryField[any]
	Fields      []*FieldInfo[attrs.FieldDefinition]
	Where       []expr.ClauseExpression
	Having      []expr.ClauseExpression
	Joins       []JoinDef
	GroupBy     []FieldInfo[attrs.FieldDefinition]
	OrderBy     []OrderBy
	Limit       int
	Offset      int
	ForUpdate   bool
	Distinct    bool

	joinsMap map[string]bool

	// a pointer to the annotations field info
	// to avoid having to create a new one every time
	// an annotation is added
	//
	// this is not cloned to prevent
	// a clone from changing the annotations
	annotations *FieldInfo[attrs.FieldDefinition]
}

func (i *QuerySetInternals) addJoin(join JoinDef) {
	if i.joinsMap == nil {
		i.joinsMap = make(map[string]bool)
	}

	var key = join.JoinDefCondition.String()
	if _, exists := i.joinsMap[key]; !exists {
		i.joinsMap[key] = true
		i.Joins = append(i.Joins, join)
	}
}

// ObjectsFunc is a function type that takes a model of type T and returns a QuerySet for that model.
//
// It is used to create a new QuerySet for a model which is automatically initialized with a transaction.
//
// See [RunInTransaction] for more details.
type ObjectsFunc[T attrs.Definer] func(model T) *QuerySet[T]

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
	internals    *QuerySetInternals
	model        attrs.Definer
	compiler     QueryCompiler
	AliasGen     *alias.Generator
	explicitSave bool
	latestQuery  QueryInfo
	useCache     bool
	cached       any
}

// GetQuerySet creates a new QuerySet for the given model.
//
// If the model implements the QuerySetDefiner interface,
// it will use the GetQuerySet method to get the initial QuerySet.
//
// A model should use Objects[T](model) to get the default QuerySet inside of it's
// GetQuerySet method. If not, it will recursively call itself.
//
// See [Objects] for more details.
func GetQuerySet[T attrs.Definer](model T) *QuerySet[T] {
	if m, ok := any(model).(QuerySetDefiner); ok {

		var qs = m.GetQuerySet()
		qs = qs.Clone()
		return ChangeObjectsType[attrs.Definer, T](qs)
	}

	return Objects(model)
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
func Objects[T attrs.Definer](model T, database ...string) *QuerySet[T] {
	model = attrs.NewObject[T](model)
	var modelV = reflect.ValueOf(model)

	if !modelV.IsValid() {
		panic("QuerySet: model is not a valid value")
	}

	if modelV.IsNil() {
		panic("QuerySet: model is nil")
	}

	var defaultDb = django.APPVAR_DATABASE
	if len(database) > 1 {
		panic("QuerySet: too many databases provided")
	}

	// If the model implements the QuerySetDatabaseDefiner interface,
	// it will use the QuerySetDatabase method to get the default database.
	// Function arguments still take precedence however.
	if m, ok := any(model).(QuerySetDatabaseDefiner); ok && len(database) == 0 {
		defaultDb = m.QuerySetDatabase()
	}

	// Arguments take precedence over the default database
	if len(database) == 1 {
		defaultDb = database[0]
	}

	var (
		meta        = attrs.GetModelMeta(model)
		definitions = meta.Definitions()
		primary     = definitions.Primary()
		tableName   = definitions.TableName()
	)

	if tableName == "" {
		panic(query_errors.ErrNoTableName)
	}

	var qs = &QuerySet[T]{
		model:    model,
		AliasGen: alias.NewGenerator(),
		internals: &QuerySetInternals{
			Model: modelInfo{
				Meta:      meta,
				Primary:   primary,
				Fields:    definitions.Fields(),
				TableName: tableName,
			},
			Annotations: make(map[string]*queryField[any]),
			Where:       make([]expr.ClauseExpression, 0),
			Having:      make([]expr.ClauseExpression, 0),
			Joins:       make([]JoinDef, 0),
			GroupBy:     make([]FieldInfo[attrs.FieldDefinition], 0),
			OrderBy:     make([]OrderBy, 0),
			Limit:       MAX_DEFAULT_RESULTS,
			Offset:      0,
		},

		// enable queryset caching by default
		// this can result in race conditions in some rare edge cases
		// but is generally safe to use
		useCache: QUERYSET_USE_CACHE_DEFAULT,
	}
	qs.compiler = Compiler(model, defaultDb)

	// Allow the model to change the QuerySet
	if c, ok := any(model).(QuerySetChanger); ok {
		var changed = c.ChangeQuerySet(
			ChangeObjectsType[T, attrs.Definer](qs),
		)
		qs = ChangeObjectsType[attrs.Definer, T](changed)
	}

	return qs
}

// Change the type of the objects in the QuerySet.
//
// Mostly used to change the type of the QuerySet
// from the generic QuerySet[attrs.Definer] to a concrete non-interface type
//
// Some things to note:
// - This does not clone the QuerySet
// - If the type mismatches and is not assignable, it will panic.
func ChangeObjectsType[OldT, NewT attrs.Definer](qs *QuerySet[OldT]) *QuerySet[NewT] {
	if _, ok := qs.model.(NewT); !ok {
		panic(fmt.Errorf(
			"ChangeObjectsType: cannot change QuerySet type from %T to %T: %w",
			qs.model, new(NewT),
			query_errors.ErrTypeMismatch,
		))
	}

	return &QuerySet[NewT]{
		AliasGen:     qs.AliasGen,
		model:        qs.model,
		compiler:     qs.compiler,
		explicitSave: qs.explicitSave,
		useCache:     qs.useCache,
		cached:       qs.cached,
		internals:    qs.internals,
	}
}

func RunInTransaction[T attrs.Definer](ctx context.Context, fn func(NewQuerySet ObjectsFunc[T]) error) error {
	var (
		comitted             bool
		panicFromNewQuerySet error
		transaction          Transaction
		dbName               string
	)

	// a constructor function to create a new QuerySet with the given model
	// and then bind the transaction to it.
	var newQuerySetFunc = func(model T) *QuerySet[T] {
		var (
			err error
			qs  = GetQuerySet(model)
		)

		// if there was no transaction started yet, start a new one
		// otherwise, let the compiler wrap and handle the transaction
		if transaction == nil {
			// set the database name to the one used by the compiler
			dbName = qs.compiler.DatabaseName()
			transaction, err = qs.compiler.StartTransaction(ctx)
		} else {
			// a transaction cannot be started if the database name is different
			// cross-database transactions are not supported
			var databaseName = qs.compiler.DatabaseName()
			if dbName != databaseName {
				panicFromNewQuerySet = fmt.Errorf(
					"RunInTransaction, %q != %q: %w",
					dbName, databaseName,
					query_errors.ErrCrossDatabaseTransaction,
				)
				panic(panicFromNewQuerySet)
			}

			transaction, err = qs.compiler.WithTransaction(transaction)
		}

		if err != nil {
			panicFromNewQuerySet = fmt.Errorf(
				"RunInTransaction: failed to start transaction: %w",
				err,
			)
			panic(panicFromNewQuerySet)
		}

		return qs
	}

	// rollback the transaction if anything bad happens or the transaction is not committed.
	// this should do nothing if the transaction is already committed.
	defer func() {
		if rec := recover(); rec != nil {
			logger.Errorf("RunInTransaction: panic recovered: %v", rec)
		}

		if transaction != nil && !comitted {
			if err := transaction.Rollback(); err != nil {
				logger.Errorf("RunInTransaction: failed to rollback transaction: %v", err)
			}
		}
	}()

	// if the function returns an error, the transaction will be rolled back
	var err = fn(newQuerySetFunc)
	if err != nil {
		return errors.Wrap(err, "RunInTransaction: function returned an error")
	}

	// if the transaction is nil, it means that no transaction was started
	// i.e. no newQuerySetFunc was called
	if transaction == nil {
		return query_errors.ErrNoTransaction
	}

	// commit the transaction if everything went well
	err = transaction.Commit()
	if err != nil {
		return errors.Wrap(err, "RunInTransaction: failed to commit transaction")
	}
	comitted = true

	return nil
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

// WithTransaction wraps the transaction and binds it to the QuerySet compiler.
func (qs *QuerySet[T]) WithTransaction(tx Transaction) (Transaction, error) {
	return qs.compiler.WithTransaction(tx)
}

// Clone creates a new QuerySet with the same parameters as the original one.
//
// It is used to create a new QuerySet with the same parameters as the original one, so that the original one is not modified.
//
// It is a shallow clone, underlying values like `*queries.Expr` are not cloned and have built- in immutability.
func (qs *QuerySet[T]) Clone() *QuerySet[T] {

	return &QuerySet[T]{
		model:    qs.model,
		AliasGen: qs.AliasGen.Clone(),
		internals: &QuerySetInternals{
			Model:       qs.internals.Model,
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

			// annotations are not cloned
			// this is to prevent the previous annotations
			// from being modified by the cloned QuerySet
		},
		explicitSave: qs.explicitSave,
		useCache:     qs.useCache,
		compiler:     qs.compiler,

		// do not copy the cached value
		// changing the queryset should automatically
		// invalidate the cache
	}
}

// Prefix sets the prefix for the alias generator
func (qs *QuerySet[T]) Prefix(prefix string) *QuerySet[T] {
	qs.AliasGen.Prefix = prefix
	return qs
}

// Return the string representation of the QuerySet.
//
// It shows a truncated list of the first 20 results, or an error if one occurred.
//
// This method WILL query the database!
func (qs *QuerySet[T]) String() string {
	var sb = strings.Builder{}
	sb.WriteString("QuerySet{")

	qs = qs.Clone()
	qs = qs.Limit(MAX_GET_RESULTS)

	var rows, err = qs.All()
	if err != nil {
		sb.WriteString("Error: ")
		sb.WriteString(err.Error())
		sb.WriteString("}")
		return sb.String()
	}

	if len(rows) == 0 {
		sb.WriteString("<empty>")
		sb.WriteString("}")
		return sb.String()
	}

	for i, row := range rows {
		if i == MAX_GET_RESULTS {
			sb.WriteString("... (truncated)")
			break
		}

		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(
			attrs.ToString(row.Object),
		)
	}

	sb.WriteString("}")
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
			sb.WriteString(string(join.TypeJoin))
			sb.WriteString(" ")
			if join.Table.Alias == "" {
				sb.WriteString(join.Table.Name)
			} else {
				sb.WriteString(join.Table.Alias)
			}
			sb.WriteString(" ON ")
			var cond = join.JoinDefCondition
			for cond != nil {

				var colA, _ = qs.compiler.FormatColumn(
					&cond.ConditionA,
				)
				var colB, _ = qs.compiler.FormatColumn(
					&cond.ConditionB,
				)

				sb.WriteString(colA)
				sb.WriteString(" ")
				sb.WriteString(string(cond.Operator))
				sb.WriteString(" ")
				sb.WriteString(colB)

				cond = cond.Next

				if cond != nil {
					sb.WriteString(" AND ")
				}
			}
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
func (qs *QuerySet[T]) unpackFields(fields ...any) (infos []FieldInfo[attrs.FieldDefinition], hasRelated bool) {
	infos = make([]FieldInfo[attrs.FieldDefinition], 0, len(qs.internals.Fields))
	var info = FieldInfo[attrs.FieldDefinition]{
		Table: Table{
			Name: qs.internals.Model.TableName,
		},
		Fields: make([]attrs.FieldDefinition, 0),
	}

	if len(fields) == 0 || len(fields) == 1 && fields[0] == "*" {
		fields = make([]any, 0, len(qs.internals.Model.Fields))
		for _, field := range qs.internals.Model.Fields {
			fields = append(fields, field.Name())
		}
	}

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
		default:
			panic(fmt.Errorf("Select: invalid field type %T, can be one of [string, NamedExpression]", v))
		}

		var current, parent, field, chain, aliases, isRelated, err = internal.WalkFields(
			qs.model, selectedField, qs.AliasGen,
		)
		if err != nil {
			field, ok := qs.internals.Annotations[selectedField]
			if ok {
				infos = append(infos, FieldInfo[attrs.FieldDefinition]{
					Table: Table{
						Name: qs.internals.Model.TableName,
					},
					Fields: []attrs.FieldDefinition{field},
				})
				continue
			}

			panic(err)
		}

		if expr, ok := selectedFieldObj.(expr.NamedExpression); ok {
			field = &exprField{
				Field: field,
				expr:  expr,
			}
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

			var relMeta = attrs.GetModelMeta(current)
			var relDefs = relMeta.Definitions()
			var tableName = relDefs.TableName()
			infos = append(infos, FieldInfo[attrs.FieldDefinition]{
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

func (qs *QuerySet[T]) attrFields(obj attrs.Definer) (attrs.Definitions, []attrs.Field) {
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
	return defs, fields
}

func (qs *QuerySet[T]) addJoinForFK(foreignKey attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]*FieldInfo[attrs.FieldDefinition], []JoinDef) {
	var (
		target      = foreignKey.Model()
		relField    = foreignKey.Field()
		targetDefs  = target.FieldDefs()
		targetTable = targetDefs.TableName()
		condA_Alias = parentDefs.TableName()
		condB_Alias = targetTable
	)

	if relField == nil {
		relField = targetDefs.Primary()
	}

	if len(aliases) == 1 {
		condB_Alias = aliases[0]
	} else if len(aliases) > 1 {
		condA_Alias = aliases[len(aliases)-2]
		condB_Alias = aliases[len(aliases)-1]
	}

	var condA = expr.TableColumn{
		TableOrAlias: condA_Alias,
		FieldColumn:  parentField,
	}
	var condB = expr.TableColumn{
		TableOrAlias: condB_Alias,
		FieldColumn:  relField,
	}

	var includedFields []attrs.FieldDefinition
	if all {
		includedFields = ForSelectAllFields[attrs.FieldDefinition](
			targetDefs.Fields(),
		)
	} else {
		includedFields = []attrs.FieldDefinition{field}
	}

	var info = &FieldInfo[attrs.FieldDefinition]{
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

	var joinCond = &JoinDefCondition{
		ConditionA: condA,
		Operator:   expr.EQ,
		ConditionB: condB,
	}

	var key = joinCond.String()
	if _, ok := joinM[key]; ok {
		return []*FieldInfo[attrs.FieldDefinition]{info}, nil
	}

	joinM[key] = true

	var join = JoinDef{
		TypeJoin: TypeJoinLeft,
		Table: Table{
			Name:  targetTable,
			Alias: aliases[len(aliases)-1],
		},
		JoinDefCondition: joinCond,
	}

	return []*FieldInfo[attrs.FieldDefinition]{info}, []JoinDef{join}
}

func (qs *QuerySet[T]) addJoinForM2M(manyToMany attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]*FieldInfo[attrs.FieldDefinition], []JoinDef) {
	var through = manyToMany.Through()
	if through == nil {
		panic(fmt.Errorf("manyToMany relation %T.%s does not have a through table", manyToMany.Model(), field.Name()))
	}

	// through model info
	var (
		throughModel = through.Model()
		throughMeta  = attrs.GetModelMeta(throughModel)
		throughDefs  = throughMeta.Definitions()
		throughTable = throughDefs.TableName()

		target      = manyToMany.Model()
		targetDefs  = target.FieldDefs()
		targetTable = targetDefs.TableName()
		// targetField = getTargetField()
	)

	throughSourceField, ok := throughDefs.Field(through.SourceField())
	if !ok {
		panic(fmt.Errorf("field %q not found in %T", through.SourceField(), throughModel))
	}
	throughTargetField, ok := throughDefs.Field(through.TargetField())
	if !ok {
		panic(fmt.Errorf("field %q not found in %T", through.TargetField(), throughModel))
	}

	var parentAlias string
	if len(aliases) > 1 {
		parentAlias = aliases[len(aliases)-2]
	} else {
		parentAlias = parentDefs.TableName()
	}

	var (
		alias        = aliases[len(aliases)-1]
		aliasThrough = fmt.Sprintf("%s_through", alias)
		targetField  = getTargetField(
			throughTargetField,
			targetDefs,
		)
	)

	// JOIN through table
	var join1 = JoinDef{
		TypeJoin: TypeJoinLeft,
		Table: Table{
			Name:  throughTable,
			Alias: aliasThrough,
		},
		JoinDefCondition: &JoinDefCondition{
			Operator: expr.EQ,
			ConditionA: expr.TableColumn{
				TableOrAlias: parentAlias,
				FieldColumn:  parentField,
			},
			ConditionB: expr.TableColumn{
				TableOrAlias: aliasThrough,
				FieldColumn:  throughSourceField,
			},
		},
	}

	// JOIN target table
	var join2 = JoinDef{
		TypeJoin: TypeJoinLeft,
		Table: Table{
			Name:  targetTable,
			Alias: alias,
		},
		JoinDefCondition: &JoinDefCondition{
			Operator: expr.EQ,
			ConditionA: expr.TableColumn{
				TableOrAlias: aliasThrough,
				FieldColumn:  throughTargetField,
			},
			ConditionB: expr.TableColumn{
				TableOrAlias: alias,
				FieldColumn:  targetField,
			},
		},
	}

	// Prevent duplicate joins
	var (
		joins = make([]JoinDef, 0, 2)
		key1  = join1.JoinDefCondition.String()
		key2  = join2.JoinDefCondition.String()
	)
	if _, ok := joinM[key1]; !ok {
		joins = append(joins, join1)
		joinM[key1] = true
	}
	if _, ok := joinM[key2]; !ok {
		joins = append(joins, join2)
		joinM[key2] = true
	}

	var includedFields []attrs.FieldDefinition
	if all {
		includedFields = ForSelectAllFields[attrs.FieldDefinition](
			targetDefs.Fields(),
		)
	} else {
		includedFields = []attrs.FieldDefinition{field}
	}

	return []*FieldInfo[attrs.FieldDefinition]{{
		RelType:     manyToMany.Type(),
		SourceField: field,
		Model:       target,
		Table: Table{
			Name:  targetTable,
			Alias: alias,
		},
		Fields: includedFields,
		Chain:  chain,
		Through: &FieldInfo[attrs.FieldDefinition]{
			RelType:     manyToMany.Type(),
			SourceField: field,
			Model:       throughModel,
			Table: Table{
				Name:  throughTable,
				Alias: aliasThrough,
			},
			Fields: throughDefs.Fields(),
		},
	}}, joins

}

func (qs *QuerySet[T]) addJoinForO2O(oneToOne attrs.Relation, parentDefs attrs.Definitions, parentField attrs.Field, field attrs.Field, chain, aliases []string, all bool, joinM map[string]bool) ([]*FieldInfo[attrs.FieldDefinition], []JoinDef) {
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

	qs.internals.Fields = make([]*FieldInfo[attrs.FieldDefinition], 0)
	if qs.internals.joinsMap == nil {
		qs.internals.joinsMap = make(map[string]bool, len(qs.internals.Joins))
	}

	if len(fields) == 0 {
		fields = []any{"*"}
	}

fieldsLoop:
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
		case *FieldInfo[attrs.FieldDefinition]:
			qs.internals.Fields = append(qs.internals.Fields, v)
			continue fieldsLoop
		default:
			panic(fmt.Errorf("Select: invalid field type %T, can be one of [string, NamedExpression]", v))
		}

		var allFields bool
		if strings.HasSuffix(selectedField, "*") {
			selectedField = strings.TrimSuffix(selectedField, "*")
			selectedField = strings.TrimSuffix(selectedField, ".")
			allFields = true
		}

		if selectedField == "" && !allFields {
			panic(fmt.Errorf("Select: empty field path, cannot select field \"\""))
		}

		if selectedField == "" && allFields {
			qs.internals.Fields = append(qs.internals.Fields, &FieldInfo[attrs.FieldDefinition]{
				Model: qs.model,
				Table: Table{
					Name: qs.internals.Model.TableName,
				},
				Fields: ForSelectAllFields[attrs.FieldDefinition](qs.model),
			})
			continue fieldsLoop
		}

		var current, parent, field, chain, aliases, isRelated, err = internal.WalkFields(
			qs.model, selectedField, qs.AliasGen,
		)
		if err != nil {
			field, ok := qs.internals.Annotations[selectedField]
			if ok {
				qs.internals.Fields = append(qs.internals.Fields, &FieldInfo[attrs.FieldDefinition]{
					Table: Table{
						Name: qs.internals.Model.TableName,
					},
					Fields: []attrs.FieldDefinition{field},
				})
				continue fieldsLoop
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
			var (
				meta = attrs.GetModelMeta(rel.Model())
				defs = meta.Definitions()
			)
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
				infos []*FieldInfo[attrs.FieldDefinition]
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

			continue fieldsLoop
		}

		qs.internals.Fields = append(qs.internals.Fields, &FieldInfo[attrs.FieldDefinition]{
			Model: current,
			Table: Table{
				Name: tableName,
			},
			Fields: []attrs.FieldDefinition{field},
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
func (qs *QuerySet[T]) GroupBy(fields ...any) *QuerySet[T] {
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

		if alias != "" {
			tableAlias = ""
			field = nil
		}

		nqs.internals.OrderBy = append(nqs.internals.OrderBy, OrderBy{
			Column: expr.TableColumn{
				TableOrAlias: tableAlias,
				FieldColumn:  field,
				FieldAlias:   alias,
			},
			Desc: desc,
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
			Column: ord.Column,
			Desc:   !ord.Desc,
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
	// If the has not been added to the annotations, we need to add it
	if qs.internals.annotations == nil {
		qs.internals.annotations = &FieldInfo[attrs.FieldDefinition]{
			Table: Table{
				Name: qs.internals.Model.TableName,
			},
			Fields: make([]attrs.FieldDefinition, 0, len(qs.internals.Annotations)),
		}
		qs.internals.Fields = append(
			qs.internals.Fields, qs.internals.annotations,
		)
	}

	// Add the field to the annotations
	var field = newQueryField[any](alias, expr)
	qs.internals.Annotations[alias] = field
	qs.internals.annotations.Fields = append(
		qs.internals.annotations.Fields, field,
	)
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

// Scope is used to apply a scope to the QuerySet.
//
// It takes a function that modifies the QuerySet as an argument and returns a QuerySet with the applied scope.
//
// The queryset is modified in place, so the original QuerySet is changed.
func (qs *QuerySet[T]) Scope(scopes ...func(*QuerySet[T]) *QuerySet[T]) *QuerySet[T] {
	var (
		newQs   = qs.Clone()
		changed bool
	)
	for _, scopeFunc := range scopes {
		newQs = scopeFunc(newQs)
		if newQs != nil {
			changed = true
		}
	}
	if changed {
		*qs = *newQs
	}
	return qs
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
		qs.internals,
	)
	qs.latestQuery = query

	return query
}

func (qs *QuerySet[T]) queryAggregate() CompiledQuery[[][]interface{}] {
	var dereferenced = *qs.internals
	dereferenced.OrderBy = nil     // no order by for aggregates
	dereferenced.Limit = 0         // no limit for aggregates
	dereferenced.Offset = 0        // no offset for aggregates
	dereferenced.ForUpdate = false // no for update for aggregates
	dereferenced.Distinct = false  // no distinct for aggregates
	var query = qs.compiler.BuildSelectQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		&dereferenced,
	)
	qs.latestQuery = query
	return query
}

func (qs *QuerySet[T]) queryCount() CompiledQuery[int64] {
	var q = qs.compiler.BuildCountQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals,
	)
	qs.latestQuery = q
	return q
}

// All is used to retrieve all rows from the database.
//
// It returns a Query that can be executed to get the results, which is a slice of Row objects.
//
// Each Row object contains the model object and a map of annotations.
//
// If no fields are provided, it selects all fields from the model, see `Select()` for more details.
func (qs *QuerySet[T]) All() (Rows[T], error) {
	if qs.cached != nil && qs.useCache {
		return qs.cached.([]*Row[T]), nil
	}

	var resultQuery = qs.queryAll()
	var results, err = resultQuery.Exec()
	if err != nil {
		return nil, err
	}

	var runActors = func(o attrs.Definer) error {
		if o == nil {
			return nil
		}
		return runActor(
			actsAfterQuery, o,
			ChangeObjectsType[T, attrs.Definer](qs),
		)
	}

	rows, err := newRows[T](
		qs.internals.Fields,
		internal.NewObjectFromIface(qs.model),
		runActors,
	)
	if err != nil {
		return nil, errors.Wrap(err, "QuerySet.All: failed to create rows")
	}

	for resultIndex, row := range results {
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
			datastore = annotator.DataStore()
		}

		for j, field := range scannables {
			f := field.field
			val := row[j]

			if err := f.Scan(val); err != nil {
				return nil, errors.Wrapf(err, "failed to scan field %q (%T) in %T", f.Name(), f, f.Instance())
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

				if datastore != nil {
					datastore.SetValue(alias, val)
				}
			}
		}

		var (
			uniqueValue any
			throughObj  attrs.Definer
		)

		// required in case the root object has a through relation bound to it
		if rows.hasRoot() {
			var rootRow = rows.rootRow(scannables)
			uniqueValue, err = GetUniqueKey(rootRow.field)
			switch {
			case err != nil && errors.Is(err, query_errors.ErrNoUniqueKey) && rows.hasMultiRelations:
				return nil, errors.Wrapf(
					err, "failed to get unique key for %T, but has multi relations",
					rootRow.object,
				)
			case err != nil && errors.Is(err, query_errors.ErrNoUniqueKey):
				// if no unique key is found, we can use the result index as a unique value
				// this is only valid for the root object, as it is not a relation
				uniqueValue = resultIndex + 1
			}

			// if the root object has a through relation
			// we should store it in the rows tree for
			// binding it to the root.
			throughObj = rootRow.through
		}

		// fake unique value for the root object is OK
		if uniqueValue == nil {
			uniqueValue = resultIndex + 1
		}

		// add the root object to the rows tree
		// this has to be done before adding possible duplicate relations
		rows.addRoot(
			uniqueValue, obj, throughObj, annotations,
		)

		for _, possibleDuplicate := range rows.possibleDuplicates {
			var chainParts = buildChainParts(
				scannables[possibleDuplicate.idx],
			)
			rows.addRelationChain(chainParts)
		}
	}

	return rows.compile(qs)
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

	qs.internals.Fields = make([]*FieldInfo[attrs.FieldDefinition], 0, len(annotations))

	for alias, expr := range annotations {
		qs.annotate(alias, expr)
	}

	var query = qs.queryAggregate()
	var results, err = query.Exec()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return map[string]any{}, nil
	}

	var (
		row        = results[0]
		out        = make(map[string]any)
		scannables = getScannableFields(
			qs.internals.Fields,
			internal.NewObjectFromIface(qs.model),
		)
	)

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
func (qs *QuerySet[T]) GetOrCreate(value T) (T, bool, error) {

	if len(qs.internals.Where) == 0 {
		panic(query_errors.ErrNoWhereClause)
	}

	// Create a new transaction if the queryset is not already in a transaction
	//
	// If the queryset is already in a transaction, that transaction will be used
	// automatically.
	var transaction Transaction
	var commitTransaction = func() error {
		if transaction == nil {
			return nil
		}
		return transaction.Commit()
	}

	if !qs.compiler.InTransaction() {
		var (
			err error
			ctx = context.Background()
		)

		transaction, err = qs.compiler.StartTransaction(ctx)
		if err != nil {
			return *new(T), false, err
		}

		defer transaction.Rollback()
	}

	// Check if the object already exists
	qs.useCache = false
	row, err := qs.Get()
	if err != nil {
		if errors.Is(err, query_errors.ErrNoRows) {
			goto create
		} else {
			return *new(T), false, err
		}
	}

	// Object already exists, return it and commit the transaction
	if row != nil {
		return row.Object, false, commitTransaction()
	}

	// Object does not exist, create it
create:
	obj, err := qs.Create(value)
	if err != nil {
		return *new(T), false, err
	}

	// Object was created successfully, commit the transaction
	return obj, true, commitTransaction()
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

	var dereferenced = *qs.internals
	dereferenced.Limit = 1  // limit to 1 row
	dereferenced.Offset = 0 // no offset for exists
	var resultQuery = qs.compiler.BuildCountQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		&dereferenced,
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

	var result, err = qs.BulkCreate([]T{value})
	if err != nil {
		return *new(T), err
	}

	var support = qs.compiler.SupportsReturning()
	if len(result) == 0 && support != SupportsReturningNone {
		return *new(T), query_errors.ErrNoRows
	}

	return result[0], nil
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

	return qs.BulkUpdate([]T{value}, expressions...)
}

// BulkCreate is used to create multiple objects in the database.
//
// It takes a list of definer objects as arguments and returns a Query that can be executed
// to get the result, which is a slice of the created objects.
func (qs *QuerySet[T]) BulkCreate(objects []T) ([]T, error) {
	var (
		values     = make([]any, 0, len(objects))
		attrFields = make([][]attrs.Field, 0, len(objects))
		infos      = make([]*FieldInfo[attrs.Field], 0, len(objects))
		primary    attrs.Field
	)

	for _, object := range objects {

		if err := runActor(actsBeforeCreate, object, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
			return nil, errors.Wrapf(
				err,
				"failed to run ActsBeforeCreate for %T",
				object,
			)
		}

		var defs = object.FieldDefs()
		var fields = defs.Fields()
		var infoFields = make([]attrs.Field, 0, len(fields))
		var info = &FieldInfo[attrs.Field]{
			Model: object,
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

			var isPrimary = field.IsPrimary()
			if isPrimary && primary == nil {
				primary = field
			}

			if isPrimary || !field.AllowEdit() {
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
		attrFields = append(attrFields, infoFields)
		info.Fields = slices.Clone(infoFields)
		infos = append(infos, info)
	}

	var support = qs.compiler.SupportsReturning()
	var resultQuery = qs.compiler.BuildCreateQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals,
		infos,
		values,
	)
	qs.latestQuery = resultQuery

	// Set the old values on the new object
	var resultList = make([]T, 0, len(objects))
	for _, fieldRow := range attrFields {
		var newObj = internal.NewObjectFromIface(qs.model)
		var newDefs = newObj.FieldDefs()

		for _, field := range fieldRow {
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

		resultList = append(resultList, newObj.(T))
	}

	// Execute the create query
	var results, err = resultQuery.Exec()
	if err != nil {
		return nil, err
	}

	// Check results & which returning method to use
	switch {
	case len(results) == 0 && support == SupportsReturningNone:
		for i, row := range resultList {
			var rVal = reflect.ValueOf(objects[i])
			if rVal.Kind() == reflect.Ptr {
				rVal.Elem().Set(reflect.ValueOf(row).Elem())
			}

			if err := runActor(actsAfterCreate, row, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to run ActsAfterCreate for %T",
					row,
				)
			}
		}

	case len(results) > 0 && support == SupportsReturningLastInsertId:

		for i, row := range resultList {
			var id = results[i][0].(int64)
			var rowDefs = row.FieldDefs()
			var prim = rowDefs.Primary()
			if err := prim.SetValue(id, true); err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to set primary key %q in %T",
					prim.Name(), row,
				)
			}

			//	if prim != nil {
			//		row = Setup[T](row)
			//	}

			var rVal = reflect.ValueOf(objects[i])
			if rVal.Kind() == reflect.Ptr {
				rVal.Elem().Set(reflect.ValueOf(row).Elem())
			}

			if err := runActor(actsAfterCreate, row, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to run ActsAfterCreate for %T",
					row,
				)
			}
		}

	case len(results) > 0 && support == SupportsReturningColumns:

		if len(results) != len(resultList) {
			return nil, errors.Wrapf(
				query_errors.ErrLastInsertId,
				"expected %d results returned after insert, got %d",
				len(resultList), len(results),
			)
		}

		for i, row := range resultList {
			var (
				scannables = getScannableFields([]*FieldInfo[attrs.Field]{infos[i]}, row)
				resLen     = len(results[i])
				newDefs    = row.FieldDefs()
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
				if err := prim.Scan(results[i][0]); err != nil {
					return nil, errors.Wrapf(
						err, "failed to scan primary key %q in %T",
						prim.Name(), row,
					)
				}
				idx++
			}

			for j, field := range scannables {
				var f = field.field
				var val = results[i][j+idx]

				if err := f.Scan(val); err != nil {
					return nil, errors.Wrapf(
						err,
						"failed to scan field %q in %T",
						f.Name(), row,
					)
				}
			}

			//	if prim != nil {
			//		row = Setup[T](row)
			//	}

			var rVal = reflect.ValueOf(objects[i])
			rVal.Elem().Set(reflect.ValueOf(row).Elem())

			if err := runActor(actsAfterCreate, row, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to run ActsAfterCreate for %T",
					row,
				)
			}
		}
	}

	return resultList, nil

}

// BulkUpdate is used to update multiple objects in the database.
//
// It takes a list of definer objects as arguments and any possible NamedExpressions.
// It does not try to call any save methods on the objects.
func (qs *QuerySet[T]) BulkUpdate(objects []T, expressions ...expr.NamedExpression) (int64, error) {

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

	var (
		infos = make([]UpdateInfo, 0, len(objects))
		where = slices.Clone(qs.internals.Where)
		joins = slices.Clone(qs.internals.Joins)
	)

	var (
		canBeforeUpdate bool
		canAfterUpdate  bool
	)

	if len(objects) > 0 {
		var obj = objects[0]
		_, canBeforeUpdate = any(obj).(ActsBeforeUpdate)
		_, canAfterUpdate = any(obj).(ActsAfterUpdate)
		_, canBeforeSave := any(obj).(ActsBeforeSave)
		_, canAfterSave := any(obj).(ActsAfterSave)
		canBeforeUpdate = canBeforeUpdate || canBeforeSave
		canAfterUpdate = canAfterUpdate || canAfterSave
	}

	var typ reflect.Type
	for _, obj := range objects {

		if typ == nil {
			typ = reflect.TypeOf(obj)
		} else if typ != reflect.TypeOf(obj) {
			panic(fmt.Errorf(
				"QuerySet: all objects must be of the same type, got %T and %T",
				typ, reflect.TypeOf(obj),
			))
		}

		if canBeforeUpdate {
			if err := runActor(actsBeforeUpdate, obj, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
				return 0, errors.Wrapf(
					err,
					"failed to run ActsBeforeUpdate for %T",
					obj,
				)
			}
		}

		var defs, fields = qs.attrFields(obj)
		var info = UpdateInfo{
			FieldInfo: FieldInfo[attrs.Field]{
				Model: obj,
				Table: Table{
					Name: defs.TableName(),
				},
				Fields: make([]attrs.Field, 0, len(fields)),
			},
			Values: make([]any, 0, len(fields)),
		}

		for _, field := range fields {
			var atts = field.Attrs()
			var v, ok = atts[attrs.AttrAutoIncrementKey]
			if ok && v.(bool) {
				continue
			}

			var isPrimary = field.IsPrimary()
			if isPrimary || !field.AllowEdit() {
				continue
			}

			if expr, ok := exprMap[field.Name()]; ok {
				info.FieldInfo.Fields = append(info.FieldInfo.Fields, &exprField{
					Field: field,
					expr:  expr,
				})
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

			info.FieldInfo.Fields = append(info.FieldInfo.Fields, field)
			info.Values = append(info.Values, value)
		}

		if len(qs.internals.Where) == 0 {
			var err error
			info.Where, err = GenerateObjectsWhereClause(obj)
			if err != nil {
				return 0, errors.Wrapf(
					err, "failed to generate where clause for %T",
					qs.model,
				)
			}
		} else {
			info.Where = where
		}

		if len(qs.internals.Joins) > 0 {
			info.Joins = joins
		}

		infos = append(infos, info)
	}

	var resultQuery = qs.compiler.BuildUpdateQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals,
		infos,
	)
	qs.latestQuery = resultQuery

	var res, err = resultQuery.Exec()
	if err != nil {
		return 0, err
	}

	if canAfterUpdate {
		for _, obj := range objects {
			if err := runActor(actsAfterUpdate, obj, ChangeObjectsType[T, attrs.Definer](qs)); err != nil {
				return 0, errors.Wrapf(
					err,
					"failed to run ActsAfterUpdate for %T",
					obj,
				)
			}
		}
	}

	return res, nil
}

// Delete is used to delete an object from the database.
//
// It returns a CountQuery that can be executed to get the result, which is the number of rows affected.
func (qs *QuerySet[T]) Delete(objects ...T) (int64, error) {

	if len(objects) > 0 {
		var where, err = GenerateObjectsWhereClause(objects...)
		if err != nil {
			return 0, errors.Wrapf(
				err, "failed to generate where clause for %T",
				qs.model,
			)
		}
		qs.internals.Where = append(qs.internals.Where, where...)
	}

	var resultQuery = qs.compiler.BuildDeleteQuery(
		context.Background(),
		ChangeObjectsType[T, attrs.Definer](qs),
		qs.internals,
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
	isThrough bool          // is this a through model field (many-to-many or one-to-one)
	through   attrs.Definer // the through field if this is a many-to-many or one-to-one relation
	chainPart string        // name of the field in the chain
	chainKey  string        // the chain up to this point, joined by "."
}

func getScannableFields[T attrs.FieldDefinition](fields []*FieldInfo[T], root attrs.Definer) []*scannableField {
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
		// handle through objects
		//
		// this has to be before the final fields are added - the logic
		// matches that in [FieldInfo.WriteFields].
		var throughObj attrs.Definer
		if info.Through != nil {
			var newObj = internal.NewObjectFromIface(info.Through.Model)
			var newDefs = newObj.FieldDefs()
			throughObj = newObj

			for _, f := range info.Through.Fields {
				var field, ok = newDefs.Field(f.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", f.Name(), newObj))
				}

				var throughField = &scannableField{
					isThrough: true,
					idx:       idx,
					object:    newObj,
					field:     field,
					relType:   info.RelType,
				}

				scannables = append(scannables, throughField)
				idx++
			}
		}

		// if isNil(reflect.ValueOf(info.SourceField)) {
		if any(info.SourceField) == any(*(new(T))) {
			defs := root.FieldDefs()
			for _, f := range info.Fields {
				if virt, ok := any(f).(VirtualField); ok && info.Model == nil {
					var attrField, ok = virt.(attrs.Field)
					if !ok {
						panic(fmt.Errorf("virtual field %q does not implement attrs.Field", f.Name()))
					}

					scannables = append(scannables, &scannableField{
						idx:     idx,
						field:   attrField,
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
					through: throughObj,
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
		var (
			parentScannable = rootScannable
			parentObj       = root

			parentKey string
		)
		for i, name := range info.Chain {
			key := strings.Join(info.Chain[:i+1], ".")
			parent := instances[parentKey]
			defs := parent.FieldDefs()
			field, ok := defs.Field(name)
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", name, parent))
			}

			var rel = field.Rel()
			var relType = rel.Type()
			if _, exists := instances[key]; !exists {
				var obj attrs.Definer
				if i == len(info.Chain)-1 {
					obj = internal.NewObjectFromIface(info.Model)
				} else {
					obj = internal.NewObjectFromIface(rel.Model())
				}

				// only set fk relations - the rest are added later
				// in the dedupe rows object.
				if relType == attrs.RelManyToOne {
					setRelatedObjects(
						name,
						relType,
						parent,
						[]Relation{&baseRelation{object: obj}},
					)
				}

				instances[key] = obj

				// Make the scannableField node for this relation link to its parent
				newParent := &scannableField{
					relType:   relType,
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
			cpy.through = throughObj
			scannables = append(scannables, &cpy)

			idx++
		}
	}

	return scannables
}
