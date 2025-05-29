package queries

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/Nigel2392/go-signals"
	"github.com/Nigel2392/goldcrest"
	"github.com/elliotchance/orderedmap/v2"
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

const __generate_where_filters_key = "queries.__GENERATE_WHERE_CLAUSE_FOR_OBJECTS"

func GenerateObjectsWhereClause[T attrs.Definer](objects ...T) ([]expr.LogicalExpression, error) {

	if len(objects) == 0 {
		return []expr.LogicalExpression{}, nil
	}

	var (
		modelMeta  = attrs.GetModelMeta(objects[0])
		primaryDef = modelMeta.Definitions().Primary()
	)

	if primaryDef == nil {
		// If the model has no primary key defined, we need to generate a where clause
		//
		// There has to be a function registered which can generate a proper where clause
		// for selections, this can be based on multiple fields of the object.
		var q, has = modelMeta.Storage(__generate_where_filters_key)
		if !has {
			return nil, fmt.Errorf("model %T has no primary key defined and no function registered to generate a where clauuse", objects[0])
		}

		var or = make([]expr.Expression, 0, len(objects))
		switch q := q.(type) {
		case func([]attrs.Definer) ([]expr.LogicalExpression, error):
			var definers = make([]attrs.Definer, len(objects))
			for i, object := range objects {
				definers[i] = object
			}

			var exprs, err = q(definers)
			if err != nil {
				return nil, fmt.Errorf("error generating where clause for objects %T: %w", objects[0], err)
			}

			for _, expr := range exprs {
				or = append(or, expr)
			}

		case func(attrs.Definer) ([]expr.LogicalExpression, error):
			for _, object := range objects {
				var exprs, err = q(object)
				if err != nil {
					return nil, fmt.Errorf("error generating where clause for object %T: %w", object, err)
				}
				for _, expr := range exprs {
					or = append(or, expr)
				}
			}

		case func(attrs.Definer) (expr.LogicalExpression, error):
			for _, object := range objects {
				var expr, err = q(object)
				if err != nil {
					return nil, fmt.Errorf("error generating where clause for object %T: %w", object, err)
				}
				or = append(or, expr)
			}

		default:
			return nil, fmt.Errorf("model %T has no primary key defined, cannot delete", objects[0])
		}

		return []expr.LogicalExpression{expr.Or(or...).(expr.LogicalExpression)}, nil
	} else {
		var primaryName = primaryDef.Name()

		if len(objects) == 1 {
			var obj = objects[0]
			var defs = obj.FieldDefs()
			var prim = defs.Primary()

			return expr.Express(primaryName, prim.GetValue()), nil
		}

		var ids = make([]any, 0, len(objects))
		for _, object := range objects {
			var def = object.FieldDefs()
			var primary = def.Primary()
			ids = append(ids, primary.GetValue())
		}

		return []expr.LogicalExpression{expr.Q(
			fmt.Sprintf("%s__in", primaryName), ids,
		)}, nil
	}
}

func getUniqueFields(modelMeta attrs.ModelMeta) [][]string {
	var (
		modelDefs    = modelMeta.Definitions()
		uniqueFields [][]string
	)
	var uniqueTogetherObj, ok = modelMeta.Storage(MetaUniqueTogetherKey)
	if ok {
		var fields = make([][]string, 0, 1)
		switch uqFields := uniqueTogetherObj.(type) {
		case []string:
			fields = append(fields, uqFields)
		case [][]string:
			fields = append(fields, uqFields...)
		default:
			panic(fmt.Sprintf("unexpected type for ModelMeta.Storage(%q): %T, expected []string or [][]string", MetaUniqueTogetherKey, uqFields))
		}
		uniqueFields = make([][]string, len(fields), len(fields))
		for i, fieldNames := range fields {
			uniqueFields[i] = make([]string, 0, len(fieldNames))
			for _, fieldName := range fieldNames {
				if fieldDef, ok := modelDefs.Field(fieldName); ok {
					uniqueFields[i] = append(uniqueFields[i], fieldDef.Name())
				} else {
					panic(fmt.Sprintf("field %q not found in model %T", fieldName, modelMeta.Model()))
				}
			}
		}
	}
	for _, field := range modelDefs.Fields() {
		var attributes = field.Attrs()
		var isUnique, _ = internal.GetFromAttrs[bool](attributes, attrs.AttrUniqueKey)
		if isUnique {
			uniqueFields = append(uniqueFields, []string{field.Name()})
		}
	}
	return uniqueFields
}

// Use the model meta to get the unique key for an object.
//
// If the model has a primary key defined, it will return the primary key value.
//
// If the model does not have a primary key defined, it will return the unique fields
// or unique together fields as a string of [fieldName]:[fieldValue]:[fieldName]:[fieldValue] pairs.
func GetUniqueKey(modelObject any) (any, error) {

	var (
		obj     attrs.Definer
		objDefs attrs.Definitions
	)
	switch o := modelObject.(type) {
	case attrs.Definer:
		obj = o
		objDefs = o.FieldDefs()
	case attrs.Definitions:
		obj = o.Instance()
		objDefs = o
	case attrs.Field:
		obj = o.Instance()
		if o.IsPrimary() {
			var val, err = o.Value()
			if err != nil {
				return nil, fmt.Errorf(
					"error getting primary key value for field %q in object %T: %w",
					o.Name(), obj, err,
				)
			}

			if !fields.IsZero(val) {
				return val, nil
			}

			goto createKey
		}

		objDefs = obj.FieldDefs()

	default:
		return nil, fmt.Errorf(
			"unexpected type for model object %T, expected attrs.Definer or attrs.Definitions",
			modelObject,
		)
	}

createKey:
	var (
		modelMeta    = attrs.GetModelMeta(obj)
		primaryField = objDefs.Primary()
		primaryVal   any
		err          error
	)

	if primaryField != nil {
		primaryVal, err = primaryField.Value()
		if err != nil {
			return nil, fmt.Errorf(
				"error getting primary key value for object %T: %w",
				obj, err,
			)
		}

		if !fields.IsZero(primaryVal) {
			return primaryVal, nil
		}
	}

	var uniqueFields = getUniqueFields(modelMeta)
	if len(uniqueFields) == 0 {
		return nil, fmt.Errorf(
			"model %T (%v) has no unique fields or unique together fields, cannot generate unique key: %w",
			obj, primaryVal, query_errors.ErrNoUniqueKey,
		)
	}

uniqueFieldsLoop:
	for _, fieldNames := range uniqueFields {
		var uniqueKeyParts = make([]string, 0, len(uniqueFields)*2)
		for _, fieldName := range fieldNames {
			var field, ok = objDefs.Field(fieldName)
			if !ok {
				panic(fmt.Sprintf("field %q not found in model %T", fieldName, obj))
			}

			var val, err = field.Value()
			if err != nil {
				return nil, fmt.Errorf(
					"error getting value for field %q in model %T: %w",
					fieldName, obj, err,
				)
			}

			if val == nil || fields.IsZero(val) {
				continue uniqueFieldsLoop
			}

			uniqueKeyParts = append(uniqueKeyParts, fmt.Sprintf("%s:%v", fieldName, val))
		}

		if len(uniqueKeyParts) == 0 {
			continue uniqueFieldsLoop
		}

		return strings.Join(uniqueKeyParts, ":"), nil
	}

	return nil, fmt.Errorf(
		"model %T has does not have enough unique fields or unique together fields set to generate a unique key: %w",
		obj, query_errors.ErrNoUniqueKey,
	)
}

// Registers a function to generate a where clause for a model
// without a primary key defined.
//
// This function will be called when a model object needs to be referenced
// in the queryset, for example when updating or deleting objects.
//
// See [GenerateObjectsWhereClause] for the implementation details when a primary key is defined.
var _, _ = attrs.OnModelRegister.Listen(func(s signals.Signal[attrs.Definer], d attrs.Definer) error {
	var (
		modelMeta = attrs.GetModelMeta(d)
		modelDefs = modelMeta.Definitions()
		primary   = modelDefs.Primary()
	)

	// See [GenerateObjectsWhereClause] for the implementation details
	// when a primary key is defined.
	if primary != nil {
		return nil
	}

	var uniqueFields = getUniqueFields(modelMeta)
	if len(uniqueFields) == 0 {
		return fmt.Errorf(
			"model %T has no unique fields or unique together fields, cannot generate where clause: %w",
			d, query_errors.ErrFieldNotFound,
		)
	}

	attrs.StoreOnMeta(d, __generate_where_filters_key, func(objects []attrs.Definer) ([]expr.LogicalExpression, error) {
		var (
			expressions = make([]expr.Expression, 0, len(uniqueFields))
			defs        = objects[0].FieldDefs()
		)

	uniqueFieldsLoop:
		for _, fieldNames := range uniqueFields {
			var and = make([]expr.Expression, 0, len(fieldNames))
			for _, fieldName := range fieldNames {
				var field, ok = defs.Field(fieldName)
				if !ok {
					panic(fmt.Sprintf("field %q not found in model %T", fieldName, d))
				}

				var val, err = field.Value()
				if err != nil {
					return nil, fmt.Errorf(
						"error getting value for field %q in model %T: %w",
						fieldName, d, err,
					)
				}

				if val == nil || fields.IsZero(val) {
					continue uniqueFieldsLoop
				}

				and = append(and, expr.Q(fieldName, val))
			}

			if len(and) == 0 {
				continue uniqueFieldsLoop
			}

			expressions = append(expressions, expr.And(and...).(expr.LogicalExpression))
		}

		if len(expressions) == 0 {
			return nil, fmt.Errorf(
				"model %T has does not have enough unique fields or unique together fields set to generate a where clause: %w",
				d, query_errors.ErrNoWhereClause,
			)
		}

		return []expr.LogicalExpression{expr.Or(expressions...).(expr.LogicalExpression)}, nil
	})
	return nil
})

// Registers a function to generate a where clause for a through model
// without a primary key defined.
//
// This function will be called when a through object needs to be referenced
// in the queryset, for example when deleting through objects.
//
// See [GenerateObjectsWhereClause] for the implementation details
// when a primary key is defined.
//
// It generates a where clause for a list of through model objects
// that match the source and target fields of the through model meta.
var _, _ = attrs.OnThroughModelRegister.Listen(func(s signals.Signal[attrs.ThroughModelMeta], d attrs.ThroughModelMeta) error {

	var (
		throughModel   = d.ThroughInfo.Model()
		sourceFieldStr = d.ThroughInfo.SourceField()
		targetFieldStr = d.ThroughInfo.TargetField()
		throughDefs    = throughModel.FieldDefs()
	)

	if _, ok := throughDefs.Field(sourceFieldStr); !ok {
		panic("source field not found in through model meta")
	}

	if _, ok := throughDefs.Field(targetFieldStr); !ok {
		panic("target field not found in through model meta")
	}

	// See [GenerateObjectsWhereClause] for the implementation details
	// when a primary key is defined.
	if throughDefs.Primary() == nil {
		attrs.StoreOnMeta(throughModel, __generate_where_filters_key, func(objects []attrs.Definer) ([]expr.LogicalExpression, error) {

			// groups of source object ids to target ids
			var groups = orderedmap.NewOrderedMap[any, []any]()

			for _, object := range objects {
				var (
					err                      error
					ok                       bool
					sourceField, targetField attrs.Field
					sourceVal, targetVal     any
					group                    []any

					instDefs = object.FieldDefs()
				)

				if sourceField, ok = instDefs.Field(sourceFieldStr); !ok {
					panic("source field not found in through model meta")
				}

				if targetField, ok = instDefs.Field(targetFieldStr); !ok {
					panic("target field not found in through model meta")
				}

				sourceVal, err = sourceField.Value()
				if err != nil {
					panic(err)
				}

				targetVal, err = targetField.Value()
				if err != nil {
					panic(err)
				}

				if sourceVal == nil || targetVal == nil {
					return nil, fmt.Errorf(
						"source or target field value is nil for object %T, source: %v, target: %v",
						object, sourceVal, targetVal,
					)
				}

				group, ok = groups.Get(sourceVal)
				if !ok {
					group = make([]any, 0, 1)
					groups.Set(sourceVal, group)
				}

				group = append(group, targetVal)
				groups.Set(sourceVal, group)
			}

			var expressions = make([]expr.LogicalExpression, 0, groups.Len())
			for head := groups.Front(); head != nil; head = head.Next() {
				var (
					source  = head.Key
					targets = head.Value
				)

				if len(targets) == 0 {
					continue
				}

				var sourceExpr = expr.Q(d.ThroughInfo.SourceField(), source)
				if len(targets) == 1 {
					expressions = append(expressions, expr.Express(
						sourceExpr,
						expr.Q(d.ThroughInfo.TargetField(), targets[0]),
					)...)
					continue
				}

				var targetExprs = make([]expr.Expression, 0, len(targets))
				for _, target := range targets {
					targetExprs = append(targetExprs, expr.Q(d.ThroughInfo.TargetField(), target))
				}

				expressions = append(expressions, sourceExpr.And(
					expr.Expr(d.ThroughInfo.TargetField(), "in", targets...),
				))
			}

			return expressions, nil
		})
	}

	return nil
})
