package fields

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type FieldConfig struct {
	Dst         any
	ForModel    attrs.Definer
	ReverseName string
	ColumnName  string
	RelType     attrs.RelationType
	Rel         attrs.Relation
	TargetField string
	Through     attrs.Through
}

type unbound[T attrs.Field] struct {
	name   string
	config *FieldConfig
	field  func(attrs.Definer, any, string, string, string, attrs.Relation) T
}

// Name returns the name of the field.
func (u *unbound[T]) Name() string {
	return u.name
}

// BindField binds the field to the model.
func (u *unbound[T]) BindField(model attrs.Definer) (attrs.Field, error) {
	if u.name == "" {
		panic(fmt.Sprintf("field name cannot be empty for %T", model))
	}

	var fieldConfig = &FieldConfig{
		Dst:         u.config.Dst,
		ForModel:    model,
		ReverseName: u.config.ReverseName,
		ColumnName:  u.config.ColumnName,
		Rel:         u.config.Rel,
	}

	if fieldConfig.ForModel == nil {
		return nil, fmt.Errorf("model cannot be nil for field %s", u.name)
	}

	if fieldConfig.Dst == nil {
		var (
			rVal = reflect.ValueOf(model)
			rTyp = reflect.TypeOf(model)
		)

		if rVal.Kind() != reflect.Ptr || rTyp.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("model must be a pointer to a struct, got %T", model)
		}

		rTyp = rTyp.Elem()
		rVal = rVal.Elem()

		var field = rVal.FieldByName(u.name)
		if !field.IsValid() {
			return nil, fmt.Errorf("field %s not found in model %s", u.name, rTyp.Name())
		}

		fieldConfig.Dst = field.Addr().Interface()
	}

	if fieldConfig.Rel == nil {
		return nil, fmt.Errorf("relation cannot be nil for field %s in model %T", u.name, model)
	}

	if fieldConfig.ReverseName == "" {
		switch fieldConfig.Rel.Type() {
		case attrs.RelOneToOne:
			fieldConfig.ReverseName = fmt.Sprintf("%sReverse", u.name)
		case attrs.RelManyToOne:
			fieldConfig.ReverseName = fmt.Sprintf("%sReverse", u.name)
		case attrs.RelManyToMany:
			fieldConfig.ReverseName = fmt.Sprintf("%sSet", u.name)
		case attrs.RelOneToMany:
			fieldConfig.ReverseName = fmt.Sprintf("%sSet", u.name)
		default:
			return nil, fmt.Errorf("unsupported relation type %s for field %s in model %T", fieldConfig.Rel.Type(), u.name, model)
		}
	}

	var field = u.field(
		fieldConfig.ForModel,
		fieldConfig.Dst,
		u.name,
		fieldConfig.ReverseName,
		fieldConfig.ColumnName,
		fieldConfig.Rel,
	)

	return field, nil
}

func fieldConstructor[FieldT attrs.Field, T any](name string, relTyp attrs.RelationType, fieldFunc func(attrs.Definer, any, string, string, string, attrs.Relation) FieldT, conf ...*FieldConfig) attrs.UnboundFieldConstructor {
	var cnf = &FieldConfig{}
	if len(conf) > 0 {
		cnf = conf[0]
	}

	if cnf.Rel != nil {
		cnf.Rel = &typedRelation{
			Relation: cnf.Rel,
			typ:      relTyp,
		}
	} else {
		var rV = reflect.New(reflect.TypeOf(new(T)).Elem().Elem())
		cnf.Rel = &typedRelation{
			Relation: attrs.Relate(
				rV.Interface().(attrs.Definer),
				cnf.TargetField,
				cnf.Through,
			),
			typ: relTyp,
		}
	}

	return &unbound[FieldT]{
		name:   name,
		config: cnf,
		field:  fieldFunc,
	}
}

func OneToOne[T any](name string, conf ...*FieldConfig) attrs.UnboundFieldConstructor {
	return fieldConstructor[*OneToOneField[T], T](
		name, attrs.RelOneToOne, NewOneToOneField[T], conf...,
	)
}

func ForeignKey[T any](name string, conf ...*FieldConfig) attrs.UnboundFieldConstructor {
	return fieldConstructor[*ForeignKeyField[T], T](
		name, attrs.RelManyToOne, NewForeignKeyField[T], conf...,
	)
}

func ManyToMany[T any](name string, conf ...*FieldConfig) attrs.UnboundFieldConstructor {
	if len(conf) == 0 {
		panic("ManyToMany requires at least one FieldConfig with a Through relation defined")
	}

	if conf[0].Rel == nil || (conf[0].Through == nil && conf[0].Rel.Through() == nil) {
		panic("ManyToMany requires a Through relation defined in the FieldConfig")
	}

	return fieldConstructor[*ManyToManyField[T], T](
		name, attrs.RelManyToMany, NewManyToManyField[T], conf...,
	)
}
