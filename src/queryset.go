package queries

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/pkg/errors"
)

// -----------------------------------------------------------------------------
// QuerySet
// -----------------------------------------------------------------------------

const MAX_GET_RESULTS = 21

type Union func(*QuerySet) *QuerySet

type JoinDef struct {
	Table      string
	TypeJoin   string
	ConditionA string
	Logic      string
	ConditionB string
}

type FieldInfo struct {
	SourceField attrs.Field
	Model       attrs.Definer
	Table       string
	Chain       []string
	Fields      []attrs.Field
}

type OrderBy struct {
	Table string
	Field string
	Desc  bool
}

func (f *FieldInfo) WriteFields(sb *strings.Builder, quote string) {
	for i, field := range f.Fields {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(quote)
		sb.WriteString(f.Table)
		sb.WriteString(quote)
		sb.WriteString(".")
		sb.WriteString(quote)
		sb.WriteString(field.ColumnName())
		sb.WriteString(quote)
	}
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
	queryInfo *queryInfo
	model     attrs.Definer
	fields    []FieldInfo
	where     []Expression
	having    []Expression
	joins     []JoinDef
	groupBy   []FieldInfo
	orderBy   []OrderBy
	limit     int
	offset    int
	union     []Union
	forUpdate bool
	distinct  bool
	compiler  QueryCompiler
}

// Objects creates a new QuerySet for the given model.
//
// It panics if:
// - the model is nil
// - the base query info cannot be retrieved
//
// It returns a pointer to a new QuerySet.
//
// The model must implement the Definer interface.
func Objects(model attrs.Definer) *QuerySet {

	if model == nil {
		panic("QuerySet: model is nil")
	}

	var queryInfo, err = getBaseQueryInfo(model)
	if err != nil {
		panic(fmt.Errorf("QuerySet: %w", err))
	}

	if queryInfo == nil {
		panic("QuerySet: queryInfo is nil")
	}

	var qs = &QuerySet{
		model:     model,
		queryInfo: queryInfo,
		where:     make([]Expression, 0),
		having:    make([]Expression, 0),
		joins:     make([]JoinDef, 0),
		groupBy:   make([]FieldInfo, 0),
		orderBy:   make([]OrderBy, 0),
		limit:     1000,
		offset:    0,
	}
	qs.compiler = Compiler(model)
	return qs
}

func (qs *QuerySet) DB() DB {
	return qs.compiler.DB()
}

func (qs *QuerySet) Model() attrs.Definer {
	return qs.model
}

func (qs *QuerySet) Compiler() QueryCompiler {
	return qs.compiler
}

func (qs *QuerySet) Clone() *QuerySet {
	return &QuerySet{
		model:     qs.model,
		queryInfo: qs.queryInfo,
		fields:    slices.Clone(qs.fields),
		union:     slices.Clone(qs.union),
		where:     slices.Clone(qs.where),
		having:    slices.Clone(qs.having),
		joins:     slices.Clone(qs.joins),
		groupBy:   slices.Clone(qs.groupBy),
		orderBy:   slices.Clone(qs.orderBy),
		limit:     qs.limit,
		offset:    qs.offset,
		forUpdate: qs.forUpdate,
		distinct:  qs.distinct,
		compiler:  qs.compiler,
	}
}

func (w *QuerySet) String() string {
	var sb = strings.Builder{}
	sb.WriteString("QuerySet{")
	sb.WriteString("model: ")
	sb.WriteString(fmt.Sprintf("%T", w.model))
	sb.WriteString(", fields: [")
	var written bool
	for _, field := range w.fields {
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

func (qs *QuerySet) GoString() string {
	var sb = strings.Builder{}
	sb.WriteString("QuerySet{")
	sb.WriteString("\n\tmodel: ")
	sb.WriteString(fmt.Sprintf("%T", qs.model))
	sb.WriteString(",\n\tfields: [")
	var written bool
	for _, field := range qs.fields {
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

	if len(qs.where) > 0 {
		sb.WriteString("\n\twhere: [")
		for _, expr := range qs.where {
			fmt.Fprintf(&sb, "\n\t\t%T: %#v", expr, expr)
		}
		sb.WriteString("\n\t],")
	}

	if len(qs.joins) > 0 {
		sb.WriteString("\n\tjoins: [")
		for _, join := range qs.joins {
			sb.WriteString("\n\t\t")
			sb.WriteString(join.TypeJoin)
			sb.WriteString(" ")
			sb.WriteString(join.Table)
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

func (qs *QuerySet) unpackFields(fields ...string) (infos []FieldInfo, hasRelated bool) {
	infos = make([]FieldInfo, 0, len(qs.fields))
	var info = FieldInfo{
		Table:  qs.queryInfo.tableName,
		Fields: make([]attrs.Field, 0),
	}

	if len(fields) == 0 || len(fields) == 1 && fields[0] == "*" {
		fields = make([]string, 0, len(qs.queryInfo.fields))
		for _, field := range qs.queryInfo.fields {
			fields = append(fields, field.Name())
		}
	}

	for _, field := range fields {

		var onlyPrimary = false
		if strings.HasSuffix(strings.ToLower(field), "__pk") {
			field = field[:len(field)-4]
			onlyPrimary = true
		}

		var current, _, field, chain, isRelated, err = walkFields(qs.model, field)
		if err != nil {
			panic(err)
		}

		if isRelated && ((!onlyPrimary && len(chain) == 1) || len(chain) > 1) {
			hasRelated = true

			var relDefs = current.FieldDefs()
			infos = append(infos, FieldInfo{
				SourceField: field,
				Model:       current,
				Table:       relDefs.TableName(),
				Fields:      relDefs.Fields(),
				Chain:       chain,
			})

			continue
		}

		info.Fields = append(info.Fields, field)
	}

	infos = append(infos, info)
	return infos, hasRelated
}

func (qs *QuerySet) attrFields(obj attrs.Definer) []attrs.Field {
	var defs = obj.FieldDefs()
	var fields []attrs.Field
	if len(qs.fields) > 0 {
		fields = make([]attrs.Field, 0, len(qs.fields))
		for _, info := range qs.fields {
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

func (qs *QuerySet) Select(fields ...string) *QuerySet {
	qs = qs.Clone()

	var (
		fieldInfos = make([]FieldInfo, 0)
		joins      = make([]JoinDef, 0)
		joinM      = make(map[string]bool)
	)

	if len(fields) == 0 || len(fields) == 1 && fields[0] == "*" {
		fields = make([]string, 0, len(qs.queryInfo.fields))
		for _, field := range qs.queryInfo.fields {
			fields = append(fields, field.Name())
		}
	}

	for _, info := range fields {

		var allFields bool
		if strings.HasSuffix(strings.ToLower(info), ".*") {
			info = info[:len(info)-2]
			allFields = true
		}

		var current, parent, field, chain, isRelated, err = walkFields(
			qs.model, info,
		)
		if err != nil {
			panic(err)
		}

		// The field might be a relation
		var rel = field.Rel()

		// If all fields of the relation are requested, we need to add the relation
		// to the join list. We also need to add the parent field to the chain.
		if rel != nil && allFields {
			chain = append(chain, field.Name())
			parent = current
			current = rel
			isRelated = true
		}

		var defs = current.FieldDefs()
		var tableName = defs.TableName()
		if len(chain) > 0 && isRelated {
			var relField = defs.Primary()
			var parentDefs = parent.FieldDefs()
			var parentField, ok = parentDefs.Field(chain[len(chain)-1])
			if !ok {
				panic(fmt.Errorf("field %q not found in %T", chain[len(chain)-1], parent))
			}

			var front, back = qs.compiler.Quote()
			var condA = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				front, parentDefs.TableName(), back,
				front, parentField.ColumnName(), back,
			)
			var condB = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				front, tableName, back,
				front, relField.ColumnName(), back,
			)

			var includedFields []attrs.Field
			if allFields {
				includedFields = defs.Fields()
			} else {
				includedFields = []attrs.Field{field}
			}

			fieldInfos = append(fieldInfos, FieldInfo{
				SourceField: field,
				Table:       tableName,
				Model:       current,
				Fields:      includedFields,
				Chain:       chain,
			})

			var key = fmt.Sprintf("%s.%s", condA, condB)
			if _, ok := joinM[key]; ok {
				continue
			}

			joinM[key] = true
			joins = append(joins, JoinDef{
				TypeJoin:   "LEFT JOIN",
				Table:      defs.TableName(),
				ConditionA: condA,
				Logic:      "=",
				ConditionB: condB,
			})

			continue
		}

		fieldInfos = append(fieldInfos, FieldInfo{
			Model:  current,
			Table:  tableName,
			Fields: []attrs.Field{field},
			Chain:  chain,
		})

	}

	qs.joins = joins
	qs.fields = fieldInfos

	return qs
}

func (qs *QuerySet) Filter(key interface{}, vals ...interface{}) *QuerySet {
	var nqs = qs.Clone()
	nqs.where = append(qs.where, express(key, vals...)...)
	return nqs
}

func (qs *QuerySet) Having(key interface{}, vals ...interface{}) *QuerySet {
	var nqs = qs.Clone()
	nqs.having = append(qs.having, express(key, vals...)...)
	return nqs
}

func (qs *QuerySet) GroupBy(fields ...string) *QuerySet {
	var nqs = qs.Clone()
	nqs.groupBy, _ = qs.unpackFields(fields...)
	return nqs
}

func (qs *QuerySet) OrderBy(fields ...string) *QuerySet {
	var nqs = qs.Clone()
	nqs.orderBy = make([]OrderBy, 0, len(fields))

	for _, field := range fields {
		var ord = strings.TrimSpace(field)
		var desc = false
		if strings.HasPrefix(ord, "-") {
			desc = true
			ord = strings.TrimPrefix(ord, "-")
		}

		var obj, _, field, _, _, err = walkFields(
			qs.model, ord,
		)

		if err != nil {
			panic(err)
		}

		var defs = obj.FieldDefs()
		nqs.orderBy = append(nqs.orderBy, OrderBy{
			Table: defs.TableName(),
			Field: field.ColumnName(),
			Desc:  desc,
		})
	}

	return nqs
}

func (qs *QuerySet) Reverse() *QuerySet {
	var ordBy = make([]OrderBy, 0, len(qs.orderBy))
	for _, ord := range qs.orderBy {
		ordBy = append(ordBy, OrderBy{
			Table: ord.Table,
			Field: ord.Field,
			Desc:  !ord.Desc,
		})
	}
	var nqs = qs.Clone()
	nqs.orderBy = ordBy
	return nqs
}

func (qs *QuerySet) Union(f func(*QuerySet) *QuerySet) *QuerySet {
	var nqs = qs.Clone()
	nqs.union = append(nqs.union, f)
	return nqs
}

func (qs *QuerySet) Limit(n int) *QuerySet {
	var nqs = qs.Clone()
	nqs.limit = n
	return nqs
}

func (qs *QuerySet) Offset(n int) *QuerySet {
	var nqs = qs.Clone()
	nqs.offset = n
	return nqs
}

func (qs *QuerySet) ForUpdate() *QuerySet {
	var nqs = qs.Clone()
	nqs.forUpdate = true
	return nqs
}

func (qs *QuerySet) Distinct() *QuerySet {
	var nqs = qs.Clone()
	nqs.distinct = true
	return nqs
}

func (qs *QuerySet) All() Query[[]attrs.Definer] {
	if len(qs.fields) == 0 {
		qs = qs.Select("*")
	}

	var resultQuery = qs.compiler.BuildSelectQuery(
		context.Background(),
		qs,
		qs.fields,
		qs.where,
		qs.having,
		qs.joins,
		qs.groupBy,
		qs.orderBy,
		qs.limit,
		qs.offset,
		qs.union,
		qs.forUpdate,
		qs.distinct,
	)

	return &wrappedQuery[[][]interface{}, []attrs.Definer]{
		query: resultQuery,
		exec: func(q Query[[][]interface{}]) ([]attrs.Definer, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}

			var list = make([]attrs.Definer, len(results))
			for i, row := range results {
				var obj = newObjectFromIface(qs.model)
				var fields = getScannableFields(qs.fields, obj)

				for j, field := range fields {
					var f = field.(attrs.Field)
					var val = row[j]

					if err = f.Scan(val); err != nil {
						return nil, errors.Wrapf(
							err,
							"failed to scan field %q in %T",
							f.Name(), obj,
						)
					}
				}

				list[i] = obj
			}

			return list, nil
		},
	}
}

func (qs *QuerySet) ValuesList(fields ...string) ValuesListQuery {

	qs = qs.Select(fields...)

	var resultQuery = qs.compiler.BuildSelectQuery(
		context.Background(),
		qs,
		qs.fields,
		qs.where,
		qs.having,
		qs.joins,
		qs.groupBy,
		qs.orderBy,
		qs.limit,
		qs.offset,
		qs.union,
		qs.forUpdate,
		qs.distinct,
	)

	return &wrappedQuery[[][]interface{}, [][]any]{
		query: resultQuery,
		exec: func(q Query[[][]interface{}]) ([][]any, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}

			var list = make([][]any, len(results))
			for i, row := range results {
				var obj = newObjectFromIface(qs.model)
				var fields = getScannableFields(qs.fields, obj)
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

			return list, nil
		},
	}
}

func (qs *QuerySet) Get() Query[attrs.Definer] {
	qs = qs.Limit(MAX_GET_RESULTS)
	q := qs.All()

	return &wrappedQuery[[]attrs.Definer, attrs.Definer]{
		query: q,
		exec: func(q Query[[]attrs.Definer]) (attrs.Definer, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}
			var resCnt = len(results)
			if resCnt == 0 {
				return nil, ErrNoRows
			}
			if resCnt > 1 {
				var errResCnt string
				if MAX_GET_RESULTS == 0 || resCnt < MAX_GET_RESULTS {
					errResCnt = strconv.Itoa(resCnt)
				} else {
					errResCnt = strconv.Itoa(MAX_GET_RESULTS-1) + "+"
				}

				return nil, errors.Wrapf(
					ErrMultipleRows,
					"multiple rows returned for %T: %s rows",
					qs.model, errResCnt,
				)
			}
			return results[0], nil
		},
	}
}

func (qs *QuerySet) GetOrCreate(value attrs.Definer) (attrs.Definer, error) {

	if len(qs.where) == 0 {
		return nil, ErrNoWhereClause
	}

	// Create a new transaction
	var ctx = context.Background()
	var transaction, err = qs.compiler.StartTransaction(ctx)
	if err != nil {
		return nil, err
	}

	defer transaction.Rollback()

	// Check if the object already exists
	var resultQuery = qs.Get()
	obj, err := resultQuery.Exec()
	if err != nil {
		if errors.Is(err, ErrNoRows) {
			goto create
		} else {
			return nil, err
		}
	}

	// Object already exists, return it and commit the transaction
	if obj != nil {
		return obj, transaction.Commit()
	}

	// Object does not exist, create it
create:
	var createQuery = qs.Create(value)
	obj, err = createQuery.Exec()
	if err != nil {
		return nil, err
	}

	// Object was created successfully, commit the transaction
	return obj, transaction.Commit()
}

func (qs *QuerySet) First() Query[attrs.Definer] {
	qs = qs.Limit(1)
	q := qs.All()

	return &wrappedQuery[[]attrs.Definer, attrs.Definer]{
		query: q,
		exec: func(q Query[[]attrs.Definer]) (attrs.Definer, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, ErrNoRows
			}
			return results[0], nil
		},
	}
}

func (qs *QuerySet) Last() Query[attrs.Definer] {
	var nqs = qs.Reverse()
	nqs.limit = 1
	nqs.offset = 0
	q := nqs.All()

	return &wrappedQuery[[]attrs.Definer, attrs.Definer]{
		query: q,
		exec: func(q Query[[]attrs.Definer]) (attrs.Definer, error) {
			var results, err = q.Exec()
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, ErrNoRows
			}
			return results[0], nil
		},
	}
}

func (qs *QuerySet) Exists() ExistsQuery {
	qs = qs.Clone()

	var resultQuery = qs.compiler.BuildCountQuery(
		context.Background(),
		qs,
		qs.where,
		qs.joins,
		qs.groupBy,
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

func (qs *QuerySet) Count() CountQuery {

	qs = qs.Clone()

	return qs.compiler.BuildCountQuery(
		context.Background(),
		qs,
		qs.where,
		qs.joins,
		qs.groupBy,
		qs.limit,
		qs.offset,
	)
}

func (qs *QuerySet) Create(value attrs.Definer) Query[attrs.Definer] {

	// Check if the object is a saver
	// If it is, we can use the Save method to save the object
	if saver, ok := value.(models.Saver); ok {
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
		Table: defs.TableName(),
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
				ErrFieldNull,
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

			var newObj = newObjectFromIface(qs.model)
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
						ErrLastInsertId,
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

func (qs *QuerySet) Update(value attrs.Definer) CountQuery {
	qs = qs.Clone()

	if len(qs.where) == 0 {
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
		Model:  value,
		Table:  defs.TableName(),
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
				ErrFieldNull,
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
		qs.where,
		qs.joins,
		qs.groupBy,
		values,
	)

	return resultQuery
}

func (qs *QuerySet) Delete() CountQuery {
	qs = qs.Clone()

	var resultQuery = qs.compiler.BuildDeleteQuery(
		context.Background(),
		qs,
		qs.where,
		qs.joins,
		qs.groupBy,
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
					obj = newObjectFromIface(info.Model)
				} else {
					obj = newObjectFromIface(field.Rel())
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
