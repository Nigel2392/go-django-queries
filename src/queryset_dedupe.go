package queries

import (
	"fmt"
	"strings"

	"github.com/Nigel2392/go-django/src/core/attrs"
)

type dedupeNode struct {
	children map[string]map[any]*dedupeNode // chain name -> PK -> next node
	objects  map[any]attrs.Definer          // Only for leaves: PKs we've already seen at this level
}

func printDedupe(sb *strings.Builder, dedupe *dedupeNode, depth int) {
	if dedupe.children != nil {
		for path, childMap := range dedupe.children {
			for k, dedupe := range childMap {
				sb.WriteString(strings.Repeat("\t", depth+1))
				sb.WriteString(fmt.Sprintf("'%v' (%s): %v\n", k, path, dedupe))
				printDedupe(sb, dedupe, depth+1)
			}
		}
	}
}

func newDedupeNode() *dedupeNode {
	return &dedupeNode{
		children: make(map[string]map[any]*dedupeNode),
		objects:  make(map[any]attrs.Definer),
	}
}

type chainPart struct {
	chain  string
	pk     any
	object attrs.Definer
}

func (n *dedupeNode) Has(keyParts []chainPart) bool {
	return n.has(keyParts, 0)
}

func (n *dedupeNode) Add(keyParts []chainPart) {
	n.add(keyParts, 0)
}

func (n *dedupeNode) has(keyParts []chainPart, partsIdx int) bool {
	part := keyParts[partsIdx]
	if partsIdx == len(keyParts)-1 {
		_, ok := n.objects[part.pk]
		return ok
	}
	nextMap, ok := n.children[part.chain]
	if !ok {
		return false
	}
	child, ok := nextMap[part.pk]
	if !ok {
		return false
	}
	return child.has(keyParts, partsIdx+1)
}

func (n *dedupeNode) add(keyParts []chainPart, partsIdx int) {
	part := keyParts[partsIdx]
	if partsIdx == len(keyParts)-1 {
		n.objects[part.pk] = part.object
		return
	}
	nextMap, ok := n.children[part.chain]
	if !ok {
		nextMap = make(map[any]*dedupeNode)
		n.children[part.chain] = nextMap
	}
	child, ok := nextMap[part.pk]
	if !ok {
		child = newDedupeNode()
		nextMap[part.pk] = child

	}
	child.add(keyParts, partsIdx+1)
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
