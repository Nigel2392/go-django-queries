package queries

import (
	"database/sql"
	"database/sql/driver"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django/src/core/attrs"

	_ "unsafe"
)

type SupportsReturning = internal.SupportsReturning

const (
	SupportsReturningNone         SupportsReturning = internal.SupportsReturningNone
	SupportsReturningLastInsertId SupportsReturning = internal.SupportsReturningLastInsertId
	SupportsReturningColumns      SupportsReturning = internal.SupportsReturningColumns
)

type RelationType = internal.RelationType

const (
	RelationTypeForeignKeyReverse = internal.RelationTypeForeignKeyReverse
	RelationTypeOneToOne          = internal.RelationTypeOneToOne
	RelationTypeManyToMany        = internal.RelationTypeManyToMany
	RelationTypeForeignKey        = internal.RelationTypeForeignKey
)

const (
	ATTR_REVERSE_ALIAS = internal.ATTR_REVERSE_ALIAS
)

//go:linkname DBSupportsReturning github.com/Nigel2392/go-django-queries/internal.DBSupportsReturning
func DBSupportsReturning(db *sql.DB) SupportsReturning

// RegisterDriver registers a driver with the given database name.
//
// This is used to determine the database type when using sqlx.
//
// If your driver is not one of:
// - github.com/go-sql-driver/mysql.MySQLDriver
// - github.com/mattn/go-sqlite3.SQLiteDriver
// - github.com/jackc/pgx/v5/stdlib.Driver
//
// Then it explicitly needs to be registered here.
//
//go:linkname RegisterDriver github.com/Nigel2392/go-django-queries/internal.RegisterDriver
func RegisterDriver(driver driver.Driver, database string, supportsReturning ...SupportsReturning)

type (
	ModelMeta      = internal.ModelMeta
	Relation       = internal.Relation
	RelationChain  = internal.RelationChain
	RelationTarget = internal.RelationTarget
)

//go:linkname RegisterModel github.com/Nigel2392/go-django-queries/internal.RegisterModel
func RegisterModel(model attrs.Definer)

//go:linkname GetModelMeta github.com/Nigel2392/go-django-queries/internal.GetModelMeta
func GetModelMeta(model attrs.Definer) ModelMeta

//go:linkname GetRelationMeta github.com/Nigel2392/go-django-queries/internal.GetRelationMeta
func GetRelationMeta(m attrs.Definer, name string) (Relation, bool)
