package migrator

import (
	"encoding/json"
	"reflect"
	"slices"

	"github.com/Nigel2392/go-django-queries/internal"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-django/src/core/contenttypes"
	"github.com/elliotchance/orderedmap/v2"
)

var _ Table = (*ModelTable)(nil)

type Changed[T any] struct {
	Old T `json:"old,omitempty"`
	New T `json:"new,omitempty"`
}

func unchanged[T any](v T) *Changed[T] {
	var t T
	return &Changed[T]{
		Old: t,
		New: v,
	}
}

func changed[T any](old, new T) *Changed[T] {
	return &Changed[T]{
		Old: old,
		New: new,
	}
}

type Index struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
	Comment string   `json:"comment,omitempty"`
}

type ModelTable struct {
	Object attrs.Definer
	Table  string
	Desc   string
	Fields *orderedmap.OrderedMap[string, Column]
	Index  []Index
}

func NewModelTable(object attrs.Definer) *ModelTable {

	var (
		defs   = object.FieldDefs()
		fields = defs.Fields()
	)

	var t = ModelTable{
		Table:  defs.TableName(),
		Object: object,
		Fields: orderedmap.NewOrderedMap[string, Column](),
	}

	// Move primary fields to the front of the list
	slices.SortStableFunc(fields, func(a, b attrs.Field) int {
		if a.IsPrimary() && !b.IsPrimary() {
			return -1
		}
		if !a.IsPrimary() && b.IsPrimary() {
			return 1
		}
		return 0
	})

	for _, field := range fields {
		var atts = field.Attrs()

		attrUseInDB, ok := internal.GetFromAttrs[bool](atts, AttrUseInDBKey)
		if !ok {
			attrUseInDB = true
		}

		attrMaxLength, _ := internal.GetFromAttrs[int64](atts, attrs.AttrMaxLengthKey)
		attrMinLength, _ := internal.GetFromAttrs[int64](atts, attrs.AttrMinLengthKey)
		attrMinValue, _ := internal.GetFromAttrs[float64](atts, attrs.AttrMinValueKey)
		attrMaxValue, _ := internal.GetFromAttrs[float64](atts, attrs.AttrMaxValueKey)
		attrAutoIncrement, _ := internal.GetFromAttrs[bool](atts, attrs.AttrAutoIncrementKey)
		attrUnique, _ := internal.GetFromAttrs[bool](atts, attrs.AttrUniqueKey)
		attrReverseAlias, _ := internal.GetFromAttrs[string](atts, attrs.AttrReverseAliasKey)
		attrOnDelete, _ := internal.GetFromAttrs[Action](atts, AttrOnDeleteKey)
		attrOnUpdate, _ := internal.GetFromAttrs[Action](atts, AttrOnUpdateKey)

		var rel *MigrationRelation
		var fRel = field.Rel()
		if fRel != nil {

			var cType = contenttypes.NewContentType(
				fRel.Model(),
			)

			rel = &MigrationRelation{
				Type:        fRel.Type(),
				TargetModel: cType,
				TargetField: fRel.Field(),
				OnDelete:    attrOnDelete,
				OnUpdate:    attrOnUpdate,
			}

			var through = fRel.Through()
			if through != nil {
				rel.Through = &MigrationRelationThrough{
					Model:       contenttypes.NewContentType(through.Model()),
					SourceField: through.SourceField(),
					TargetField: through.TargetField(),
				}
			}
		}

		t.Fields.Set(field.Name(), Column{
			Table:        &t,
			Field:        field,
			Name:         field.Name(),
			Column:       field.ColumnName(),
			UseInDB:      attrUseInDB,
			MinLength:    attrMinLength,
			MaxLength:    attrMaxLength,
			MinValue:     attrMinValue,
			MaxValue:     attrMaxValue,
			Unique:       attrUnique,
			Auto:         attrAutoIncrement,
			Nullable:     field.AllowNull(),
			Primary:      field.IsPrimary(),
			Default:      field.GetDefault(),
			ReverseAlias: attrReverseAlias,
			Rel:          rel,
		})
	}

	return &t
}

type serializableModelTable struct {
	Table   string                                       `json:"table"`
	Model   *contenttypes.BaseContentType[attrs.Definer] `json:"model"`
	Fields  []*Column                                    `json:"fields"`
	Indexes []Index                                      `json:"indexes"`
	Comment string                                       `json:"comment"`
}

func (t *ModelTable) MarshalJSON() ([]byte, error) {
	var s = serializableModelTable{
		Table:   t.TableName(),
		Model:   contenttypes.NewContentType(t.Object),
		Indexes: t.Indexes(),
		Comment: t.Comment(),
		Fields:  make([]*Column, 0, t.Fields.Len()),
	}

	for head := t.Fields.Front(); head != nil; head = head.Next() {
		s.Fields = append(s.Fields, &head.Value)
	}

	return json.Marshal(s)
}

func (t *ModelTable) UnmarshalJSON(data []byte) error {
	var s serializableModelTable
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	t.Table = s.Table
	t.Desc = s.Comment
	t.Object = s.Model.New()
	t.Fields = orderedmap.NewOrderedMap[string, Column]()
	t.Index = s.Indexes

	var defs = t.Object.FieldDefs()
	for _, col := range s.Fields {
		col.Table = t
		var f, ok = defs.Field(col.Name)
		if ok {
			col.Field = f
		}

		t.Fields.Set(col.Name, *col)
	}

	return nil
}

func (t *ModelTable) ModelName() string {
	var rt = reflect.TypeOf(t.Object)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	return rt.Name()
}

func (t *ModelTable) TableName() string {
	if t.Table != "" {
		return t.Table
	}

	var defs = t.Object.FieldDefs()
	return defs.TableName()
}

func (t *ModelTable) Model() attrs.Definer {
	return t.Object
}

func (t *ModelTable) Columns() []*Column {
	if t.Fields == nil {
		return nil
	}
	var cols = make([]*Column, 0, t.Fields.Len())
	for head := t.Fields.Front(); head != nil; head = head.Next() {
		cols = append(cols, &head.Value)
	}
	return cols
}

func (t *ModelTable) Comment() string {
	return t.Desc
}

func (t *ModelTable) Indexes() []Index {
	return t.Index
}

func (t *ModelTable) Diff(other *ModelTable) (added, removed []Column, diffs []Changed[Column]) {
	if t == nil && other == nil {
		return nil, nil, nil
	}
	if t == nil || other == nil {
		return nil, nil, nil
	}

	for head := other.Fields.Front(); head != nil; head = head.Next() {
		var col = head.Value
		var _, ok = t.Fields.Get(col.Name)
		if !ok {
			removed = append(removed, col)
			continue
		}
	}

	for head := t.Fields.Front(); head != nil; head = head.Next() {
		var col = head.Value
		var otherCol, ok = other.Fields.Get(col.Name)
		if !ok {
			added = append(added, col)
			continue
		}

		if !col.Equals(&otherCol) {
			diffs = append(diffs, Changed[Column]{
				Old: otherCol,
				New: col,
			})
		}
	}

	return added, removed, diffs
}
