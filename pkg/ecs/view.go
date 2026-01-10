package ecs

import "reflect"

func newView(r *registry, components ...any) View {
	var v View
	for _, c := range components {
		t := reflect.TypeOf(c)
		id, ok := r.typeIDs[t]
		if !ok {
			panic("ecs: component " + t.String() + " must be registered before creating a View")
		}
		v.mask = v.mask.Set(id)
	}
	return v
}
