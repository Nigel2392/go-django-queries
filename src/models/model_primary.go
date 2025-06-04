package models

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/assert"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type primaryField struct {
	attrs.Field
	model *Model
}

func (f *primaryField) SetValue(v any, force bool) error {
	assert.False(
		f.model == nil || f.model.internals == nil || f.model.internals.object == nil,
		"model is not initialized, cannot set value on primary field",
	)

	var val, _ = f.Field.Value()
	if val == v {
		return nil
	}

	f.Field.SetValue(v, force)
	f.model.initDefaults(f.model.internals.defs)
	return nil
}

func (f *primaryField) Scan(v any) error {
	assert.False(
		f.model == nil || f.model.internals == nil || f.model.internals.object == nil,
		"model is not initialized, cannot scan value on primary field",
	)

	var val, _ = f.Field.Value()
	if val == v {
		return nil
	}

	if err := f.Field.Scan(v); err != nil {
		return fmt.Errorf("failed to scan value for primary field %s: %w", f.Name(), err)
	}
	f.model.initDefaults(f.model.internals.defs)
	return nil
}
