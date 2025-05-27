package fields

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
)

type ForeignKeyField[T any] struct {
	*RelationField[T]
}

func NewForeignKeyField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ForeignKeyField[T] {
	var f = &ForeignKeyField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelManyToOne,
			},
		),
	}
	return f
}

type ForeignKeyReverseField[T any] struct {
	*RelationField[T]
}

func NewForeignKeyReverseField[T any](forModel attrs.Definer, dst any, name string, reverseName string, columnName string, rel attrs.Relation) *ForeignKeyReverseField[T] {
	var f = &ForeignKeyReverseField[T]{
		RelationField: NewRelatedField[T](
			forModel,
			dst,
			name,
			reverseName,
			columnName,
			&typedRelation{
				Relation: rel,
				typ:      attrs.RelOneToMany,
			},
		),
	}
	return f
}

//
//type foreignKeyRelation[T attrs.Definer] struct {
//}
//
//type ForeignKeyField[T attrs.Definer] struct {
//	*DataModelField[any]   // The field that defines the foreign key relationship
//	Target               T // The target model of the foreign key relationship
//}
//
//func NewForeignKeyField[T attrs.Definer](forModel attrs.Definer, dst any, name string, target T) *ForeignKeyField[T] {
//	if forModel == nil || dst == nil {
//		panic("NewForeignKeyField: model is nil")
//	}
//
//	if name == "" {
//		panic("NewForeignKeyField: name is empty")
//	}
//
//	return &ForeignKeyField[T]{
//		DataModelField: NewDataModelField[T](forModel, dst, name),
//		Target:         target,
//	}
//}
//
//func (f *ForeignKeyField[T]) SetValue(value any, _ bool) error {
//	if value == nil {
//		return nil
//	}
//
//	if v, ok := value.(attrs.Definer); ok {
//		return f.DataModelField.SetValue(v, false)
//	}
//
//	var relDefs = f.Target.FieldDefs()
//}
//
//func (f *ForeignKeyField[T]) Inject(ctx InjectContext) error {
//	if ctx.QuerySet == nil {
//		return errors.New("Inject: QuerySet is nil")
//	}
//	if ctx.QuerySet.joinM[ctx.ToAlias] {
//		return nil // already joined
//	}
//	ctx.QuerySet.joinM[ctx.ToAlias] = true
//
//	defs := f.Instance().FieldDefs()
//	field := defs.Primary()
//	relDefs := f.Rel().FieldDefs()
//
//	ctx.QuerySet.joins = append(ctx.QuerySet.joins, queries.JoinDef{
//		FromAlias: ctx.FromAlias,
//		ToAlias:   ctx.ToAlias,
//		FromField: f.ColumnName(),
//		ToField:   relDefs.Primary().ColumnName(),
//		JoinType:  "LEFT JOIN",
//		Table:     relDefs.TableName(),
//	})
//
//	if ctx.SelectAll {
//		ctx.QuerySet.fields = append(ctx.QuerySet.fields, queries.FieldInfo{
//			Model:  f.Rel(),
//			Table:  Table{Name: relDefs.TableName(), Alias: ctx.ToAlias},
//			Fields: relDefs.Fields(),
//			Chain:  ctx.Chain,
//		})
//	}
//	return nil
//}
//
