package queries

import (
	"context"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/Nigel2392/go-signals"
	"github.com/Nigel2392/goldcrest"
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

	goldcrest.Register(models.MODEL_SAVE_HOOK, 0, models.ModelFunc(func(c context.Context, m attrs.Definer) (changed bool, err error) {
		if u, ok := m.(ForUseInQueries); ok && !u.ForUseInQueries() {
			return false, nil
		}

		var (
			defs         = m.FieldDefs()
			primaryField = defs.Primary()
		)

		primaryValue, err := primaryField.Value()
		if err != nil {
			return false, err
		}

		if primaryValue == nil || fields.IsZero(primaryValue) {
			var _, err = GetQuerySet(m).ExplicitSave().Create(m)
			if err != nil {
				return false, err
			}
			return true, nil
		}

		ct, err := GetQuerySet(m).
			ExplicitSave().
			Filter(
				primaryField.Name(), primaryValue,
			).
			Update(m)
		return ct > 0, err
	}))

	goldcrest.Register(models.MODEL_DELETE_HOOK, 0, models.ModelFunc(func(c context.Context, m attrs.Definer) (changed bool, err error) {
		if u, ok := m.(ForUseInQueries); ok && !u.ForUseInQueries() {
			return false, nil
		}

		var (
			defs         = m.FieldDefs()
			primaryField = defs.Primary()
		)

		primaryValue, err := primaryField.Value()
		if err != nil {
			return false, err
		}

		if primaryValue == nil || fields.IsZero(primaryValue) {
			return false, nil
		}

		ct, err := GetQuerySet(m).Filter(
			primaryField.Name(),
			primaryValue,
		).Delete()
		return ct > 0, err
	}))
}

var _, _ = attrs.OnBeforeModelRegister.Listen(func(s signals.Signal[attrs.Definer], d attrs.Definer) error {

	var (
		def           = contenttypes.DefinitionForObject(d)
		registerCType = false
		changeCType   = false
	)

	if def == nil {
		def = &contenttypes.ContentTypeDefinition{
			ContentObject:     d,
			GetInstance:       CT_GetObject(d),
			GetInstances:      CT_ListObjects(d),
			GetInstancesByIDs: CT_ListObjectsByIDs(d),
		}
		registerCType = true
	} else {
		if def.GetInstance == nil {
			def.GetInstance = CT_GetObject(d)
			changeCType = true
		}
		if def.GetInstances == nil {
			def.GetInstances = CT_ListObjects(d)
			changeCType = true
		}
		if def.GetInstancesByIDs == nil {
			def.GetInstancesByIDs = CT_ListObjectsByIDs(d)
			changeCType = true
		}
	}

	switch {
	case changeCType:
		contenttypes.EditDefinition(def)
	case registerCType:
		contenttypes.Register(def)
	}

	return nil
})
