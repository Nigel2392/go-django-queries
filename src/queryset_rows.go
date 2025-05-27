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

type Row[T attrs.Definer] struct {
	Object      T
	Annotations map[string]any
	QuerySet    *QuerySet[T]
}

type Rows[T attrs.Definer] []*Row[T]

func (r Rows[T]) Len() int {
	return len(r)
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

		for _, row := range rows {
			var err = walkFieldValues(row.Object.FieldDefs(), fieldNames, &idx, 0, yieldFn)
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
