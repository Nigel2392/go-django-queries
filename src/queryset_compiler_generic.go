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

func (g *genericQueryBuilder) DB() DB {
	if g.InTransaction() {
		return g.transaction
	}
	return g.queryInfo.DB
}

func (g *genericQueryBuilder) Quote() (string, string) {
	return g.quote, g.quote
}

func (g *genericQueryBuilder) StartTransaction(ctx context.Context) (Transaction, error) {
	if g.InTransaction() {
		return nil, query_errors.ErrTransactionStarted
	}

	var tx, err = g.queryInfo.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, query_errors.ErrFailedStartTransaction
	}

	g.transaction = &wrappedTransaction{tx, g}
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
	fields []FieldInfo,
	where []expr.LogicalExpression,
	having []expr.LogicalExpression,
	joins []JoinDef,
	groupBy []FieldInfo,
	orderBy []OrderBy,
	limit int,
	offset int,
	forUpdate bool,
	distinct bool,
) CompiledQuery[[][]interface{}] {
	var (
		query = new(strings.Builder)
		args  []any
		model = qs.Model()
	)

	query.WriteString("SELECT ")

	if distinct {
		query.WriteString("DISTINCT ")
	}

	for i, info := range fields {
		if i > 0 {
			query.WriteString(", ")
		}

		args = append(
			args, info.WriteFields(
				query, g.driver, model, g.quote)...)
	}

	query.WriteString(" FROM ")
	g.writeTableName(query)
	g.writeJoins(query, joins)
	args = append(args, g.writeWhereClause(query, model, where)...)
	g.writeGroupBy(query, groupBy)
	args = append(args, g.writeHaving(query, model, having)...)
	g.writeOrderBy(query, orderBy)
	args = append(args, g.writeLimitOffset(query, limit, offset)...)

	if forUpdate {
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
			for _, info := range fields {
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
	where []expr.LogicalExpression,
	joins []JoinDef,
	groupBy []FieldInfo,
	limit int,
	offset int,
) CompiledQuery[int64] {

	var model = qs.Model()
	var query = new(strings.Builder)
	query.WriteString("SELECT COUNT(*) FROM ")
	g.writeTableName(query)
	g.writeJoins(query, joins)
	args := g.writeWhereClause(query, model, where)
	g.writeGroupBy(query, groupBy)
	args = append(args, g.writeLimitOffset(query, limit, offset)...)

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
	fields FieldInfo,
	primary attrs.Field,
	values []any,
) CompiledQuery[[]interface{}] {
	var model = qs.Model()
	var query = new(strings.Builder)
	query.WriteString("INSERT INTO ")
	query.WriteString(g.quote)
	query.WriteString(g.queryInfo.TableName)
	query.WriteString(g.quote)
	query.WriteString(" (")

	for i, field := range fields.Fields {
		if i > 0 {
			query.WriteString(", ")
		}

		query.WriteString(g.quote)
		query.WriteString(field.ColumnName())
		query.WriteString(g.quote)
	}

	query.WriteString(") VALUES (")

	for i := range fields.Fields {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteString("?")
	}

	query.WriteString(")")

	var support = internal.DBSupportsReturning(g.queryInfo.DB)

	switch {
	case support == SupportsReturningLastInsertId:
		// Handled in QueryObject.Exec(), do nothing

	case support == SupportsReturningColumns:
		query.WriteString(" RETURNING ")

		var written = false
		if primary != nil {
			query.WriteString(g.quote)
			query.WriteString(primary.ColumnName())
			query.WriteString(g.quote)
			written = true
		}

		for _, field := range fields.Fields {
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

	return &QueryObject[[]interface{}]{
		sql:   g.queryInfo.DBX.Rebind(query.String()),
		model: model,
		args:  values,
		exec: func(query string, args ...any) ([]interface{}, error) {
			var err error
			switch support {
			case SupportsReturningLastInsertId:
				var id sql.Result
				id, err = g.DB().ExecContext(ctx, query, args...)
				if err != nil {
					return nil, errors.Wrap(err, "failed to execute query")
				}

				var lastId, err = id.LastInsertId()
				if err != nil {
					return nil, errors.Wrap(err, "failed to get last insert id")
				}

				return []interface{}{lastId}, nil

			case SupportsReturningColumns:
				var resLen = len(fields.Fields)
				if primary != nil {
					resLen++
				}

				var result = make([]interface{}, resLen)
				for i := range result {
					result[i] = new(interface{})
				}

				var row *sql.Row
				row = g.DB().QueryRowContext(ctx, query, args...)

				if err := row.Scan(result...); err != nil {
					return nil, errors.Wrap(err, "failed to scan row")
				}

				if err := row.Err(); err != nil {
					return nil, errors.Wrap(err, "failed to iterate rows")
				}

				for i, iface := range result {
					var field = iface.(*interface{})
					result[i] = *field
				}

				return result, nil

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
	fields FieldInfo,
	where []expr.LogicalExpression,
	joins []JoinDef,
	groupBy []FieldInfo,
	values []any,
) CompiledQuery[int64] {
	var model = qs.Model()
	var query = new(strings.Builder)
	query.WriteString("UPDATE ")
	query.WriteString(g.quote)
	query.WriteString(g.queryInfo.TableName)
	query.WriteString(g.quote)
	query.WriteString(" SET ")

	var args = make([]any, 0)
	var written bool
	var valuesIdx int
	for _, field := range fields.Fields {
		if written {
			query.WriteString(", ")
		}

		var a, isSQL, ok = fields.WriteField(
			query, g.driver, model, g.quote, field, true,
		)
		written = ok || written
		if !ok {
			continue
		}

		if isSQL {
			args = append(args, a...)
		} else {
			args = append(args, values[valuesIdx])
			valuesIdx++
		}
	}

	args = append(
		args,
		g.writeWhereClause(query, model, where)...,
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

func (g *genericQueryBuilder) BuildDeleteQuery(
	ctx context.Context,
	qs *GenericQuerySet,
	where []expr.LogicalExpression,
	joins []JoinDef,
	groupBy []FieldInfo,
) CompiledQuery[int64] {
	var model = qs.Model()
	var query = new(strings.Builder)
	query.WriteString("DELETE FROM ")
	query.WriteString(g.quote)
	query.WriteString(g.queryInfo.TableName)
	query.WriteString(g.quote)
	g.writeJoins(query, joins)

	var args = make([]any, 0)
	args = append(
		args,
		g.writeWhereClause(query, model, where)...,
	)

	g.writeGroupBy(query, groupBy)

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

func (g *genericQueryBuilder) writeTableName(sb *strings.Builder) {
	sb.WriteString(g.quote)
	sb.WriteString(g.queryInfo.TableName)
	sb.WriteString(g.quote)
}

func (g *genericQueryBuilder) writeJoins(sb *strings.Builder, joins []JoinDef) {
	for _, join := range joins {
		sb.WriteString(" ")
		sb.WriteString(join.TypeJoin)
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
		sb.WriteString(join.ConditionA)
		sb.WriteString(" ")
		sb.WriteString(join.Logic)
		sb.WriteString(" ")
		sb.WriteString(join.ConditionB)
	}
}

func (g *genericQueryBuilder) writeWhereClause(sb *strings.Builder, model attrs.Definer, where []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	if len(where) > 0 {
		sb.WriteString(" WHERE ")
		args = append(
			args, buildWhereClause(sb, g.driver, model, g.quote, where)...,
		)
	}
	return args
}

func (g *genericQueryBuilder) writeGroupBy(sb *strings.Builder, groupBy []FieldInfo) {
	if len(groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, info := range groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}

			info.WriteFields(sb, g.driver, nil, g.quote)
		}
	}
}

func (g *genericQueryBuilder) writeHaving(sb *strings.Builder, model attrs.Definer, having []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	if len(having) > 0 {
		sb.WriteString(" HAVING ")
		args = append(
			args, buildWhereClause(sb, g.driver, model, g.quote, having)...,
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

			if field.Alias != "" {
				sb.WriteString(g.quote)
				sb.WriteString(field.Alias)
				sb.WriteString(g.quote)
			} else {
				sb.WriteString(g.quote)
				if field.Table.Alias == "" {
					sb.WriteString(field.Table.Name)
				} else {
					sb.WriteString(field.Table.Alias)
				}
				sb.WriteString(g.quote)
				sb.WriteString(".")
				sb.WriteString(g.quote)
				sb.WriteString(field.Field)
				sb.WriteString(g.quote)
			}

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

func buildWhereClause(b *strings.Builder, d driver.Driver, model attrs.Definer, quote string, exprs []expr.LogicalExpression) []any {
	var args = make([]any, 0)
	for i, e := range exprs {
		e := e.Resolve(d, model, quote)
		var a = e.SQL(b)
		if i < len(exprs)-1 {
			b.WriteString(" AND ")
		}
		args = append(args, a...)
	}

	return args
}
