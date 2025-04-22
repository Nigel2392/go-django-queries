package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

func (qs *QuerySet[T]) addJoin(typ, field, table, conditionA, logic, conditionB string) *QuerySet[T] {
	var (
		tableName string = table
		related   attrs.Definer
		f         attrs.Field
	)

	if table == "" && field == "" {
		panic("table and field cannot be empty")
	}

	if field != "" {
		var ok bool
		f, ok = qs.queryInfo.fields_map[field]
		if !ok {
			panic(fmt.Errorf("field %q not found in model %T", field, qs.model))
		}

		related = f.Rel()
	}

	if related != nil {
		var defs = related.FieldDefs()
		tableName = defs.TableName()
	}

	qs.joins = append(qs.joins, joinDef{
		typeJoin: typ,
		info: fieldInfo{
			dstTable: tableName,
			model:    related,
		},
		conditionA: conditionA,
		logic:      logic,
		conditionB: conditionB,
	})
	return qs
}

func (qs *QuerySet[T]) Join(field, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("JOIN", field, "", conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) InnerJoin(field, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("INNER JOIN", field, "", conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) LeftJoin(field, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("LEFT JOIN", field, "", conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) RightJoin(field, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("RIGHT JOIN", field, "", conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) CrossJoin(field, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("CROSS JOIN", field, "", conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) JoinTable(table, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("JOIN", "", table, conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) InnerJoinTable(table, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("INNER JOIN", "", table, conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) LeftJoinTable(table, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("LEFT JOIN", "", table, conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) RightJoinTable(table, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("RIGHT JOIN", "", table, conditionA, logic, conditionB)
}

func (qs *QuerySet[T]) CrossJoinTable(table, conditionA, logic, conditionB string) *QuerySet[T] {
	return qs.addJoin("CROSS JOIN", "", table, conditionA, logic, conditionB)
}
