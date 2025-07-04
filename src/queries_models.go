package queries

import (
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Nigel2392/go-django-queries/src/expr"
	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/forms/fields"
)

const __GENERATE_WHERE_CLAUSE_FOR_OBJECTS = "queries.__GENERATE_WHERE_CLAUSE_FOR_OBJECTS"

// GenerateObjectsWhereClause generates a where clause for the given objects.
//
// This where clause is used to reference the given objects in the database.
//
//   - If the model has a primary key defined, it will
//     generate a where clause based on the primary key.
//
//   - If the model does not have a primary key defined,
//     it will try to generate a where clause based on
//     the unique fields or unique together fields.
//
//   - If no primary key, unique fields or unique together
//     fields are defined, it will return an error.
func GenerateObjectsWhereClause[T attrs.Definer](objects ...T) ([]expr.ClauseExpression, error) {

	if len(objects) == 0 {
		return []expr.ClauseExpression{}, nil
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
		var q, has = modelMeta.Storage(__GENERATE_WHERE_CLAUSE_FOR_OBJECTS)
		if !has {
			return nil, fmt.Errorf("model %T has no primary key defined and no function registered to generate a where clauuse", objects[0])
		}

		var or = make([]expr.Expression, 0, len(objects))
		switch q := q.(type) {
		case func([]attrs.Definer) ([]expr.ClauseExpression, error):
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

		case func(attrs.Definer) ([]expr.ClauseExpression, error):
			for _, object := range objects {
				var exprs, err = q(object)
				if err != nil {
					return nil, fmt.Errorf("error generating where clause for object %T: %w", object, err)
				}
				for _, expr := range exprs {
					or = append(or, expr)
				}
			}

		case func(attrs.Definer) (expr.ClauseExpression, error):
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

		return []expr.ClauseExpression{expr.Or(or...)}, nil
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

		return []expr.ClauseExpression{expr.Q(
			fmt.Sprintf("%s__in", primaryName), ids,
		)}, nil
	}
}

type keyPart struct {
	name  string
	value driver.Value
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

	var uniqueFields = getMetaUniqueFields(modelMeta)
	if len(uniqueFields) == 0 {
		return nil, fmt.Errorf(
			"model %T (%v) has no unique fields or unique together fields, cannot generate unique key: %w",
			obj, primaryVal, query_errors.ErrNoUniqueKey,
		)
	}

uniqueFieldsLoop:
	for _, fieldNames := range uniqueFields {
		var uniqueKeyParts = make([]keyPart, 0, len(uniqueFields)*2)
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

			uniqueKeyParts = append(uniqueKeyParts, keyPart{
				name:  fieldName,
				value: val,
			})
		}

		if len(uniqueKeyParts) == 0 {
			continue uniqueFieldsLoop
		}

		if len(uniqueKeyParts) == 1 {
			// If there is only one unique field, return its value directly
			return uniqueKeyParts[0].value, nil
		}

		var sb strings.Builder
		for i, part := range uniqueKeyParts {
			if i > 0 {
				sb.WriteString(":")
			}
			sb.WriteString(part.name)
			sb.WriteString(":")

			// ensure the value is properly formatted
			// otherwise this might cause a security issue
			// returning conflicting non- unique key values
			switch v := part.value.(type) {
			case string:
				sb.WriteString(url.QueryEscape(v)) // percent-encode
			case []byte:
				sb.WriteString(base64.StdEncoding.EncodeToString(v))
			case time.Time:
				sb.WriteString(v.UTC().Format(time.RFC3339Nano)) // consistent time format
			default:
				fmt.Fprintf(&sb, "%v", v)
			}
		}
	}

	return nil, fmt.Errorf(
		"model %T has does not have enough unique fields or unique together fields set to generate a unique key: %w",
		obj, query_errors.ErrNoUniqueKey,
	)
}
