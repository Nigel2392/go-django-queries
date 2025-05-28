package queries

import (
	"iter"
	"strings"

	"github.com/Nigel2392/go-django-queries/src/query_errors"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/errs"
	"github.com/pkg/errors"
)

const errStopIteration errs.Error = "stop iteration"

// A row represents a single row in the result set of a QuerySet.
//
// It contains the model object, a map of annotations, and a pointer to the QuerySet.
//
// The annotations map contains additional data that is not part of the model object,
// such as calculated fields or additional information derived from the query.
type Row[T attrs.Definer] struct {
	Object      T
	Annotations map[string]any
	QuerySet    *QuerySet[T]
}

type Rows[T attrs.Definer] []*Row[T]

func (r Rows[T]) Len() int {
	return len(r)
}

func (r Rows[T]) FilterFunc(fn func(*Row[T]) bool) Rows[T] {
	var filteredRows = make(Rows[T], 0, len(r))
	for _, row := range r {
		if fn(row) {
			filteredRows = append(filteredRows, row)
		}
	}
	return filteredRows
}

func (r Rows[T]) Values(fieldPaths ...string) []map[string]any {
	var values = make([]map[string]any, 0, len(r))
	var idx = 0

	if len(r) == 0 {
		return values // Return empty slice if there are no rows
	}

	if len(fieldPaths) == 0 {
		panic(errors.New("Values called with no field paths, please provide at least one field path"))
	}

	for i, row := range r {
		var valueMap = make(map[string]any, len(fieldPaths))

	pathLoop:
		for _, path := range fieldPaths {

			if row.Annotations != nil {
				if value, ok := row.Annotations[path]; ok {
					valueMap[path] = value // Use annotation value if it exists
					continue pathLoop
				}
			}

			splitPath := strings.Split(path, ".")

			err := walkFieldValues(row.Object.FieldDefs(), splitPath, &idx, 0, i, func(w walkInfo) bool {
				if w.field.GetValue() == nil {
					valueMap[path] = nil // Set nil if the value is nil
				} else {
					valueMap[path] = w.field.GetValue()
				}
				return true // Continue walking
			})
			if errors.Is(err, errStopIteration) {
				continue // Stop iteration if the yield function returned false
			}
			if err != nil && !errors.Is(err, query_errors.ErrFieldNotFound) {
				panic(errors.Wrapf(err, "error getting field %s from row", strings.Join(fieldPaths, ".")))
			}
			if err != nil {
				panic(errors.Wrapf(err, "error getting field %s from row", strings.Join(fieldPaths, ".")))
			}
		}

		values = append(values, valueMap)
	}
	return values
}

func (rows Rows[T]) Pluck(pathToField string) iter.Seq2[int, attrs.Field] {
	var (
		idx        = 0
		fieldNames = strings.Split(pathToField, ".")
	)

	return iter.Seq2[int, attrs.Field](func(yield func(int, attrs.Field) bool) {
		var yieldFn = func(w walkInfo) bool {
			return yield(idx, w.field)
		}

		for rowIdx, row := range rows {
			var err = walkFieldValues(row.Object.FieldDefs(), fieldNames, &idx, 0, rowIdx, yieldFn)
			if errors.Is(err, errStopIteration) {
				return // Stop iteration if the yield function returned false
			}
			if err != nil && errors.Is(err, query_errors.ErrFieldNotFound) {
				panic(errors.Wrapf(err, "error getting field %s from row", pathToField))
			} else if err != nil {
				panic(errors.Wrapf(err, "error getting field %s from row", pathToField))
			}
		}
	})
}

func PluckRowValues[ValueT any, ModelT attrs.Definer](rows Rows[ModelT], pathToField string) iter.Seq2[int, ValueT] {
	return func(yield func(int, ValueT) bool) {
		for idx, field := range rows.Pluck(pathToField) {
			var value = field.GetValue()
			if value == nil {
				if !yield(idx, *new(ValueT)) {
					return // Stop yielding if the yield function returns false
				}
			}
			if v, ok := value.(ValueT); ok {
				if !yield(idx, v) {
					return // Stop yielding if the yield function returns false
				}
			} else {
				panic(errors.Errorf("type mismatch on pluckRows[%T]: %v", *new(ValueT), value))
			}
		}
	}
}
