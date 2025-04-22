package queries

import (
	"database/sql"
	"reflect"
	"slices"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/errs"
	"github.com/Nigel2392/go-django/src/core/logger"
	"github.com/Nigel2392/go-django/src/forms/fields"
	"github.com/Nigel2392/go-django/src/models"
	"github.com/pkg/errors"
)

const quote = `"`

func GetObject[T attrs.Definer](identifier any) (T, error) {
	var obj = newDefiner[T]()
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return obj, err
	}

	var (
		primaryField = queryInfo.definitions.Primary()
		query        strings.Builder
		args         []any
	)

	if err = primaryField.SetValue(identifier, true); err != nil {
		return obj, err
	}

	primaryValue, err := primaryField.Value()
	if err != nil {
		return obj, err
	}

	if fields.IsZero(primaryValue) {
		return obj, errors.Wrapf(
			ErrFieldNull,
			"Primary field %q cannot be null",
			primaryField.Name(),
		)
	}

	query.WriteString("SELECT * FROM ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" WHERE ")
	query.WriteString(primaryField.ColumnName())
	query.WriteString(" = ?")
	args = append(args, primaryValue)

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("GetObject (%T, %v): %s", obj, primaryValue, dbSpecific)
	var results = queryInfo.dbx.QueryRow(dbSpecific, args...)
	err = results.Scan(attrs.InterfaceList(queryInfo.fields)...)
	return obj, err
}

//
//func GetObjectByField[T attrs.Definer](obj T, fieldName string, value ...any) ([]T, error) {
//	var queryInfo, err = getQueryInfo(obj)
//	if err != nil {
//		return nil, err
//	}
//
//	if len(value) == 0 {
//		return nil, errors.New("value cannot be empty")
//	}
//
//	var f, ok = queryInfo.fields_map[fieldName]
//	if !ok {
//		return nil, errors.Errorf("field %q not found", fieldName)
//	}
//
//	var query = strings.Builder{}
//	var args = make([]any, 0, len(value)+2)
//	query.WriteString("SELECT * FROM ")
//	query.WriteString(queryInfo.tableName)
//	query.WriteString(" WHERE ")
//	query.WriteString(f.ColumnName())
//
//	if len(value) > 1 {
//		query.WriteString(" IN (")
//		for i, v := range value {
//			if i > 0 {
//				query.WriteString(", ")
//			}
//			query.WriteString("?")
//			args = append(args, v)
//		}
//		query.WriteString(")")
//	} else {
//		query.WriteString(" = ?")
//		args = append(args, value[0])
//	}
//
//	query.WriteString(" LIMIT ?")
//	query.WriteString(" OFFSET ?")
//	args = append(args, 1, 0)
//
//	var dbSpecific = queryInfo.dbx.Rebind(query.String())
//	logger.Debugf("GetObjectByField (%T, %q): %s", obj, fieldName, dbSpecific)
//	results, err := queryInfo.dbx.Query(dbSpecific, args...)
//	if err != nil {
//		return nil, err
//	}
//
//	var rT = reflect.TypeOf(obj)
//	if rT.Kind() != reflect.Ptr {
//		return nil, errors.New("object must be a pointer to a struct")
//	}
//
//	rT = rT.Elem()
//
//	var newList = make([]T, 0, len(value))
//	for results.Next() {
//		var newObj = reflect.New(rT).Interface().(T)
//		var fieldDefs = newObj.FieldDefs()
//		err = results.Scan(
//			attrs.InterfaceList(fieldDefs.Fields())...,
//		)
//		if err != nil {
//			if errors.Is(err, sql.ErrNoRows) {
//				break
//			}
//			return nil, err
//		}
//		newList = append(newList, newObj)
//	}
//
//	return newList, err
//}

func CT_GetObject[T attrs.Definer](identifier any) (interface{}, error) {
	var obj, err = GetObject[T](identifier)
	return obj, err
}
func CT_ListObjects[T attrs.Definer](amount, offset uint) ([]interface{}, error) {
	var results, err = ListObjects[T](uint64(offset), uint64(amount))
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}
func CT_ListObjectsByIDs[T attrs.Definer](i []interface{}) ([]interface{}, error) {
	var results, err = ListObjectsByIDs[T](0, 1000, i)
	if err != nil {
		return nil, err
	}
	return attrs.InterfaceList(results), nil
}

func ListObjectsByIDs[T attrs.Definer, T2 any](offset, limit uint64, ids []T2) ([]T, error) {
	var obj = newDefiner[T]()
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return nil, err
	}

	var (
		primaryField = queryInfo.definitions.Primary()
		query        strings.Builder
		args         []any
	)

	if len(ids) == 0 {
		return nil, errors.New("ids cannot be empty")
	}

	query.WriteString("SELECT * FROM ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" WHERE ")
	query.WriteString(primaryField.ColumnName())

	query.WriteString(" IN (")
	for i, v := range ids {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteString("?")
		args = append(args, v)
	}
	query.WriteString(")")

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("ListObjectsByIDs (%T): %s", obj, dbSpecific)
	results, err := queryInfo.dbx.Query(dbSpecific, args...)
	if err != nil {
		return nil, err
	}

	var newList = make([]T, 0, limit)
	var rT = reflect.TypeOf(obj)
	if rT.Kind() != reflect.Ptr {
		return nil, errors.New("object must be a pointer to a struct")
	}

	rT = rT.Elem()

	for results.Next() {
		var newObj = reflect.New(rT).Interface().(T)
		var fieldDefs = newObj.FieldDefs()
		err = results.Scan(
			attrs.InterfaceList(fieldDefs.Fields())...,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		newList = append(newList, newObj)
	}

	return newList, err
}

func ListObjects[T attrs.Definer](offset, limit uint64, ordering ...string) ([]T, error) {
	var obj = newDefiner[T]()
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return nil, err
	}

	var (
		primaryField = queryInfo.definitions.Primary()
		primaryName  = primaryField.ColumnName()
		fieldNames   = make([]string, 0, len(queryInfo.fields))
	)
	for _, field := range queryInfo.fields {
		fieldNames = append(fieldNames, field.ColumnName())
	}

	var orderer = models.Orderer{
		Quote:     quote,
		Default:   "-" + primaryName,
		TableName: queryInfo.tableName,
		Fields:    ordering,
		Validate: func(field string) bool {
			return slices.Contains(fieldNames, field)
		},
	}

	orderStr, err := orderer.Build()
	if err != nil {
		return nil, err
	}

	var query = strings.Builder{}
	query.WriteString("SELECT ")
	for i, name := range fieldNames {
		if i > 0 {
			query.WriteString(", ")
		}

		query.Grow(len(name) + len(queryInfo.tableName) + 1 + (len(quote) * 4))
		query.WriteString(quote)
		query.WriteString(queryInfo.tableName)
		query.WriteString(quote)
		query.WriteString(".")
		query.WriteString(quote)
		query.WriteString(name)
		query.WriteString(quote)
	}

	query.WriteString(" FROM ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" ORDER BY ")
	query.WriteString(orderStr)
	query.WriteString(" LIMIT ? OFFSET ?")

	var args = make([]any, 2)
	args[0] = limit
	args[1] = offset

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("ListObjects (%T): %s", obj, dbSpecific)
	results, err := queryInfo.dbx.Query(dbSpecific, args...)
	if err != nil {
		return nil, err
	}

	var newList = make([]T, 0, limit)
	var rT = reflect.TypeOf(obj)
	if rT.Kind() != reflect.Ptr {
		return nil, errors.New("object must be a pointer to a struct")
	}

	rT = rT.Elem()

	for results.Next() {
		var newObj = reflect.New(rT).Interface().(T)
		var fieldDefs = newObj.FieldDefs()
		err = results.Scan(
			attrs.InterfaceList(fieldDefs.Fields())...,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		newList = append(newList, newObj)
	}

	return newList, err
}

func CountObjects[T attrs.Definer](obj T) (int64, error) {
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return 0, err
	}

	var (
		query strings.Builder
		args  []any
	)

	query.WriteString("SELECT COUNT(*) FROM ")
	query.WriteString(queryInfo.tableName)

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("CountObjects (%T): %s", obj, dbSpecific)

	var count int64
	err = queryInfo.dbx.Get(&count, dbSpecific, args...)
	return count, err
}

func SaveObject[T attrs.Definer](obj T) error {
	var fieldDefs = obj.FieldDefs()
	var primaryField = fieldDefs.Primary()
	var primaryValue, err = primaryField.Value()
	if err != nil {
		return err
	}
	if fields.IsZero(primaryValue) {
		return CreateObject(obj)
	}
	return UpdateObject(obj)
}

func CreateObject[T attrs.Definer](obj T) error {
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return err
	}

	var (
		written      bool
		primaryField = queryInfo.definitions.Primary()
		query        strings.Builder
		args         []any
		tx           = queryInfo.dbx.MustBegin()
	)

	query.WriteString("INSERT INTO ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" (")

	for _, field := range queryInfo.fields {
		if field.IsPrimary() || !field.AllowEdit() {
			continue
		}

		var value, err = field.Value()
		if err != nil {
			return err
		}

		if value == nil && !field.AllowNull() {
			return errors.Wrapf(
				ErrFieldNull,
				"Field %q cannot be null",
				field.Name(),
			)
		}

		if written {
			query.WriteString(", ")
		}

		query.WriteString(field.ColumnName())
		args = append(args, value)
		written = true
	}

	query.WriteString(") VALUES (")
	for i := 0; i < len(args); i++ {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteString("?")
	}
	query.WriteString(")")

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("CreateObject (%T): %s", obj, dbSpecific)
	result, err := tx.Exec(dbSpecific, args...)
	if err != nil {
		return err
	}

	lastId, err := result.LastInsertId()
	if err != nil {
		return errs.WrapErrors(
			ErrLastInsertId,
			tx.Rollback(),
			err,
		)
	}

	err = primaryField.Scan(lastId)
	if err != nil {
		return errs.WrapErrors(
			err,
			tx.Rollback(),
		)
	}

	return tx.Commit()
}

func UpdateObject[T attrs.Definer](obj T) error {
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return err
	}

	var (
		written      bool
		primaryField = queryInfo.definitions.Primary()
		query        strings.Builder
		args         []any
	)

	primaryValue, err := primaryField.Value()
	if err != nil {
		return err
	}

	if fields.IsZero(primaryValue) {
		return errors.Wrapf(
			ErrFieldNull,
			"Primary field %q cannot be null",
			primaryField.Name(),
		)
	}

	query.WriteString("UPDATE ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" SET ")

	for _, field := range queryInfo.fields {
		if field.IsPrimary() || !field.AllowEdit() {
			continue
		}

		var value, err = field.Value()
		if err != nil {
			return err
		}

		if value == nil && !field.AllowNull() {
			return errors.Wrapf(
				ErrFieldNull,
				"Field %q cannot be null",
				field.Name(),
			)
		}

		if written {
			query.WriteString(", ")
		}

		query.WriteString(field.ColumnName())
		query.WriteString(" = ?")
		args = append(args, value)
		written = true
	}

	query.WriteString(" WHERE ")
	query.WriteString(primaryField.ColumnName())
	query.WriteString(" = ?")
	args = append(args, primaryValue)

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("UpdateObject (%T, %v): %s", obj, primaryValue, dbSpecific)
	_, err = queryInfo.dbx.Exec(dbSpecific, args...)
	return err
}

func DeleteObject[T attrs.Definer](obj T) error {
	var queryInfo, err = getQueryInfo(obj)
	if err != nil {
		return err
	}

	var (
		primaryField = queryInfo.definitions.Primary()
		query        strings.Builder
		args         []any
	)

	primaryValue, err := primaryField.Value()
	if err != nil {
		return err
	}

	if fields.IsZero(primaryValue) {
		return errors.Wrapf(
			ErrFieldNull,
			"Primary field %q cannot be null",
			primaryField.Name(),
		)
	}

	query.WriteString("DELETE FROM ")
	query.WriteString(queryInfo.tableName)
	query.WriteString(" WHERE ")
	query.WriteString(primaryField.ColumnName())
	query.WriteString(" = ?")
	args = append(args, primaryValue)

	var dbSpecific = queryInfo.dbx.Rebind(query.String())
	logger.Debugf("DeleteObject (%T, %v): %s", obj, primaryValue, dbSpecific)
	_, err = queryInfo.dbx.Exec(dbSpecific, args...)
	return err
}
