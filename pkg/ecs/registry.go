package ecs

import (
	"reflect"
)

type registry struct {
	lastEntity Entity
	freeList   []Entity
	masks      map[Entity]Bitmask
	storages   map[ComponentID]any
	typeIDs    map[reflect.Type]ComponentID
	deleters   map[ComponentID]func(Entity)
}

func newRegistry() *registry {
	return &registry{
		masks:    make(map[Entity]Bitmask),
		storages: make(map[ComponentID]any),
		typeIDs:  make(map[reflect.Type]ComponentID),
		deleters: make(map[ComponentID]func(Entity)),
	}
}

func (r *registry) createEntity() Entity {
	var e Entity
	if len(r.freeList) > 0 {
		e = r.freeList[len(r.freeList)-1]
		r.freeList = r.freeList[:len(r.freeList)-1]
	} else {
		r.lastEntity++
		e = r.lastEntity
	}
	r.masks[e] = Bitmask{}
	return e
}

func (r *registry) removeEntity(e Entity) {
	mask, ok := r.masks[e]
	if !ok {
		return
	}

	mask.ForEachSet(func(id ComponentID) {
		if deleteFn, exists := r.deleters[id]; exists {
			deleteFn(e)
		}
	})

	delete(r.masks, e)
	r.freeList = append(r.freeList, e)
}

func assign[T any](r *registry, e Entity, component T) {
	id := registerComponent[T](r)
	assignByID(r, e, id, component)
}

func assignByID[T any](r *registry, e Entity, id ComponentID, component T) {
	r.masks[e] = r.masks[e].Set(id)
	storage := r.storages[id].(map[Entity]*T)
	c := component
	storage[e] = &c
}

func unassign[T any](r *registry, e Entity) {
	var dummy T
	t := reflect.TypeOf(dummy)

	id, ok := r.typeIDs[t]
	if !ok {
		return
	}

	unassignByID[T](r, e, id)
}

func unassignByID[T any](r *registry, e Entity, id ComponentID) {
	if storage, ok := r.storages[id].(map[Entity]*T); ok {
		delete(storage, e)
	}

	if mask, ok := r.masks[e]; ok {
		r.masks[e] = mask.Clear(id)
	}
}

func (r *registry) eachEntitiesMathesView(v View, fn func(e Entity)) {
	for e, m := range r.masks {
		if m.Matches(v.mask) {
			fn(e)
		}
	}
}

func mapTypeToComponent[T any](r *registry) map[Entity]*T {
	id := registerComponent[T](r)
	return r.storages[id].(map[Entity]*T)
}

func registerComponent[T any](r *registry) ComponentID {
	var dummy T
	t := reflect.TypeOf(dummy)
	if id, ok := r.typeIDs[t]; ok {
		return id
	}

	id := ComponentID(len(r.typeIDs))
	r.typeIDs[t] = id

	storage := make(map[Entity]*T)
	r.storages[id] = storage

	r.deleters[id] = func(e Entity) {
		delete(storage, e)
	}

	return id
}
