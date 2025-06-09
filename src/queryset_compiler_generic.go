package queries

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/drivers"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/pkg/errors"
)

func init() {
	drivers.RegisterDriver(&drivers.DriverMySQL{}, "mysql", drivers.SupportsReturningLastInsertId)
	drivers.RegisterDriver(&drivers.DriverSQLite{}, "sqlite3", drivers.SupportsReturningColumns)
	drivers.RegisterDriver(&drivers.DriverPostgres{}, "postgres", drivers.SupportsReturningColumns)
	drivers.RegisterDriver(&drivers.DriverPostgres{}, "pgx", drivers.SupportsReturningColumns)

	RegisterCompiler(&drivers.DriverMySQL{}, NewGenericQueryBuilder)
	RegisterCompiler(&drivers.DriverSQLite{}, NewGenericQueryBuilder)
	RegisterCompiler(&drivers.DriverPostgres{}, NewGenericQueryBuilder)
}

func newExpressionInfo(g *genericQueryBuilder, qs *QuerySet[attrs.Definer], updating bool) *expr.ExpressionInfo {
	return &expr.ExpressionInfo{
		Driver: g.driver,
		Model: attrs.NewObject[attrs.Definer](
			qs.Model(),
		),
		Quote:       g.QuoteString,
		AliasGen:    qs.AliasGen,
		FormatField: g.FormatColumn,
		Placeholder: generic_PLACEHOLDER,
		Lookups: expr.ExpressionLookupInfo{
			PrepForLikeQuery: g.PrepForLikeQuery,
			FormatLookupCol:  g.FormatLookupCol,
			LogicalOpRHS:     g.LogicalOpRHS(),
			OperatorsRHS:     g.LookupOperatorsRHS(),
			PatternOpsRHS:    g.LookupPatternOperatorsRHS(),
		},
		ForUpdate: updating,
	}
}

const generic_PLACEHOLDER = "?"

type genericQueryBuilder struct {
	transaction Transaction
	queryInfo   *internal.QueryInfo
	support     drivers.SupportsReturningType
	quote       string
	driver      driver.Driver
}

func NewGenericQueryBuilder(db string) QueryCompiler {
	var q, err = internal.GetQueryInfo(db)
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
		support:   drivers.SupportsReturning(q.DB),
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

func (g *genericQueryBuilder) Placeholder() string {
	return generic_PLACEHOLDER
}

func (g *genericQueryBuilder) QuoteString(s string) string {
	var sb strings.Builder
	sb.Grow(len(s) + len(g.quote)*2)
	sb.WriteString(g.quote)
	sb.WriteString(s)
	sb.WriteString(g.quote)
	return sb.String()
}

func (g *genericQueryBuilder) PrepForLikeQuery(v any) string {
	// For LIKE queries, we need to escape the percent and underscore characters.
	// This is done by replacing them with their escaped versions.
	switch internal.SqlxDriverName(g.queryInfo.DB) {
	case "mysql":
		return strings.ReplaceAll(
			strings.ReplaceAll(fmt.Sprint(v), "%", "\\%"),
			"_", "\\_",
		)

	case "postgres", "pgx":
		return strings.ReplaceAll(
			strings.ReplaceAll(fmt.Sprint(v), "%", "\\%"),
			"_", "\\_",
		)

	case "sqlite3":
		return strings.ReplaceAll(
			strings.ReplaceAll(fmt.Sprint(v), "%", "\\%"),
			"_", "\\_",
		)

	default:
		panic(fmt.Errorf("unknown database driver: %s", internal.SqlxDriverName(g.queryInfo.DB)))
	}
}

func (g *genericQueryBuilder) FormatLookupCol(lookupName string, inner string) string {
	switch lookupName {
	case "iexact", "icontains", "istartswith", "iendswith":
		switch internal.SqlxDriverName(g.queryInfo.DB) {
		case "mysql":
			return fmt.Sprintf("BINARY %s", inner)
		case "postgres", "pgx":
			return fmt.Sprintf("LOWER(%s)", inner)
		case "sqlite3":
			return fmt.Sprintf("LOWER(%s)", inner)
		default:
			panic(fmt.Errorf("unknown database driver: %s", internal.SqlxDriverName(g.queryInfo.DB)))
		}
	default:
		return inner
	}
}

func equalityFormat(op expr.LogicalOp) func(string, []any) (string, []any) {
	return func(rhs string, value []any) (string, []any) {
		return fmt.Sprintf("%s %s", op, rhs), []any{value[0]}
	}
}

func mathOpFormat(op expr.LogicalOp) func(string, []any) (string, []any) {
	return func(rhs string, value []any) (string, []any) {
		return fmt.Sprintf("%s %s = %s", op, rhs, rhs), []any{value[0], value[0]}
	}
}

var defaultCompilerLogicalOperators = map[expr.LogicalOp]func(rhs string, value []any) (string, []any){
	expr.EQ:  equalityFormat(expr.EQ),  // = %s
	expr.NE:  equalityFormat(expr.NE),  // != %s
	expr.GT:  equalityFormat(expr.GT),  // > %s
	expr.LT:  equalityFormat(expr.LT),  // < %s
	expr.GTE: equalityFormat(expr.GTE), // >= %s
	expr.LTE: equalityFormat(expr.LTE), // <= %s
	//expr.ADD:    mathOpFormat(expr.ADD),    // + %s = %s
	//expr.SUB:    mathOpFormat(expr.SUB),    // - %s = %s
	//expr.MUL:    mathOpFormat(expr.MUL),    // * %s = %s
	//expr.DIV:    mathOpFormat(expr.DIV),    // / %s = %s
	//expr.MOD:    mathOpFormat(expr.MOD),    // % %s = %s
	//expr.BITAND: mathOpFormat(expr.BITAND), // & %s = %s
	//expr.BITOR:  mathOpFormat(expr.BITOR),  // | %s = %s
	//expr.BITXOR: mathOpFormat(expr.BITXOR), // ^ %s = %s
	//expr.BITLSH: mathOpFormat(expr.BITLSH), // << %s = %s
	//expr.BITRSH: mathOpFormat(expr.BITRSH), // >> %s = %s
	//expr.BITNOT: mathOpFormat(expr.BITNOT), // ~ %s = %s
}

func (g *genericQueryBuilder) LogicalOpRHS() map[expr.LogicalOp]func(rhs string, value []any) (string, []any) {
	return defaultCompilerLogicalOperators
}

func (g *genericQueryBuilder) LookupOperatorsRHS() map[string]string {
	switch internal.SqlxDriverName(g.queryInfo.DB) {
	case "mysql":
		return map[string]string{
			"iexact":      "LIKE %s",
			"contains":    "LIKE BINARY %s",
			"icontains":   "LIKE %s",
			"startswith":  "LIKE BINARY %s",
			"endswith":    "LIKE BINARY %s",
			"istartswith": "LIKE %s",
			"iendswith":   "LIKE %s",
		}
	case "postgres", "pgx":
		return map[string]string{
			"iexact":      "= LOWER(%s)",
			"contains":    "LIKE %s",
			"icontains":   "LIKE LOWER(%s)",
			"regex":       "~ %s",
			"startswith":  "LIKE %s",
			"endswith":    "LIKE %s",
			"istartswith": "LIKE LOWER(%s)",
			"iendswith":   "LIKE LOWER(%s)",
		}
	case "sqlite3":
		return map[string]string{
			"iexact":      "LIKE %s ESCAPE '\\'",
			"contains":    "LIKE %s ESCAPE '\\'",
			"icontains":   "LIKE %s ESCAPE '\\'",
			"regex":       "REGEXP %s",
			"iregex":      "REGEXP '(?i)' || %s",
			"startswith":  "LIKE %s ESCAPE '\\'",
			"endswith":    "LIKE %s ESCAPE '\\'",
			"istartswith": "LIKE %s ESCAPE '\\'",
			"iendswith":   "LIKE %s ESCAPE '\\'",
		}
	}
	panic(fmt.Errorf("unknown database driver: %s", internal.SqlxDriverName(g.queryInfo.DB)))
}

func (g *genericQueryBuilder) LookupPatternOperatorsRHS() map[string]string {
	switch internal.SqlxDriverName(g.queryInfo.DB) {
	case "mysql":
		return map[string]string{
			"contains":    "LIKE BINARY CONCAT('%%', %s, '%%')",
			"icontains":   "LIKE CONCAT('%%', %s, '%%')",
			"startswith":  "LIKE BINARY CONCAT(%s, '%%')",
			"istartswith": "LIKE CONCAT(%s, '%%')",
			"endswith":    "LIKE BINARY CONCAT('%%', %s)",
			"iendswith":   "LIKE CONCAT('%%', %s)",
		}
	case "postgres", "pgx":
		return map[string]string{
			"contains":    "LIKE '%%' || %s || '%%'",
			"icontains":   "LIKE '%%' || LOWER(%s) || '%%'",
			"startswith":  "LIKE %s || '%%'",
			"istartswith": "LIKE LOWER(%s) || '%%'",
			"endswith":    "LIKE '%%' || %s",
			"iendswith":   "LIKE '%%' || LOWER(%s)",
		}
	case "sqlite3":
		return map[string]string{
			"contains":    "LIKE '%%' || %s || '%%' ESCAPE '\\'",
			"icontains":   "LIKE '%%' || LOWER(%s) || '%%' ESCAPE '\\'",
			"startswith":  "LIKE %s || '%%' ESCAPE '\\'",
			"istartswith": "LIKE LOWER(%s) || '%%' ESCAPE '\\'",
			"endswith":    "LIKE '%%' || %s ESCAPE '\\'",
			"iendswith":   "LIKE '%%' || LOWER(%s) ESCAPE '\\'",
		}
	}
	panic(fmt.Errorf("unknown database driver: %s", internal.SqlxDriverName(g.queryInfo.DB)))
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
		var colName = col.FieldColumn.ColumnName()
		if colName == "" {
			panic(fmt.Errorf("cannot format column with empty column name: %+v (%s)", col, col.FieldColumn.Name()))
		}
		sb.WriteString(g.quote)
		sb.WriteString(colName)
		sb.WriteString(g.quote)

	case col.RawSQL != "":
		sb.WriteString(col.RawSQL)
		if col.Value != nil {
			args = append(args, col.Value)
		}

	case col.Value != nil:
		sb.WriteString(generic_PLACEHOLDER)
		args = append(args, col.Value)

	case col.FieldAlias != "":
		aliasWritten = true
		sb.WriteString(g.quote)
		sb.WriteString(col.FieldAlias)
		sb.WriteString(g.quote)

	default:
		panic(fmt.Errorf("cannot format column, no field, value or raw SQL provided: %+v", col))
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

func (g *genericQueryBuilder) Transaction() Transaction {
	if g.InTransaction() {
		return g.transaction
	}
	return nil
}

func (g *genericQueryBuilder) StartTransaction(ctx context.Context) (Transaction, error) {
	if g.InTransaction() {
		return g.transaction, nil
	}

	var tx, err = g.queryInfo.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, query_errors.ErrFailedStartTransaction
	}

	logger.Debugf("Starting transaction for %s", g.DatabaseName())

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

func (g *genericQueryBuilder) SupportsReturning() drivers.SupportsReturningType {
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
		inf   = newExpressionInfo(g, qs, false)
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
		model: inf.Model,
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
	var inf = newExpressionInfo(g, qs, false)
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
		model: inf.Model,
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
		model   = attrs.NewObject[attrs.Definer](qs.Model())
		query   = new(strings.Builder)
		support = drivers.SupportsReturning(
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

			query.WriteString(generic_PLACEHOLDER)
		}
		query.WriteString(")")
		written = true
	}

	switch {
	case support == drivers.SupportsReturningLastInsertId:

		if internals.Model.Primary != nil {
			query.WriteString(" RETURNING ")
			query.WriteString(g.quote)
			query.WriteString(
				internals.Model.Primary.ColumnName(),
			)
			query.WriteString(g.quote)
		}

	case support == drivers.SupportsReturningColumns:
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
	case support == drivers.SupportsReturningNone:
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
			case drivers.SupportsReturningLastInsertId:

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

			case drivers.SupportsReturningColumns:

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

			case drivers.SupportsReturningNone:
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
	var (
		inf = newExpressionInfo(
			g,
			qs,
			true,
		)
		written bool
		args    = make([]any, 0)
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
		model: inf.Model,
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
	var inf = newExpressionInfo(g, qs, false)
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
		model: inf.Model,
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

func (g *genericQueryBuilder) writeWhereClause(sb *strings.Builder, inf *expr.ExpressionInfo, where []expr.ClauseExpression) []any {
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

func (g *genericQueryBuilder) writeHaving(sb *strings.Builder, inf *expr.ExpressionInfo, having []expr.ClauseExpression) []any {
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

func buildWhereClause(b *strings.Builder, inf *expr.ExpressionInfo, exprs []expr.ClauseExpression) []any {
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
