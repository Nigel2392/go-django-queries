package expr

import (
	"fmt"
	"reflect"
	"strings"
)

type lookupRegistry struct {
	lookupsLocal  map[reflect.Type]map[string]Lookup
	lookupsGlobal map[string]Lookup
}

func (r *lookupRegistry) Register(lookup Lookup) {
	if lookup == nil {
		panic("lookup cannot be nil")
	}

	var name = lookup.Name()
	if name == "" {
		panic("lookup name cannot be empty")
	}

	if r.lookupsLocal == nil {
		r.lookupsLocal = make(map[reflect.Type]map[string]Lookup)
	}
	if r.lookupsGlobal == nil {
		r.lookupsGlobal = make(map[string]Lookup)
	}

	var drivers = lookup.Drivers()
	if len(drivers) == 0 {
		// register globally
		r.lookupsGlobal[name] = lookup
		return
	}

	for _, drv := range drivers {
		var t = reflect.TypeOf(drv)
		if _, ok := r.lookupsLocal[t]; !ok {
			r.lookupsLocal[t] = make(map[string]Lookup)
		}
		r.lookupsLocal[t][name] = lookup
	}
}

func (r *lookupRegistry) Lookup(inf *ExpressionInfo, lookupName string, lhs any, args []any) (func(sb *strings.Builder) []any, error) {
	var t = reflect.TypeOf(inf.Driver)
	var localLookups, ok = r.lookupsLocal[t]
	if !ok {
		localLookups = r.lookupsGlobal
	}

	var lookup Lookup
	if lkup, ok := localLookups[lookupName]; ok {
		lookup = lkup
	} else if lkup, ok := r.lookupsGlobal[lookupName]; ok {
		lookup = lkup
	}

	if lookup == nil {
		return nil, fmt.Errorf(
			"no lookup %q found for driver %T: %w",
			lookupName, inf.Driver, ErrLookupNotFound,
		)
	}

	var min, max = lookup.Arity()
	if len(args) < min || (max >= 0 && len(args) > max) {
		return nil, fmt.Errorf(
			"lookup %s requires between %d and %d arguments, got %d: %w",
			lookup.Name(), min, max, len(args), ErrLookupArgsInvalid,
		)
	}

	var normalizedArgs, err = lookup.NormalizeArgs(inf, args)
	if err != nil {
		return nil, fmt.Errorf(
			"error normalizing args for lookup %s: %w",
			lookup.Name(), err,
		)
	}

	var lhsExpr Expression
	switch lhs := lhs.(type) {
	case string:
		lhsExpr = String(lhs).Resolve(inf)
	case Expression:
		lhsExpr = lhs.Resolve(inf)
	default:
		return nil, fmt.Errorf("unsupported type for lhs: %T", lhs)
	}

	var expr = lookup.Resolve(inf, lhsExpr, normalizedArgs)
	if expr == nil {
		return nil, fmt.Errorf("lookup %s returned nil expression", lookup.Name())
	}

	return expr, nil
}
