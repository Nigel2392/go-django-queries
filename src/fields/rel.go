package fields

import (
	"fmt"

	queries "github.com/Nigel2392/go-django-queries/src"
	"github.com/Nigel2392/go-django-queries/src/migrator"
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/elliotchance/orderedmap/v2"
	"github.com/pkg/errors"
)

var (
	_ queries.ForUseInQueriesField = (*RelationField[any])(nil)
	_ attrs.CanRelatedName         = (*RelationField[any])(nil)
)

type RelationField[T any] struct {
	*DataModelField[T]
	rel  attrs.Relation
	name string
	col  string
}

type typedRelation struct {
	attrs.Relation
	typ attrs.RelationType
}

func (r *typedRelation) Type() attrs.RelationType {
	return r.typ
}

type wrappedRelation struct {
	attrs.Relation
	from attrs.RelationTarget
}

func (r *wrappedRelation) From() attrs.RelationTarget {
	if r.from == nil {
		return r.Relation.From()
	}
	return r.from
}

type relationTarget struct {
	model    attrs.Definer
	field    attrs.FieldDefinition
	fieldStr string
	prev     attrs.RelationTarget
}

func (r *relationTarget) From() attrs.RelationTarget {
	return r.prev
}

func (r *relationTarget) Model() attrs.Definer {
	return r.model
}

func (r *relationTarget) Field() attrs.FieldDefinition {
	if r.field != nil {
		return r.field
	}

	var meta = attrs.GetModelMeta(r.model)
	var defs = meta.Definitions()
	if r.fieldStr != "" {
		var ok bool
		r.field, ok = defs.Field(r.fieldStr)
		if !ok {
			panic(fmt.Errorf("field %q not found in model %T", r.fieldStr, r.model))
		}
	} else {
		r.field = defs.Primary()
	}

	return r.field
}

func NewRelatedField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *RelationField[T] {
	//var (
	//	inst = field.Instance()
	//	defs = inst.FieldDefs()
	//)

	var f = &RelationField[T]{
		DataModelField: NewDataModelField[T](forModel, dst, name),
		col:            columnName,
		name:           reverseName,
		rel:            rel,
	}
	f.DataModelField.fieldRef = f // Set the field reference to itself
	return f
}

func (m *RelationField[T]) Name() string {
	return m.DataModelField.Name()
}

func (m *RelationField[T]) ForSelectAll() bool {
	return false
}

func (r *RelationField[T]) ColumnName() string {
	if r.col == "" {
		var from = r.rel.From()
		if from != nil {
			return from.Field().ColumnName()
		}
	}
	return r.col
}

func (r *RelationField[T]) GetTargetField() attrs.FieldDefinition {
	var targetField = r.rel.Field()
	if targetField == nil {
		var defs = r.rel.Model().FieldDefs()
		return defs.Primary()
	}
	return targetField
}

func (r *RelationField[T]) IsReverse() bool {
	var targetField = r.GetTargetField()
	if targetField == nil || targetField.IsPrimary() {
		return false
	}
	return true
}

func (r *RelationField[T]) Attrs() map[string]any {
	var atts = make(map[string]any)
	atts[attrs.AttrNameKey] = r.Name()
	atts[migrator.AttrUseInDBKey] = r.rel.Through() == nil && !r.IsReverse()
	return atts
}

func (r *RelationField[T]) RelatedName() string {
	return r.name
}

func (r *RelationField[T]) Rel() attrs.Relation {
	return &wrappedRelation{
		Relation: r.rel,
		from: &relationTarget{
			model: r.Instance(),
			field: r,
		},
	}
}

type Relation struct {
	Through attrs.Definer
	Object  attrs.Definer
	saved   bool
}

type MultipleRelation struct {
	rel       *orderedmap.OrderedMap[any, *Relation]
	removed   []any
	needsSave bool
}

func (m *MultipleRelation) setup() {
	if m.rel == nil {
		m.rel = orderedmap.NewOrderedMap[any, *Relation]()
	}
	if m.removed == nil {
		m.removed = make([]any, 0)
	}
}

func (m *MultipleRelation) Set(objs []Relation) error {

	if m.rel != nil {
		for head := m.rel.Front(); head != nil; head = head.Next() {
			m.removed = append(m.removed, head.Key)
		}
	}

	m.rel = orderedmap.NewOrderedMap[any, *Relation]()

	for _, obj := range objs {
		var (
			defs    = obj.Object.FieldDefs()
			pkField = defs.Primary()
			pkValue = pkField.GetValue()
		)

		m.rel.Set(pkValue, &obj)
	}

	m.needsSave = true

	return nil
}

func (m *MultipleRelation) Add(obj Relation) error {
	m.setup()

	var (
		defs    = obj.Object.FieldDefs()
		pkField = defs.Primary()
		pkValue = pkField.GetValue()
	)

	if _, ok := m.rel.Get(pkValue); ok {
		return errors.Errorf("object with primary key %v already exists", pkValue)
	}

	m.rel.Set(pkValue, &obj)

	m.needsSave = true

	return nil
}

func (m *MultipleRelation) All() []Relation {
	m.setup()

	var objs = make([]Relation, 0, m.rel.Len())
	for head := m.rel.Front(); head != nil; head = head.Next() {
		objs = append(objs, *head.Value)
	}

	return objs
}

func (m *MultipleRelation) Get(pkOrObj any) (Relation, bool) {
	m.setup()

	var pkValue any

	switch v := pkOrObj.(type) {
	case attrs.Definer:
		var defs = v.FieldDefs()
		var pkField = defs.Primary()
		pkValue = pkField.GetValue()
	case any:
		pkValue = v
	}

	if rel, ok := m.rel.Get(pkValue); ok {
		return *rel, true
	}

	return Relation{}, false
}

func (m *MultipleRelation) Remove(obj attrs.Definer) error {
	m.setup()

	var (
		defs    = obj.FieldDefs()
		pkField = defs.Primary()
		pkValue = pkField.GetValue()
	)

	if !m.rel.Delete(pkValue) {
		return errors.Errorf("object with primary key %v does not exist", pkValue)
	}

	m.needsSave = true

	return nil
}

func (m *MultipleRelation) Clear() error {
	if m.rel == nil {
		return nil
	}

	for head := m.rel.Front(); head != nil; head = head.Next() {
		m.removed = append(m.removed, head.Key)
	}

	m.needsSave = true
	return nil
}

type MultipleRelationField struct {
	*RelationField[[]attrs.Definer]
	dst *MultipleRelation
}

func NewMultipleRelatedField(forModel attrs.Definer, name string, reverseName string, columnName string, rel attrs.Relation) *MultipleRelationField {
	var f = &MultipleRelationField{
		RelationField: NewRelatedField[[]attrs.Definer](
			forModel,
			forModel,
			name,
			reverseName,
			columnName,
			rel,
		),
	}

	var dst = f.RelationField.GetValue()

	if dst == nil {
		f.dst = &MultipleRelation{}
	} else {
		switch v := dst.(type) {
		case *MultipleRelation:
			f.dst = v
		case []Relation:
			f.dst = &MultipleRelation{}
			f.dst.Set(v)
		default:
			panic(fmt.Errorf("invalid type %T for field %q", v, f.Name()))
		}
	}

	f.RelationField.SetValue(f.dst, true)

	return f
}

func (f *MultipleRelationField) Objects() *MultipleRelation {
	return f.dst
}

func (f *MultipleRelationField) SetValue(value any, force bool) error {
	switch v := value.(type) {
	case []Relation:
		return f.dst.Set(v)
	case Relation:
		return f.dst.Add(v)
	default:
		return fmt.Errorf("invalid type %T for field %q", v, f.Name())
	}
}

func (f *MultipleRelationField) GetValue() any {
	return f.dst.All()
}
