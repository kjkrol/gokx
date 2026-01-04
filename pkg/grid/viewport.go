package grid

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
)

type Viewport struct {
	mu      sync.RWMutex
	origin  geom.Vec[int]
	size    geom.Vec[int]
	world   geom.Vec[int]
	wrap    bool
	version uint64
}

func NewViewport(worldSize, viewSize geom.Vec[int], wrap bool) *Viewport {
	v := &Viewport{
		size:  viewSize,
		world: worldSize,
		wrap:  wrap,
	}
	v.setOriginLocked(geom.NewVec(0, 0))
	return v
}

func (v *Viewport) Rect() geom.AABB[int] {
	v.mu.RLock()
	origin := v.origin
	size := v.size
	v.mu.RUnlock()
	return geom.NewAABBAt(origin, size.X, size.Y)
}

func (v *Viewport) Origin() geom.Vec[int] {
	v.mu.RLock()
	origin := v.origin
	v.mu.RUnlock()
	return origin
}

func (v *Viewport) Size() geom.Vec[int] {
	v.mu.RLock()
	size := v.size
	v.mu.RUnlock()
	return size
}

func (v *Viewport) WorldSize() geom.Vec[int] {
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

func (v *Viewport) SetOrigin(x, y int) {
	v.mu.Lock()
	v.setOriginLocked(geom.NewVec(x, y))
	v.mu.Unlock()
}

func (v *Viewport) Move(dx, dy int) {
	v.mu.Lock()
	v.setOriginLocked(v.origin.Add(geom.NewVec(dx, dy)))
	v.mu.Unlock()
}

func (v *Viewport) setOriginLocked(origin geom.Vec[int]) {
	normalized := v.normalize(origin)
	if normalized == v.origin {
		return
	}
	v.origin = normalized
	v.version++
}

func (v *Viewport) normalize(origin geom.Vec[int]) geom.Vec[int] {
	if v.wrap && v.world.X > 0 && v.world.Y > 0 {
		math := geom.VectorMathByType[int]()
		return math.Wrap(origin, v.world)
	}
	maxX := v.world.X - v.size.X
	maxY := v.world.Y - v.size.Y
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	return geom.NewVec(clamp(origin.X, 0, maxX), clamp(origin.Y, 0, maxY))
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
