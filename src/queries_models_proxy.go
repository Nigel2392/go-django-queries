package queries

import (
	"fmt"

	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-signals"
	"github.com/elliotchance/orderedmap/v2"
)

const _PROXY_FIELDS_KEY = "models.embed.proxy.fields"

// Build out the proxy field map when a model is registered.
//
// This will create a tree structure that contains all the proxy fields
// and their respective sub-proxy fields.
var _, _ = attrs.OnModelRegister.Listen(func(s signals.Signal[attrs.SignalModelMeta], meta attrs.SignalModelMeta) error {
	var newDefiner = attrs.NewObject[attrs.Definer](meta.Definer)
	var proxyFields = buildProxyFieldMap(newDefiner)
	attrs.StoreOnMeta(
		meta.Definer,
		_PROXY_FIELDS_KEY,
		proxyFields,
	)

	return nil
})

type proxyTree struct {
	object  attrs.Definer
	proxies *orderedmap.OrderedMap[string, *proxyFieldNode]
	fields  *orderedmap.OrderedMap[string, ProxyField]
}

type proxyFieldNode struct {
	sourceField ProxyField
	tree        *proxyTree
}

func (n *proxyTree) Object() attrs.Definer {
	return n.object
}

func (n *proxyTree) FieldsLen() int {
	return n.fields.Len()
}

func (n *proxyTree) hasProxies() bool {
	return n.proxies.Len() > 0 || n.fields.Len() > 0
}

func (n *proxyTree) Get(name string) (ProxyField, bool) {
	return n.fields.Get(name)
}

func buildProxyFieldMap(definer attrs.Definer) *proxyTree {
	if attrs.IsModelRegistered(definer) {
		var (
			meta     = attrs.GetModelMeta(definer)
			vals, ok = meta.Storage(_PROXY_FIELDS_KEY)
		)
		if ok {
			return vals.(*proxyTree)
		}
	}

	var node = &proxyTree{
		object:  definer,
		fields:  orderedmap.NewOrderedMap[string, ProxyField](),
		proxies: orderedmap.NewOrderedMap[string, *proxyFieldNode](),
	}
	var newDefiner = attrs.NewObject[attrs.Definer](definer)
	var defs = newDefiner.FieldDefs()
	for _, field := range defs.Fields() {
		var proxyField, ok = field.(ProxyField)
		if !ok || !proxyField.IsProxy() {
			continue
		}

		node.fields.Set(
			field.Name(),
			proxyField,
		)

		var rel = field.Rel()
		var relType = rel.Type()
		if relType == attrs.RelOneToOne || relType == attrs.RelManyToOne {
			var model = rel.Model()
			var subTree = buildProxyFieldMap(model)
			if !subTree.hasProxies() {
				continue
			}

			node.proxies.Set(
				field.Name(),
				&proxyFieldNode{
					tree:        subTree,
					sourceField: proxyField,
				},
			)

			continue
		}
	}

	return node
}

func ProxyFields(definer attrs.Definer) *proxyTree {
	if !attrs.IsModelRegistered(definer) {
		return &proxyTree{
			object:  definer,
			fields:  orderedmap.NewOrderedMap[string, ProxyField](),
			proxies: orderedmap.NewOrderedMap[string, *proxyFieldNode](),
		}
	}

	var (
		meta     = attrs.GetModelMeta(definer)
		vals, ok = meta.Storage(_PROXY_FIELDS_KEY)
	)

	if !ok {
		panic(fmt.Errorf("no proxy fields found for model %T", definer))
	}

	return vals.(*proxyTree)
}
