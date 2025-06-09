package models_test

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Nigel2392/go-django-queries/src/fields"
	"github.com/Nigel2392/go-django-queries/src/models"
	"github.com/Nigel2392/go-django-queries/src/quest"
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ImageModel struct {
	models.Model
	ID         int64
	ImageTitle string
	ImageURL   string
}

func (m *ImageModel) FieldDefs() attrs.Definitions {
	return m.Model.Define(m,
		attrs.Unbound("ID", &attrs.FieldConfig{Primary: true}),
		attrs.Unbound("ImageTitle"),
		attrs.Unbound("ImageURL"),
	)
}

type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	return json.Marshal(map[string]any(m))
}

func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type for JSONMap: %T", value)
	}

	return json.Unmarshal(data, m)
}

type StatefulModel struct {
	models.Model
	ID        int64
	FirstName string
	LastName  string
	Age       int
	BinData   []byte
	MapData   JSONMap
	Image     *ImageModel
}

func (m *StatefulModel) FieldDefs() attrs.Definitions {
	return m.Model.Define(m,
		attrs.Unbound("ID", &attrs.FieldConfig{Primary: true}),
		attrs.Unbound("FirstName"),
		attrs.Unbound("LastName"),
		attrs.Unbound("Age"),
		attrs.Unbound("BinData"),
		attrs.Unbound("MapData"),
		fields.ForeignKey[*ImageModel]("Image"),
	)
}

func TestState(t *testing.T) {
	var tables = quest.Table(t,
		&ImageModel{},
		&StatefulModel{},
	)

	tables.Create()
	defer tables.Drop()

	var model = models.Setup(&StatefulModel{
		FirstName: "John",
		LastName:  "Doe",
		Age:       30,
		BinData:   []byte{1, 2, 3},
		MapData: JSONMap{
			"key1": "value1",
			"key2": 42,
		},
		Image: &ImageModel{
			ImageTitle: "Sample Image",
			ImageURL:   "http://example.com/image.jpg",
		},
	})

	t.Run("StateNilBeforeFieldDefsCalled", func(t *testing.T) {
		var state = model.State()
		if state != nil {
			t.Errorf("Expected state to be nil, got: %v", state)
		}
	})

	t.Run("StateChangedAfterSetFirstName", func(t *testing.T) {
		var defs = model.FieldDefs()
		var state = model.State()
		if state == nil {
			t.Error("Expected state to be non-nil after change")
		}

		t.Run("InitialStateUnchanged", func(t *testing.T) {
			if state.Changed(false) {
				t.Error("Expected initial state to be unchanged")
			}
			if state.Changed(true) {
				t.Error("Expected initial state to be unchanged with checkState")
			}
		})

		defs.Set("FirstName", "Jane")

		t.Run("StateChanged", func(t *testing.T) {
			if !state.Changed(false) {
				t.Error("Expected state to be changed after modifying FirstName")
			}
		})

		t.Run("FirstNameChanged", func(t *testing.T) {
			if !state.HasChanged("FirstName") {
				t.Error("Expected FirstName to be marked as changed")
			}
		})

		t.Run("StateUnchangedAfterReset", func(t *testing.T) {
			var state = model.State()
			if state == nil {
				t.Error("Expected state to be non-nil after change")
			}

			if !state.Changed(false) {
				t.Error("Expected state to be changed after test \"StateChangedAfterSetFirstName\"")
			}

			state.Reset()

			if state.Changed(false) {
				t.Error("Expected state to be unchanged after reset")
			}

			if state.Changed(true) {
				t.Error("Expected state to be unchanged after reset with checkState")
			}
		})
	})
}
