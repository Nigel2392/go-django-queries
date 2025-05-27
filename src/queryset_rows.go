package queries

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/errs"
)

const errStopIteration errs.Error = "stop iteration"

type Row[T attrs.Definer] struct {
	Object      T
	Annotations map[string]any
	QuerySet    *QuerySet[T]
}

type Rows[T attrs.Definer] []*Row[T]

func (r Rows[T]) Len() int {
	return len(r)
}

//
//func (rows Rows[T]) Pluck(pathToField string) iter.Seq2[int, attrs.Field] {
//	var (
//		idx        = 0
//		fieldNames = strings.Split(pathToField, ".")
//		getField   func(obj *object, depth int, yield func(int, attrs.Field) bool) error
//	)
//	getField = func(obj *object, depth int, yield func(int, attrs.Field) bool) error {
//		if depth >= len(fieldNames) {
//			return nil // No more fields to traverse
//		}
//
//		var fieldName = fieldNames[depth]
//		var field, ok = obj.fieldDefs.Field(fieldName)
//		if ok {
//			if depth == len(fieldNames)-1 {
//				if !yield(idx, field) {
//					return errStopIteration // Stop yielding if the yield function returns false
//				}
//				idx++      // Increment index for the next field found
//				return nil // Found the field at the last depth
//			}
//
//			for objFieldName, relObj := range obj.relations {
//				if objFieldName != fieldName {
//					continue // Not the field we are looking for
//				}
//
//				for head := relObj.objects.Front(); head != nil; head = head.Next() {
//					var err = getField(head.Value, depth+1, yield)
//					if err != nil {
//						return err
//					}
//				}
//			}
//		}
//
//		return query_errors.ErrFieldNotFound
//	}
//
//	return iter.Seq2[int, attrs.Field](func(yield func(int, attrs.Field) bool) {
//		for _, row := range rows {
//			var err = getField(row.object.object, 0, yield)
//			if err == errStopIteration {
//				return // Stop iteration if the yield function returned false
//			}
//			if err != nil && errors.Is(err, query_errors.ErrFieldNotFound) {
//				continue // Skip rows where the field is not found
//			} else if err != nil {
//				panic(errors.Wrapf(err, "error getting field %s from row", pathToField))
//			}
//		}
//	})
//}
//
//func PluckRowValues[ValueT any, ModelT attrs.Definer](rows Rows[ModelT], pathToField string) iter.Seq2[int, ValueT] {
//	return func(yield func(int, ValueT) bool) {
//		for idx, field := range rows.Pluck(pathToField) {
//			var value = field.GetValue()
//			if value == nil {
//				if !yield(idx, *new(ValueT)) {
//					return // Stop yielding if the yield function returns false
//				}
//			}
//			if v, ok := value.(ValueT); ok {
//				if !yield(idx, v) {
//					return // Stop yielding if the yield function returns false
//				}
//			} else {
//				panic(errors.Errorf("type mismatch on pluckRows[%T]: %v", *new(ValueT), value))
//			}
//		}
//	}
//}
//
