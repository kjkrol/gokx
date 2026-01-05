package gfx

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
)

type Viewport struct {
	mu      sync.RWMutex
	origin  geom.Vec[uint32]
	size    geom.Vec[uint32]
	world   geom.Vec[uint32]
	wrap    bool
	version uint64
}

func NewViewport(worldSize, viewSize geom.Vec[uint32], wrap bool) *Viewport {
	v := &Viewport{
		size:  viewSize,
		world: worldSize,
		wrap:  wrap,
	}
	v.setOriginLocked(geom.NewVec[uint32](0, 0))
	return v
}

func (v *Viewport) Rect() geom.AABB[uint32] {
	v.mu.RLock()
	origin := v.origin
	size := v.size
	v.mu.RUnlock()
	return geom.NewAABBAt(origin, size.X, size.Y)
}

func (v *Viewport) Origin() geom.Vec[uint32] {
	v.mu.RLock()
	origin := v.origin
	v.mu.RUnlock()
	return origin
}

func (v *Viewport) Size() geom.Vec[uint32] {
	v.mu.RLock()
	size := v.size
	v.mu.RUnlock()
	return size
}

func (v *Viewport) WorldSize() geom.Vec[uint32] {
	v.mu.RLock()
	world := v.world
	v.mu.RUnlock()
	return world
}

func (v *Viewport) Wrap() bool {
	v.mu.RLock()
	wrap := v.wrap
	v.mu.RUnlock()
	return wrap
}

func (v *Viewport) Version() uint64 {
	v.mu.RLock()
	version := v.version
	v.mu.RUnlock()
	return version
}

func (v *Viewport) SetOrigin(x, y uint32) {
	v.mu.Lock()
	v.setOriginLocked(geom.NewVec(x, y))
	v.mu.Unlock()
}

func (v *Viewport) Move(dx, dy int32) {
	v.mu.Lock()
	v.setOriginLocked(v.origin.Add(geom.NewVec[uint32](uint32(dx), uint32(dy))))
	v.mu.Unlock()
}

func (v *Viewport) setOriginLocked(origin geom.Vec[uint32]) {
	normalized := v.normalize(origin)
	if normalized == v.origin {
		return
	}
	v.origin = normalized
	v.version++
}

func (v *Viewport) normalize(origin geom.Vec[uint32]) geom.Vec[uint32] {
	if v.wrap && v.world.X > 0 && v.world.Y > 0 {
		math := geom.VectorMathByType[uint32]()
		return math.Wrap(origin, v.world)
	}
	maxX := int64(0)
	maxY := int64(0)
	if v.world.X > v.size.X {
		maxX = int64(v.world.X - v.size.X)
	}
	if v.world.Y > v.size.Y {
		maxY = int64(v.world.Y - v.size.Y)
	}
	x := int64(int32(origin.X))
	y := int64(int32(origin.Y))
	if x < 0 {
		x = 0
	} else if x > maxX {
		x = maxX
	}
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}
	return geom.NewVec(uint32(x), uint32(y))
}
