package queries

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/pkg/errors"
)

// -----------------------------------------------------------------------------
// QuerySet
// -----------------------------------------------------------------------------

type Union func(*QuerySet[attrs.Definer])

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

func (f *FieldInfo) writeFields(sb *strings.Builder, quote string) {
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

type QuerySet[T attrs.Definer] struct {
	quote     string
	queryInfo *queryInfo[T]
	model     T
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
}

func Objects[T attrs.Definer](model T) *QuerySet[T] {
	var q, err = getQueryInfo(model)
	if err != nil {
		panic(err)
	}

	return &QuerySet[T]{
		queryInfo: q,
		quote:     Quote,
		model:     model,
		where:     make([]Expression, 0),
		having:    make([]Expression, 0),
		joins:     make([]JoinDef, 0),
		groupBy:   make([]FieldInfo, 0),
		orderBy:   make([]OrderBy, 0),
		limit:     1000,
		offset:    0,
	}
}

func (qs *QuerySet[T]) Clone() *QuerySet[T] {
	return &QuerySet[T]{
		quote:     qs.quote,
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
	}
}

func (qs *QuerySet[T]) unpackFields(fields ...string) (infos []FieldInfo, hasRelated bool) {
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

func (qs *QuerySet[T]) Select(fields ...string) *QuerySet[T] {
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

			var condA = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				qs.quote, parentDefs.TableName(), qs.quote,
				qs.quote, parentField.ColumnName(), qs.quote,
			)
			var condB = fmt.Sprintf(
				"%s%s%s.%s%s%s",
				qs.quote, tableName, qs.quote,
				qs.quote, relField.ColumnName(), qs.quote,
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

func (qs *QuerySet[T]) Filter(key interface{}, vals ...interface{}) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.where = append(qs.where, express(key, vals...)...)
	return nqs
}

func (qs *QuerySet[T]) Having(key interface{}, vals ...interface{}) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.having = append(qs.having, express(key, vals...)...)
	return nqs
}

func (qs *QuerySet[T]) GroupBy(fields ...string) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.groupBy, _ = qs.unpackFields(fields...)
	return nqs
}

func (qs *QuerySet[T]) OrderBy(fields ...string) *QuerySet[T] {
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

func (qs *QuerySet[T]) Reverse() *QuerySet[T] {
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

func (qs *QuerySet[T]) Union(f func(*QuerySet[attrs.Definer])) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.union = append(nqs.union, f)
	return nqs
}

func (qs *QuerySet[T]) Limit(n int) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.limit = n
	return nqs
}

func (qs *QuerySet[T]) Offset(n int) *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.offset = n
	return nqs
}

func (qs *QuerySet[T]) ForUpdate() *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.forUpdate = true
	return nqs
}

func (qs *QuerySet[T]) Distinct() *QuerySet[T] {
	var nqs = qs.Clone()
	nqs.distinct = true
	return nqs
}

func (qs *QuerySet[T]) All() Query[[]T, T] {
	var (
		query = new(strings.Builder)
		args  []any
	)

	if len(qs.fields) == 0 {
		qs = qs.Select("*")
	}

	query.WriteString("SELECT ")

	if qs.distinct {
		query.WriteString("DISTINCT ")
	}

	for i, info := range qs.fields {
		if i > 0 {
			query.WriteString(", ")
		}
		info.writeFields(query, qs.quote)
	}

	query.WriteString(" FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args = append(args, qs.writeWhereClause(query)...)
	qs.writeGroupBy(query)
	args = append(args, qs.writeHaving(query)...)
	qs.writeOrderBy(query)
	args = append(args, qs.writeLimitOffset(query)...)

	if qs.forUpdate {
		query.WriteString(" FOR UPDATE")
	}

	return &queryObject[[]T, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) ([]T, error) {

			rows, err := qs.queryInfo.db.Query(sql, args...)
			if err != nil {
				return nil, errors.Wrap(err, "failed to execute query")
			}
			defer rows.Close()

			var results []T
			for rows.Next() {
				var row = newObjectFromIface(qs.model).(T)
				var fields = getScannableFields(qs.fields, row)
				if err := rows.Scan(fields...); err != nil {
					return nil, errors.Wrap(err, "failed to scan row")
				}

				results = append(results, row)
			}
			if err := rows.Err(); err != nil {
				return nil, errors.Wrap(err, "failed to iterate rows")
			}

			return results, nil
		},
	}
}

func (qs *QuerySet[T]) First() Query[T, T] {
	qs = qs.Limit(1)
	q := qs.All()
	return &queryObject[T, T]{
		sql:   q.SQL(),
		model: qs.model,
		args:  q.Args(),
		exec: func(sql string, args ...any) (T, error) {
			var list, err = q.Exec()
			if err != nil || len(list) == 0 {
				return *new(T), err
			}
			return list[0], nil
		},
	}
}

func (qs *QuerySet[T]) Last() Query[T, T] {
	var nqs = qs.Reverse()
	nqs.limit = 1
	nqs.offset = 0
	q := nqs.All()
	return &queryObject[T, T]{
		sql:   q.SQL(),
		model: qs.model,
		args:  q.Args(),
		exec: func(sql string, args ...any) (T, error) {
			var list, err = q.Exec()
			if err != nil || len(list) == 0 {
				return *new(T), err
			}
			return list[0], nil
		},
	}
}

func (qs *QuerySet[T]) Exists() ExistsQuery[T] {
	query := new(strings.Builder)
	query.WriteString("SELECT EXISTS(SELECT 1 FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args := qs.writeWhereClause(query)
	query.WriteString(" LIMIT 1)")

	return &queryObject[bool, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) (bool, error) {
			var exists bool
			var err = qs.queryInfo.dbx.Get(&exists, sql, args...)
			return exists, err
		},
	}
}

func (qs *QuerySet[T]) Count() CountQuery[T] {
	query := new(strings.Builder)
	query.WriteString("SELECT ")
	query.WriteString("COUNT(*) FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args := qs.writeWhereClause(query)

	return &queryObject[int64, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) (int64, error) {
			var count int64
			var err = qs.queryInfo.dbx.Get(&count, sql, args...)
			return count, err
		},
	}
}

func (qs *QuerySet[T]) ValuesList(fields ...string) ValuesListQuery[T] {

	qs = qs.Select(fields...)

	var query = new(strings.Builder)

	query.WriteString("SELECT ")

	if qs.distinct {
		query.WriteString("DISTINCT ")
	}

	for i, info := range qs.fields {
		if i > 0 {
			query.WriteString(", ")
		}

		info.writeFields(query, qs.quote)
	}

	query.WriteString(" FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args := qs.writeWhereClause(query)

	return &queryObject[[][]any, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) ([][]any, error) {
			var rows, err = qs.queryInfo.db.Query(sql, args...)
			if err != nil {
				return nil, errors.Wrap(err, "failed to execute query")
			}
			defer rows.Close()

			var values = make([][]any, 0)
			for rows.Next() {
				var row = newObjectFromIface(qs.model).(T)
				var fields = getScannableFields(qs.fields, row)
				if err := rows.Scan(fields...); err != nil {
					return nil, errors.Wrap(err, "failed to scan row")
				}

				var value = make([]any, 0, len(fields))
				for _, field := range fields {
					var f = field.(attrs.Field)
					value = append(value, f.GetValue())
				}

				values = append(values, value)
			}

			return values, err
		},
	}
}

func (qs *QuerySet[T]) Update(value T) CountQuery[T] {
	qs = qs.Clone()

	var query = new(strings.Builder)
	query.WriteString("UPDATE ")
	qs.writeTableName(query)
	query.WriteString(" SET ")

	var args = make([]any, 0)
	var defs = value.FieldDefs()
	var fields []attrs.Field

	if len(qs.fields) > 0 {
		fields = make([]attrs.Field, 0, len(qs.fields))
		for _, info := range qs.fields {
			for _, field := range info.Fields {
				var f, ok = defs.Field(field.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", field.Name(), value))
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

	var written = false
	for _, field := range fields {
		var atts = field.Attrs()
		var v, ok = atts[attrs.AttrAutoIncrementKey]
		if ok && v.(bool) {
			continue
		}

		if field.IsPrimary() || !field.AllowEdit() {
			continue
		}

		if written {
			query.WriteString(", ")
		}

		query.WriteString(qs.quote)
		query.WriteString(field.ColumnName())
		query.WriteString(qs.quote)
		query.WriteString(" = ?")

		var value, err = field.Value()
		if err != nil {
			panic(fmt.Errorf("failed to get value for field %q: %w", field.Name(), err))
		}

		if value == nil && !field.AllowNull() {
			panic(fmt.Errorf("field %q cannot be nil", field.Name()))
		}

		args = append(args, value)
		written = true
	}

	args = append(args, qs.writeWhereClause(query)...)

	return &queryObject[int64, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) (int64, error) {
			result, err := qs.queryInfo.db.Exec(sql, args...)
			if err != nil {
				return 0, err
			}
			return result.RowsAffected()
		},
	}
}

func (qs *QuerySet[T]) Delete() CountQuery[T] {
	query := new(strings.Builder)
	query.WriteString("DELETE FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args := qs.writeWhereClause(query)

	return &queryObject[int64, T]{
		sql:   qs.queryInfo.dbx.Rebind(query.String()),
		model: qs.model,
		args:  args,
		exec: func(sql string, args ...any) (int64, error) {
			result, err := qs.queryInfo.db.Exec(sql, args...)
			if err != nil {
				return 0, err
			}
			return result.RowsAffected()
		},
	}
}

// -----------------------------------------------------------------------------
// Query Building
// -----------------------------------------------------------------------------

func (qs *QuerySet[T]) writeTableName(sb *strings.Builder) {
	sb.WriteString(qs.quote)
	sb.WriteString(qs.queryInfo.tableName)
	sb.WriteString(qs.quote)
}

func (qs *QuerySet[T]) writeJoins(sb *strings.Builder) {
	for _, join := range qs.joins {
		sb.WriteString(" ")
		sb.WriteString(join.TypeJoin)
		sb.WriteString(" ")
		sb.WriteString(qs.quote)
		sb.WriteString(join.Table)
		sb.WriteString(qs.quote)
		sb.WriteString(" ON ")
		sb.WriteString(join.ConditionA)
		sb.WriteString(" ")
		sb.WriteString(join.Logic)
		sb.WriteString(" ")
		sb.WriteString(join.ConditionB)
	}
}

func (qs *QuerySet[T]) writeWhereClause(sb *strings.Builder) []any {
	var args = make([]any, 0)
	if len(qs.where) > 0 {
		sb.WriteString(" WHERE ")
		args = append(
			args, buildWhereClause(sb, qs.model, qs.quote, qs.where)...,
		)
	}
	return args
}

func (qs *QuerySet[T]) writeGroupBy(sb *strings.Builder) {
	if len(qs.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, info := range qs.groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}

			info.writeFields(sb, qs.quote)
		}
	}
}

func (qs *QuerySet[T]) writeHaving(sb *strings.Builder) []any {
	var args = make([]any, 0)
	if len(qs.having) > 0 {
		sb.WriteString(" HAVING ")
		args = append(
			args, buildWhereClause(sb, qs.model, qs.quote, qs.having)...,
		)
	}
	return args
}

func (qs *QuerySet[T]) writeOrderBy(sb *strings.Builder) {
	if len(qs.orderBy) > 0 {
		sb.WriteString(" ORDER BY ")

		for i, field := range qs.orderBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(qs.quote)
			sb.WriteString(field.Table)
			sb.WriteString(qs.quote)
			sb.WriteString(".")
			sb.WriteString(qs.quote)
			sb.WriteString(field.Field)
			sb.WriteString(qs.quote)

			if field.Desc {
				sb.WriteString(" DESC")
			} else {
				sb.WriteString(" ASC")
			}
		}
	}
}

func (qs *QuerySet[T]) writeLimitOffset(sb *strings.Builder) []any {
	var args = make([]any, 0)
	if qs.limit > 0 {
		sb.WriteString(" LIMIT ?")
		args = append(args, qs.limit)
	}

	if qs.offset > 0 {
		sb.WriteString(" OFFSET ?")
		args = append(args, qs.offset)
	}
	return args
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

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func buildWhereClause(b *strings.Builder, model attrs.Definer, quote string, exprs []Expression) []any {
	var args = make([]any, 0)
	for i, e := range exprs {
		e := e.With(model, quote)
		e.SQL(b)
		if i < len(exprs)-1 {
			b.WriteString(" AND ")
		}
		args = append(args, e.Args()...)
	}

	return args
}
