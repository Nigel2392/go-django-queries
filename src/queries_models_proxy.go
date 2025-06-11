package queries

import (
	"github.com/Nigel2392/go-django/src/core/attrs"
	"github.com/Nigel2392/go-signals"
	"github.com/elliotchance/orderedmap/v2"
)

const _PROXY_FIELDS_KEY = "models.embed.proxy.fields"

type proxyFieldMap struct {
	object attrs.Definer
	field  ProxyField
	next   *proxyFieldMap
	fields *orderedmap.OrderedMap[string, proxyFieldMap]
}

type ProxyFieldMap = orderedmap.OrderedMap[string, ProxyField]

var _, _ = attrs.OnModelRegister.Listen(func(s signals.Signal[attrs.SignalModelMeta], meta attrs.SignalModelMeta) error {

	var newDefiner = attrs.NewObject[attrs.Definer](meta.Definer)
	var defs = newDefiner.FieldDefs()
	var fields = defs.Fields()
	var proxyFields = orderedmap.NewOrderedMap[string, ProxyField]()
	for _, field := range fields {
		var proxyField, ok = field.(ProxyField)
		if !ok || !proxyField.IsProxy() {
			continue
		}

		proxyFields.Set(
			field.Name(),
			proxyField,
		)
	}

	if proxyFields.Len() > 0 {
		attrs.StoreOnMeta(
			meta.Definer,
			_PROXY_FIELDS_KEY,
			(*ProxyFieldMap)(proxyFields),
		)
	}

	return nil
})

func ProxyFields(definer attrs.Definer) *ProxyFieldMap {
	var (
		meta     = attrs.GetModelMeta(definer)
		vals, ok = meta.Storage(_PROXY_FIELDS_KEY)
	)

	if !ok {
		return (*ProxyFieldMap)(orderedmap.NewOrderedMap[string, ProxyField]())
	}

	return vals.(*ProxyFieldMap)
}
