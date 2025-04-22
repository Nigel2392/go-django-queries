package queries

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/pkg/errors"
)

// -----------------------------------------------------------------------------
// QuerySet
// -----------------------------------------------------------------------------

type Union func(*QuerySet[attrs.Definer])

type fieldInfo struct {
	srcField attrs.Field
	model    attrs.Definer
	srcTable string
	dstTable string
	chain    []string
	fields   []attrs.Field
}

func (f *fieldInfo) writeFields(sb *strings.Builder, quote string) {
	for i, field := range f.fields {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(quote)
		sb.WriteString(f.dstTable)
		sb.WriteString(quote)
		sb.WriteString(".")
		sb.WriteString(quote)
		sb.WriteString(field.ColumnName())
		sb.WriteString(quote)
	}
}

type orderBy struct {
	table string
	field string
	desc  bool
}

type QuerySet[T attrs.Definer] struct {
	quote     string
	queryInfo *queryInfo[T]
	model     T
	fields    []fieldInfo
	where     []Expression
	having    []Expression
	joins     []joinDef
	groupBy   []fieldInfo
	orderBy   []orderBy
	limit     int
	offset    int
	union     []Union
	forUpdate bool
}

type joinDef struct {
	field      attrs.Field
	info       fieldInfo
	typeJoin   string
	conditionA string
	logic      string
	conditionB string
}

func Objects[T attrs.Definer](model T) *QuerySet[T] {
	var q, err = getQueryInfo(model)
	if err != nil {
		panic(err)
	}

	return &QuerySet[T]{
		queryInfo: q,
		quote:     quote,
		model:     model,
		where:     make([]Expression, 0),
		having:    make([]Expression, 0),
		joins:     make([]joinDef, 0),
		groupBy:   make([]fieldInfo, 0),
		orderBy:   make([]orderBy, 0),
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
	}
}

func (qs *QuerySet[T]) unpackFields(fields ...string) (infos []fieldInfo, hasRelated bool) {
	infos = make([]fieldInfo, 0, len(qs.fields))
	var info = fieldInfo{
		dstTable: qs.queryInfo.tableName,
		model:    qs.model,
		fields:   make([]attrs.Field, 0),
	}

	if len(fields) == 0 || len(fields) == 1 && fields[0] == "*" {
		fields = make([]string, 0, len(qs.queryInfo.fields))
		for _, field := range qs.queryInfo.fields {
			fields = append(fields, field.Name())
		}
	}

	for _, field := range fields {
		var current, parent, field, chain, isRelated, err = walkFields(qs.model, field)
		if err != nil {
			panic(err)
		}

		if isRelated {
			hasRelated = true

			var relDefs = current.FieldDefs()
			infos = append(infos, fieldInfo{
				srcField: field,
				model:    current,
				srcTable: parent.FieldDefs().TableName(),
				dstTable: relDefs.TableName(),
				fields:   relDefs.Fields(),
				chain:    chain,
			})

			continue
		}

		info.fields = append(info.fields, field)
	}

	infos = append(infos, info)
	return infos, hasRelated
}

func (qs *QuerySet[T]) Fields(fields ...string) *QuerySet[T] {
	var hasRelated bool
	qs.fields, hasRelated = qs.unpackFields(fields...)

	if !hasRelated {
		return qs
	}

	for _, info := range qs.fields {
		if info.dstTable != qs.queryInfo.tableName {
			var (
				relDefs    = info.model.FieldDefs()
				relTable   = relDefs.TableName()
				relPrimary = relDefs.Primary()
			)

			var (
				condA = fmt.Sprintf(
					"%s%s%s.%s%s%s",
					quote, info.srcTable, quote,
					quote, info.srcField.ColumnName(), quote,
				)
				condB = fmt.Sprintf(
					"%s%s%s.%s%s%s",
					quote, relTable, quote,
					quote, relPrimary.ColumnName(), quote,
				)
			)

			qs.joins = append(qs.joins, joinDef{
				info:       info,
				typeJoin:   "LEFT JOIN",
				conditionA: condA,
				logic:      "=",
				conditionB: condB,
			})

			continue
		}
	}

	return qs
}

func (qs *QuerySet[T]) Filter(key interface{}, vals ...interface{}) *QuerySet[T] {
	qs.where = append(qs.where, express(key, vals...)...)
	return qs
}

func (qs *QuerySet[T]) Having(key interface{}, vals ...interface{}) *QuerySet[T] {
	qs.having = append(qs.having, express(key, vals...)...)
	return qs
}

func (qs *QuerySet[T]) GroupBy(fields ...string) *QuerySet[T] {
	qs.groupBy, _ = qs.unpackFields(fields...)
	return qs
}

func (qs *QuerySet[T]) OrderBy(fields ...string) *QuerySet[T] {
	var ordering = make([]orderBy, 0, len(fields))

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
		ordering = append(ordering, orderBy{
			table: defs.TableName(),
			field: field.ColumnName(),
			desc:  desc,
		})
	}

	qs.orderBy = append(qs.orderBy, ordering...)
	return qs
}

func (s *QuerySet[T]) Union(f func(*QuerySet[attrs.Definer])) *QuerySet[T] {
	s.union = append(s.union, f)
	return s
}

func (qs *QuerySet[T]) Limit(n int) *QuerySet[T] {
	qs.limit = n
	return qs
}

func (qs *QuerySet[T]) Offset(n int) *QuerySet[T] {
	qs.offset = n
	return qs
}

func (qs *QuerySet[T]) ForUpdate() *QuerySet[T] {
	qs.forUpdate = true
	return qs
}

func (qs *QuerySet[T]) writeTableName(sb *strings.Builder) {
	sb.WriteString(qs.quote)
	sb.WriteString(qs.queryInfo.tableName)
	sb.WriteString(qs.quote)
}

func (qs *QuerySet[T]) writeJoins(sb *strings.Builder) {
	for _, join := range qs.joins {
		sb.WriteString(" ")
		sb.WriteString(join.typeJoin)
		sb.WriteString(" ")
		sb.WriteString(qs.quote)
		sb.WriteString(join.info.dstTable)
		sb.WriteString(qs.quote)
		sb.WriteString(" ON ")

		if join.field != nil {
			var f = join.field.Rel()
			var relDefs = f.FieldDefs()
			var relField = relDefs.Primary()

			sb.WriteString(qs.quote)
			sb.WriteString(join.info.srcTable)
			sb.WriteString(qs.quote)
			sb.WriteString(".")
			sb.WriteString(qs.quote)
			sb.WriteString(join.field.ColumnName())
			sb.WriteString(qs.quote)
			sb.WriteString(" = ")
			sb.WriteString(qs.quote)
			sb.WriteString(join.info.dstTable)
			sb.WriteString(qs.quote)
			sb.WriteString(".")
			sb.WriteString(qs.quote)
			sb.WriteString(relField.ColumnName())
			sb.WriteString(qs.quote)
		} else {
			sb.WriteString(join.conditionA)
			sb.WriteString(" ")
			sb.WriteString(join.logic)
			sb.WriteString(" ")
			sb.WriteString(join.conditionB)
		}
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
			sb.WriteString(field.table)
			sb.WriteString(qs.quote)
			sb.WriteString(".")
			sb.WriteString(qs.quote)
			sb.WriteString(field.field)
			sb.WriteString(qs.quote)

			if field.desc {
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

func (qs *QuerySet[T]) All() ([]T, error) {
	var (
		query = new(strings.Builder)
		args  []any
	)

	if len(qs.fields) == 0 {
		qs.Fields("*")
	}

	query.WriteString("SELECT ")
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

	sql := qs.queryInfo.dbx.Rebind(query.String())
	logger.Debugf("QuerySet (%T):\n\t%s\n\t%v", qs.model, sql, args)

	rows, err := qs.queryInfo.db.Query(sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		var scannables = make([]any, 0, len(qs.fields))
		var row = newObjectFromIface(qs.model).(T)
		var root attrs.Definer = row
		var instances = map[string]attrs.Definer{"": root}

		for _, info := range qs.fields {
			if info.srcField == nil {
				defs := root.FieldDefs()
				for _, f := range info.fields {
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
			for i, name := range info.chain {
				key := strings.Join(info.chain[:i+1], ".")
				parent := instances[parentKey]
				defs := parent.FieldDefs()

				field, ok := defs.Field(name)
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", name, parent))
				}

				if _, exists := instances[key]; !exists {
					var obj attrs.Definer
					if i == len(info.chain)-1 {
						obj = newObjectFromIface(info.model)
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
			for _, f := range info.fields {
				field, ok := finalDefs.Field(f.Name())
				if !ok {
					panic(fmt.Errorf("field %q not found in %T", f.Name(), final))
				}
				scannables = append(scannables, field)
			}
		}

		if err := rows.Scan(scannables...); err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}

		results = append(results, row)
	}

	return results, nil
}

func (qs *QuerySet[T]) First() (T, error) {
	qs.Limit(1)
	list, err := qs.All()
	if err != nil || len(list) == 0 {
		return *new(T), err
	}
	return list[0], nil
}

func (qs *QuerySet[T]) Count() (int, error) {
	q, err := getQueryInfo(qs.model)
	if err != nil {
		return 0, err
	}

	query := new(strings.Builder)
	query.WriteString("SELECT COUNT(*) FROM ")
	qs.writeTableName(query)
	qs.writeJoins(query)
	args := qs.writeWhereClause(query)

	sql := q.dbx.Rebind(query.String())
	logger.Debugf("Count QuerySet (%T): %s", qs.model, sql)

	var count int
	err = q.dbx.Get(&count, sql, args...)
	return count, err
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func buildWhereClause(b *strings.Builder, model attrs.Definer, quote string, exprs []Expression) []any {
	var args = make([]any, 0)
	for i, e := range exprs {
		e := e.Clone().With(model, quote)
		e.SQL(b)
		if i < len(exprs)-1 {
			b.WriteString(" AND ")
		}
		args = append(args, e.Args()...)
	}

	return args
}
