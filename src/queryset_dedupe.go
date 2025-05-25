package queries

import (
	"fmt"
	"reflect"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/elliotchance/orderedmap/v2"
)

type objectRelation struct {
	relTyp  attrs.RelationType
	objects *orderedmap.OrderedMap[any, *object]
}

type object struct {
	// the primary key of the object
	pk any

	// the field defs of the object
	fieldDefs attrs.Definitions

	// The object itself, which is a Definer
	obj attrs.Definer

	// the direct relations of the object (multiple)
	relations map[string]*objectRelation
}

type rootObject struct {
	object      *object
	annotations map[string]any // Annotations for the root object
}

type rows[T attrs.Definer] struct {
	objects *orderedmap.OrderedMap[any, *rootObject]
}

func newRows[T attrs.Definer]() *rows[T] {
	return &rows[T]{
		objects: orderedmap.NewOrderedMap[any, *rootObject](),
	}
}

func (r *rows[T]) addObject(chain []chainPart, annotations map[string]any) {

	var root = chain[0]
	var obj, ok = r.objects.Get(root.pk)
	if !ok {
		obj = &rootObject{
			object: &object{
				pk:        root.pk,
				fieldDefs: root.object.FieldDefs(),
				obj:       root.object,
				relations: make(map[string]*objectRelation),
			},
			annotations: annotations,
		}
		r.objects.Set(root.pk, obj)
	}

	var current = obj.object
	var idx = 1
	for idx < len(chain) {
		var part = chain[idx]
		var next, ok = current.relations[part.chain]
		if !ok {
			next = &objectRelation{
				relTyp:  part.relTyp,
				objects: orderedmap.NewOrderedMap[any, *object](),
			}
			current.relations[part.chain] = next
		}

		child, ok := next.objects.Get(part.pk)
		if !ok {
			child = &object{
				pk:        part.pk,
				fieldDefs: part.object.FieldDefs(),
				obj:       part.object,
				relations: make(map[string]*objectRelation),
			}
			next.objects.Set(part.pk, child)
		}

		current = child
		idx++
	}

	if idx != len(chain) {
		panic(fmt.Sprintf("chain length mismatch: expected %d, got %d", len(chain), idx))
	}
}

func (r *rows[T]) compile() []*Row[T] {
	var addRelations func(*object)
	addRelations = func(obj *object) {

		if obj.pk == nil {
			panic(fmt.Sprintf("object %T has no primary key, cannot deduplicate relations", obj.obj))
		}

		for relName, rel := range obj.relations {
			if rel.objects.Len() == 0 {
				continue
			}

			var relatedObjects = make([]attrs.Definer, 0, rel.objects.Len())
			for relHead := rel.objects.Front(); relHead != nil; relHead = relHead.Next() {
				var relatedObj = relHead.Value
				if relatedObj == nil {
					continue
				}

				addRelations(relatedObj)

				relatedObjects = append(relatedObjects, relatedObj.obj)
			}

			switch rel.relTyp {
			case attrs.RelOneToOne, attrs.RelManyToOne:
				if len(relatedObjects) > 1 {
					panic(fmt.Sprintf("expected at most one related object for %s, got %d", relName, len(relatedObjects)))
				}
				var relatedObject attrs.Definer
				if len(relatedObjects) > 0 {
					relatedObject = relatedObjects[0]
				}
				obj.fieldDefs.Set(relName, relatedObject)
			case attrs.RelOneToMany, attrs.RelManyToMany:
				var field, ok = obj.fieldDefs.Field(relName)
				if !ok {
					panic(fmt.Sprintf("relation %s not found in field defs of %T", relName, obj.obj))
				}

				if !ForSelectAll(field) {
					obj.obj.(DataModel).ModelDataStore().SetValue(relName, relatedObjects)
					continue
				}

				var typ = field.Type()
				switch typ.Kind() {
				case reflect.Slice:
					// If the field is a slice, we can set the related objects directly
					obj.fieldDefs.Set(relName, relatedObjects)
				default:
					panic(fmt.Sprintf("expected field %s to be a slice, got %s", relName, typ))
				}
			}
		}
	}

	var root = make([]*Row[T], 0, r.objects.Len())
	for head := r.objects.Front(); head != nil; head = head.Next() {
		var obj = head.Value
		if obj == nil {
			continue
		}

		addRelations(obj.object)

		var definer = obj.object.obj
		if definer == nil {
			continue
		}

		root = append(root, &Row[T]{
			Object:      definer.(T),
			Annotations: obj.annotations,
		})
	}
	return root
}

type chainPart struct {
	relTyp attrs.RelationType // The relation type of the field, if any
	chain  string
	pk     any
	object attrs.Definer
}

func buildChainParts(actualField *scannableField) []chainPart {
	// Get the stack of fields from target to parent
	var stack = make([]chainPart, 0)
	for cur := actualField; cur != nil; cur = cur.srcField {
		var (
			inst    = cur.field.Instance()
			defs    = inst.FieldDefs()
			primary = defs.Primary()
		)

		stack = append(stack, chainPart{
			relTyp: cur.relType,
			chain:  cur.chainPart,
			pk:     primary.GetValue(),
			object: inst,
		})
	}

	// Reverse the stack to get the fields in the correct order
	// i.e. parent to target
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}

	return stack
}
