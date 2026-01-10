package ecs

import (
	"time"
)

var _ SystemAPI = (*scheduler)(nil)

type scheduler struct {
	register *registry
	systems  []System
}

func newScheduler(register *registry) *scheduler {
	return &scheduler{
		register: register,
		systems:  make([]System, 0),
	}
}

func (e *scheduler) NewView(components ...any) View {
	return newView(e.register, components...)
}

func (e *scheduler) Each(v View, fn func(e Entity)) {
	e.register.eachEntitiesMathesView(v, fn)
}

func (e *scheduler) registerSystems(systems []System) {
	for _, system := range systems {
		system.Init(e)
		e.systems = append(e.systems, system)
	}
}

func (e *scheduler) updateSystems(duration time.Duration) {
	for _, system := range e.systems {
		system.Update(e, duration)
	}
}

func (e *scheduler) registry() *registry { return e.register }
