package queries

import (
	"github.com/go-sql-driver/mysql"
	pg_stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/mattn/go-sqlite3"
)

func init() {
	RegisterDriver(&mysql.MySQLDriver{}, "mysql", SupportsReturningLastInsertId)
	RegisterDriver(&sqlite3.SQLiteDriver{}, "sqlite3", SupportsReturningColumns)
	RegisterDriver(&pg_stdlib.Driver{}, "postgres", SupportsReturningColumns)
	RegisterDriver(&pg_stdlib.Driver{}, "pgx", SupportsReturningColumns)

	RegisterCompiler(&mysql.MySQLDriver{}, NewGenericQueryBuilder)
	RegisterCompiler(&sqlite3.SQLiteDriver{}, NewGenericQueryBuilder)
	RegisterCompiler(&pg_stdlib.Driver{}, NewGenericQueryBuilder)
}
