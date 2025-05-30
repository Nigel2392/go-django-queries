package queries

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/pkg/errors"
)

type genericQueryBuilder struct {
	transaction Transaction
	queryInfo   *internal.QueryInfo
	support     SupportsReturning
	quote       string
	driver      driver.Driver
}

func NewGenericQueryBuilder(model attrs.Definer, db string) QueryCompiler {
	var q, err = internal.GetQueryInfo(model, db)
	if err != nil {
		panic(err)
	}

	var quote = "`"
	switch internal.SqlxDriverName(q.DB) {
	case "mysql":
		quote = "`"
	case "postgres", "pgx":
		quote = "\""
	case "sqlite3":
		quote = "`"
	}

	return &genericQueryBuilder{
		quote:     quote,
		support:   internal.DBSupportsReturning(q.DB),
		driver:    q.DB.Driver(),
		queryInfo: q,
	}
}

func (g *genericQueryBuilder) DatabaseName() string {
	return g.queryInfo.DatabaseName
}

func (g *genericQueryBuilder) DB() DB {
	if g.InTransaction() {
		return g.transaction
	}
	return g.queryInfo.DB
}

func (g *genericQueryBuilder) Quote() (string, string) {
	return g.quote, g.quote
}

func (g *genericQueryBuilder) FormatColumn(col *expr.TableColumn) (string, []any) {
	var (
		sb   = new(strings.Builder)
		args = make([]any, 0, 1)
	)

	var err = col.Validate()
	if err != nil {
		panic(fmt.Errorf("cannot format column: %w", err))
	}

	if col.TableOrAlias != "" {
		sb.WriteString(g.quote)
		sb.WriteString(col.TableOrAlias)
		sb.WriteString(g.quote)
		sb.WriteString(".")
	}

	var aliasWritten bool
	switch {
	case col.FieldColumn != nil:
		sb.WriteString(g.quote)
		sb.WriteString(col.FieldColumn.ColumnName())
		sb.WriteString(g.quote)

	case col.RawSQL != "":
		sb.WriteString(col.RawSQL)
		if col.Value != nil {
			args = append(args, col.Value)
		}

	case col.Value != nil:
		sb.WriteString("?")
		args = append(args, col.Value)

	case col.FieldAlias != "":
		aliasWritten = true
		sb.WriteString(g.quote)
		sb.WriteString(col.FieldAlias)
		sb.WriteString(g.quote)

	default:
		panic(fmt.Errorf("cannot format column, no field, value or raw SQL provided: %v", col))
	}

	if col.FieldAlias != "" && !aliasWritten {
		sb.WriteString(" AS ")
		sb.WriteString(g.quote)
		sb.WriteString(col.FieldAlias)
		sb.WriteString(g.quote)
	}

	// Values are not used in the column definition.
	// We don't append them here.
	if col.ForUpdate {
		sb.WriteString(" = ?")
	}

	return sb.String(), args
}

func (g *genericQueryBuilder) StartTransaction(ctx context.Context) (Transaction, error) {
	if g.InTransaction() {
		return nil, query_errors.ErrTransactionStarted
	}

	var tx, err = g.queryInfo.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, query_errors.ErrFailedStartTransaction
	}

	return g.WithTransaction(tx)
}

func (g *genericQueryBuilder) WithTransaction(t Transaction) (Transaction, error) {
	if g.InTransaction() {
		return nil, query_errors.ErrTransactionStarted
	}

	if t == nil {
		return nil, query_errors.ErrTransactionNil
	}

	g.transaction = &wrappedTransaction{t, g}
	return g.transaction, nil
}

func (g *genericQueryBuilder) CommitTransaction() error {
	if !g.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	return g.transaction.Commit()
}

func (g *genericQueryBuilder) RollbackTransaction() error {
	if !g.InTransaction() {
		return query_errors.ErrNoTransaction
	}
	return g.transaction.Rollback()
}

func (g *genericQueryBuilder) InTransaction() bool {
	return g.transaction != nil
}

func (g *genericQueryBuilder) SupportsReturning() SupportsReturning {
	return g.support
}

func (g *genericQueryBuilder) BuildSelectQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	internals *QuerySetInternals,
) CompiledQuery[[][]interface{}] {
	var (
		query = new(strings.Builder)
		args  []any
		model = qs.Model()
		inf   = &expr.ExpressionInfo{
			Driver:      g.driver,
			Model:       qs.Model(),
			AliasGen:    qs.AliasGen,
			FormatField: g.FormatColumn,
			ForUpdate:   false,
		}
	)

	query.WriteString("SELECT ")

	if internals.Distinct {
		query.WriteString("DISTINCT ")
	}

	for i, info := range internals.Fields {
		if i > 0 {
			query.WriteString(", ")
		}

		args = append(
			args, info.WriteFields(
				query, inf)...)
	}

	query.WriteString(" FROM ")
	g.writeTableName(query, internals)
	args = append(args, g.writeJoins(query, internals.Joins)...)
	args = append(args, g.writeWhereClause(query, inf, internals.Where)...)
	args = append(args, g.writeGroupBy(query, inf, internals.GroupBy)...)
	args = append(args, g.writeHaving(query, inf, internals.Having)...)
	g.writeOrderBy(query, internals.OrderBy)
	args = append(args, g.writeLimitOffset(query, internals.Limit, internals.Offset)...)

	if internals.ForUpdate {
		query.WriteString(" FOR UPDATE")
	}

	return &QueryObject[[][]interface{}]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  args,
		exec: func(sql string, args ...any) ([][]interface{}, error) {

			rows, err := g.DB().QueryContext(ctx, sql, args...)
			if err != nil {
				return nil, errors.Wrap(err, "failed to execute query")
			}

			defer rows.Close()

			if err := rows.Err(); err != nil {
				return nil, errors.Wrap(err, "failed to iterate rows")
			}

			var results = make([][]interface{}, 0, 8)
			var amountCols = 0
			for _, info := range internals.Fields {
				if info.Through != nil {
					amountCols += len(info.Through.Fields)
				}
				amountCols += len(info.Fields)
			}

			for rows.Next() {
				var row = make([]interface{}, amountCols)
				for i := range row {
					row[i] = new(interface{})
				}
				err = rows.Scan(row...)
				if err != nil {
					return nil, errors.Wrap(err, "failed to scan row")
				}

				var result = make([]interface{}, amountCols)
				for i, iface := range row {
					var field = iface.(*interface{})
					result[i] = *field
				}

				results = append(results, result)
			}

			return results, nil
		},
	}
}

func (g *genericQueryBuilder) BuildCountQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	internals *QuerySetInternals,
) CompiledQuery[int64] {
	var inf = &expr.ExpressionInfo{
		Driver:      g.driver,
		Model:       qs.Model(),
		AliasGen:    qs.AliasGen,
		FormatField: g.FormatColumn,
		ForUpdate:   false,
	}
	var model = qs.Model()
	var query = new(strings.Builder)
	var args = make([]any, 0)
	query.WriteString("SELECT COUNT(*) FROM ")
	g.writeTableName(query, internals)

	args = append(args, g.writeJoins(query, internals.Joins)...)
	args = append(args, g.writeWhereClause(query, inf, internals.Where)...)
	args = append(args, g.writeGroupBy(query, inf, internals.GroupBy)...)
	args = append(args, g.writeLimitOffset(query, internals.Limit, internals.Offset)...)

	return &QueryObject[int64]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  args,
		exec: func(query string, args ...any) (int64, error) {
			var count int64
			var row = g.DB().QueryRowContext(ctx, query, args...)
			if err := row.Scan(&count); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return 0, nil
				}
				return 0, errors.Wrap(err, "failed to scan row")
			}
			return count, nil
		},
	}
}

func (g *genericQueryBuilder) BuildCreateQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	internals *QuerySetInternals,
	objects []*FieldInfo[attrs.Field],
	values []any, // flattened list of values
	// e.g. for 2 rows of 3 fields: [[1, 2, 4], [2, 3, 5]] -> [1, 2, 4, 2, 3, 5]
) CompiledQuery[[][]interface{}] {
	var (
		model   = qs.Model()
		query   = new(strings.Builder)
		support = internal.DBSupportsReturning(
			g.queryInfo.DB,
		)
	)

	if len(objects) == 0 {
		return &QueryObject[[][]interface{}]{
			sql:   "",
			model: model,
			args:  nil,
			exec: func(query string, args ...any) ([][]interface{}, error) {
				return nil, nil
			},
		}
	}

	var object = objects[0]

	query.WriteString("INSERT INTO ")
	query.WriteString(g.quote)
	query.WriteString(internals.Model.TableName)
	query.WriteString(g.quote)
	query.WriteString(" (")

	for i, field := range object.Fields {
		if i > 0 {
			query.WriteString(", ")
		}

		query.WriteString(g.quote)
		query.WriteString(field.ColumnName())
		query.WriteString(g.quote)
	}

	query.WriteString(") VALUES ")

	var written bool
	for _, obj := range objects {
		if written {
			query.WriteString(", ")
		}

		query.WriteString("(")
		for i := range obj.Fields {
			if i > 0 {
				query.WriteString(", ")
			}

			query.WriteString("?")
		}
		query.WriteString(")")
		written = true
	}

	switch {
	case support == SupportsReturningLastInsertId:

		if internals.Model.Primary != nil {
			query.WriteString(" RETURNING ")
			query.WriteString(g.quote)
			query.WriteString(
				internals.Model.Primary.ColumnName(),
			)
			query.WriteString(g.quote)
		}

	case support == SupportsReturningColumns:
		query.WriteString(" RETURNING ")

		var written = false
		if internals.Model.Primary != nil {
			query.WriteString(g.quote)
			query.WriteString(
				internals.Model.Primary.ColumnName(),
			)
			query.WriteString(g.quote)
			written = true
		}

		for _, field := range object.Fields {
			if written {
				query.WriteString(", ")
			}
			query.WriteString(g.quote)
			query.WriteString(field.ColumnName())
			query.WriteString(g.quote)
			written = true
		}
	case support == SupportsReturningNone:
		// do nothing

	default:
		panic(fmt.Errorf("returning not supported: %s", support))
	}

	var fieldLen = 0
	if len(objects) > 0 {
		fieldLen = len(objects[0].Fields)
	}
	if internals.Model.Primary != nil {
		fieldLen++
	}

	return &QueryObject[[][]interface{}]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  values,
		exec: func(query string, args ...any) ([][]interface{}, error) {
			var err error
			switch support {
			case SupportsReturningLastInsertId:

				if internals.Model.Primary == nil {
					return nil, nil
				}

				var rows, err = g.DB().QueryContext(ctx, query, args...)
				if err != nil {
					return nil, errors.Wrap(err, "failed to execute query")
				}
				defer rows.Close()

				var result = make([][]interface{}, 0, len(objects))
				for rows.Next() {

					var id = new(interface{})
					err = rows.Scan(id)
					if err != nil {
						return nil, errors.Wrap(err, "failed to scan row")
					}

					if err := rows.Err(); err != nil {
						return nil, errors.Wrap(err, "failed to iterate rows")
					}

					result = append(result, []interface{}{*id})
				}
				return result, nil

			case SupportsReturningColumns:

				var results = make([][]interface{}, 0, len(objects))
				var rows, err = g.DB().QueryContext(ctx, query, args...)
				if err != nil {
					return nil, errors.Wrap(err, "failed to execute query")
				}
				defer rows.Close()

				for rows.Next() {
					var result = make([]interface{}, fieldLen)
					for i := range result {
						result[i] = new(interface{})
					}
					err = rows.Scan(result...)
					if err != nil {
						return nil, errors.Wrap(err, "failed to scan row")
					}

					if err := rows.Err(); err != nil {
						return nil, errors.Wrap(err, "failed to iterate rows")
					}

					for i, iface := range result {
						var field = iface.(*interface{})
						result[i] = *field
					}

					results = append(results, result)
				}

				return results, nil

			case SupportsReturningNone:
				_, err = g.DB().ExecContext(ctx, query, args...)
				if err != nil {
					return nil, errors.Wrap(err, "failed to execute query")
				}
				return nil, nil

			default:
				panic(fmt.Errorf("returning not supported: %s", support))
			}
		},
	}
}

func (g *genericQueryBuilder) BuildUpdateQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	internals *QuerySetInternals,
	objects []UpdateInfo, // multiple objects can be updated at once
) CompiledQuery[int64] {
	var inf = &expr.ExpressionInfo{
		Driver:      g.driver,
		Model:       qs.Model(),
		AliasGen:    qs.AliasGen,
		FormatField: g.FormatColumn,
		ForUpdate:   true,
	}

	var (
		written bool
		args    = make([]any, 0)
		model   = qs.Model()
		query   = new(strings.Builder)
	)

	for _, info := range objects {
		// Set ForUpdate to true to ensure
		// correct column formatting when writing fields.
		inf.ForUpdate = true

		if written {
			query.WriteString("; ")
		}

		written = true
		query.WriteString("UPDATE ")
		query.WriteString(g.quote)
		query.WriteString(internals.Model.TableName)
		query.WriteString(g.quote)
		query.WriteString(" SET ")

		var fieldWritten bool
		var valuesIdx int
		for _, f := range info.Fields {
			if fieldWritten {
				query.WriteString(", ")
			}

			var a, isSQL, ok = info.WriteField(
				query, inf, f, true,
			)

			fieldWritten = ok || fieldWritten
			if !ok {
				continue
			}

			if isSQL {
				args = append(args, a...)
			} else {
				args = append(args, info.Values[valuesIdx])
				valuesIdx++
			}
		}

		// Set ForUpdate to false to avoid
		// incorrect column formatting in joins and where clauses.
		inf.ForUpdate = false

		args = append(
			args,
			g.writeJoins(query, info.Joins)...,
		)

		args = append(
			args,
			g.writeWhereClause(query, inf, info.Where)...,
		)
	}

	return &QueryObject[int64]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  args,
		exec: func(sql string, args ...any) (int64, error) {
			result, err := g.DB().ExecContext(ctx, sql, args...)
			if err != nil {
				return 0, err
			}
			return result.RowsAffected()
		},
	}
}

func (g *genericQueryBuilder) BuildDeleteQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	internals *QuerySetInternals,
) CompiledQuery[int64] {
	var inf = &expr.ExpressionInfo{
		Driver:      g.driver,
		Model:       qs.Model(),
		AliasGen:    qs.AliasGen,
		FormatField: g.FormatColumn,
		ForUpdate:   false,
	}
	var model = qs.Model()
	var query = new(strings.Builder)
	var args = make([]any, 0)
	query.WriteString("DELETE FROM ")
	query.WriteString(g.quote)
	query.WriteString(internals.Model.TableName)
	query.WriteString(g.quote)

	args = append(
		args,
		g.writeJoins(query, internals.Joins)...,
	)

	args = append(
		args,
		g.writeWhereClause(query, inf, internals.Where)...,
	)

	args = append(
		args,
		g.writeGroupBy(query, inf, internals.GroupBy)...,
	)

	return &QueryObject[int64]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  args,
		exec: func(sql string, args ...any) (int64, error) {
			result, err := g.DB().ExecContext(ctx, sql, args...)
			if err != nil {
				return 0, err
			}
			return result.RowsAffected()
		},
	}
}

func (g *genericQueryBuilder) writeTableName(sb *strings.Builder, internals *QuerySetInternals) {
	sb.WriteString(g.quote)
	sb.WriteString(internals.Model.TableName)
	sb.WriteString(g.quote)
}

func (g *genericQueryBuilder) writeJoins(sb *strings.Builder, joins []JoinDef) []any {
	var args = make([]any, 0)
	for _, join := range joins {
		sb.WriteString(" ")
		sb.WriteString(string(join.TypeJoin))
		sb.WriteString(" ")
		sb.WriteString(g.quote)
		sb.WriteString(join.Table.Name)
		sb.WriteString(g.quote)

		if join.Table.Alias != "" {
			sb.WriteString(" AS ")
			sb.WriteString(g.quote)
			sb.WriteString(join.Table.Alias)
			sb.WriteString(g.quote)
		}

		sb.WriteString(" ON ")
		var condition = join.JoinDefCondition
		for condition != nil {

			var col, argsCol = g.FormatColumn(&condition.ConditionA)
			sb.WriteString(col)
			args = append(args, argsCol...)

			sb.WriteString(" ")
			sb.WriteString(string(condition.Operator))
			sb.WriteString(" ")

			col, argsCol = g.FormatColumn(&condition.ConditionB)
			sb.WriteString(col)
			args = append(args, argsCol...)

			if condition.Next != nil {
				sb.WriteString(" AND ")
			}

			condition = condition.Next
		}
	}

	return args
}

func (g *genericQueryBuilder) writeWhereClause(sb *strings.Builder, inf *expr.ExpressionInfo, where []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	if len(where) > 0 {
		sb.WriteString(" WHERE ")
		args = append(
			args, buildWhereClause(sb, inf, where)...,
		)
	}
	return args
}

func (g *genericQueryBuilder) writeGroupBy(sb *strings.Builder, inf *expr.ExpressionInfo, groupBy []FieldInfo[attrs.FieldDefinition]) []any {
	var args = make([]any, 0)
	if len(groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, info := range groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}

			args = append(
				args, info.WriteFields(sb, inf)...,
			)
		}
	}
	return args
}

func (g *genericQueryBuilder) writeHaving(sb *strings.Builder, inf *expr.ExpressionInfo, having []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	if len(having) > 0 {
		sb.WriteString(" HAVING ")
		args = append(
			args, buildWhereClause(sb, inf, having)...,
		)
	}
	return args
}

func (g *genericQueryBuilder) writeOrderBy(sb *strings.Builder, orderBy []OrderBy) {
	if len(orderBy) > 0 {
		sb.WriteString(" ORDER BY ")

		for i, field := range orderBy {
			if i > 0 {
				sb.WriteString(", ")
			}

			if field.Column.TableOrAlias != "" && field.Column.FieldColumn != nil && field.Column.FieldAlias != "" {
				panic(fmt.Errorf(
					"cannot use table/alias, field column and field alias together in order by: %v",
					field.Column,
				))
			}

			var sql, _ = g.FormatColumn(&field.Column)
			sb.WriteString(sql)

			if field.Desc {
				sb.WriteString(" DESC")
			} else {
				sb.WriteString(" ASC")
			}
		}
	}
}

func (g *genericQueryBuilder) writeLimitOffset(sb *strings.Builder, limit int, offset int) []any {
	var args = make([]any, 0)
	if limit > 0 {
		sb.WriteString(" LIMIT ?")
		args = append(args, limit)
	}

	if offset > 0 {
		sb.WriteString(" OFFSET ?")
		args = append(args, offset)
	}
	return args
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func buildWhereClause(b *strings.Builder, inf *expr.ExpressionInfo, exprs []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	for i, e := range exprs {
		e := e.Resolve(inf)
		var a = e.SQL(b)
		if i < len(exprs)-1 {
			b.WriteString(" AND ")
		}
		args = append(args, a...)
	}
	return args
}
