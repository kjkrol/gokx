package grid

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
)

type LayerPlan struct {
	Key any
	BucketPlan
}

type FramePlan struct {
	ViewRect       geom.AABB[int]
	ViewChanged    bool
	Layers         []LayerPlan
	CompositeRects []geom.AABB[int]
}

type MultiBucketGridManager struct {
	mu                      sync.RWMutex
	space                   plane.Space2D[int]
	worldResolution         spatial.Resolution
	defaultBucketResolution spatial.Resolution
	defaultBucketCapacity   int
	marginBuckets           int
	managers                map[any]*BucketGridManager
}

func NewMultiBucketGridManager(
	space plane.Space2D[int],
	worldResolution spatial.Resolution,
	marginBuckets int,
	defaultBucketResolution spatial.Resolution,
	defaultBucketCapacity int,
) *MultiBucketGridManager {
	if marginBuckets <= 0 {
		marginBuckets = 2
	}
	if defaultBucketResolution == 0 {
		defaultBucketResolution = spatial.NewResolution(6)
	}
	if defaultBucketCapacity <= 0 {
		defaultBucketCapacity = 16
	}
	return &MultiBucketGridManager{
		space:                   space,
		worldResolution:         worldResolution,
		defaultBucketResolution: defaultBucketResolution,
		defaultBucketCapacity:   defaultBucketCapacity,
		marginBuckets:           marginBuckets,
		managers:                make(map[any]*BucketGridManager),
	}
}

func (m *MultiBucketGridManager) Register(key any, cfg LayerConfig) (*BucketGridManager, error) {
	if cfg.WorldResolution == 0 {
		cfg.WorldResolution = m.worldResolution
	}
	if cfg.BucketResolution == 0 {
		cfg.BucketResolution = m.defaultBucketResolution
	}
	if cfg.BucketCapacity == 0 {
		cfg.BucketCapacity = m.defaultBucketCapacity
	}
	manager, err := NewBucketGridManager(m.space, cfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.managers[key] = manager
	m.mu.Unlock()
	return manager, nil
}

func (m *MultiBucketGridManager) Manager(key any) *BucketGridManager {
	m.mu.RLock()
	manager := m.managers[key]
	m.mu.RUnlock()
	return manager
}

func (m *MultiBucketGridManager) MarginBuckets() int {
	return m.marginBuckets
}

func (m *MultiBucketGridManager) BuildFrame(viewRect geom.AABB[int], viewChanged bool, keys []any) FramePlan {
	layers := make([]LayerPlan, 0, len(keys))
	for _, key := range keys {
		manager := m.Manager(key)
		if manager == nil {
			continue
		}
		plan := manager.Plan(viewRect, m.marginBuckets)
		layers = append(layers, LayerPlan{Key: key, BucketPlan: plan})
	}

	composite := make([]geom.AABB[int], 0, 16)
	if viewChanged {
		viewSize := rectSize(viewRect)
		if viewSize.X > 0 && viewSize.Y > 0 {
			composite = append(composite, geom.NewAABBAt(geom.NewVec(0, 0), viewSize.X, viewSize.Y))
		}
	} else {
		viewOrigin := viewRect.TopLeft
		worldSide := m.worldSideForView()
		if worldSide > 0 {
			viewSize := rectSize(viewRect)
			if viewSize.X >= worldSide && viewSize.Y >= worldSide {
				worldSide = 0
			}
		}
		for _, layer := range layers {
			for _, bucket := range layer.Buckets {
				clipped, ok := intersectWithView(m.space, bucket, viewRect)
				if !ok {
					continue
				}
				viewRectLocal := toViewRect(clipped, viewOrigin, worldSide)
				if rectEmpty(viewRectLocal) {
					continue
				}
				composite = append(composite, viewRectLocal)
			}
		}
	}

	return FramePlan{
		ViewRect:       viewRect,
		ViewChanged:    viewChanged,
		Layers:         layers,
		CompositeRects: composite,
	}
}

func rectSize(rect geom.AABB[int]) geom.Vec[int] {
	return geom.NewVec(rect.BottomRight.X-rect.TopLeft.X, rect.BottomRight.Y-rect.TopLeft.Y)
}

func (m *MultiBucketGridManager) worldSideForView() int {
	if m.space == nil {
		return 0
	}
	if m.space.Name() != "Toroidal2D" {
		return 0
	}
	return int(m.worldResolution.Side())
}

func intersectWithView(space plane.Space2D[int], bucket, viewRect geom.AABB[int]) (geom.AABB[int], bool) {
	if space == nil {
		return rectIntersect(bucket, viewRect)
	}
	wrapped := wrapViewRect(space, viewRect)
	for _, frag := range wrapped {
		if inter, ok := rectIntersect(bucket, frag); ok {
			return inter, true
		}
	}
	return geom.AABB[int]{}, false
}

func wrapViewRect(space plane.Space2D[int], viewRect geom.AABB[int]) []geom.AABB[int] {
	wrapped := space.WrapAABB(viewRect)
	out := make([]geom.AABB[int], 0, 4)
	out = append(out, wrapped.AABB)
	wrapped.VisitFragments(func(_ plane.FragPosition, frag geom.AABB[int]) bool {
		out = append(out, frag)
		return true
	})
	return out
}

func toViewRect(rect geom.AABB[int], viewOrigin geom.Vec[int], worldSide int) geom.AABB[int] {
	x0 := mapCoord(rect.TopLeft.X, viewOrigin.X, worldSide)
	y0 := mapCoord(rect.TopLeft.Y, viewOrigin.Y, worldSide)
	x1 := mapCoord(rect.BottomRight.X, viewOrigin.X, worldSide)
	y1 := mapCoord(rect.BottomRight.Y, viewOrigin.Y, worldSide)
	return geom.NewAABB(geom.NewVec(x0, y0), geom.NewVec(x1, y1))
}

func mapCoord(value, origin, worldSide int) int {
	if worldSide <= 0 {
		return value - origin
	}
	delta := value - origin
	if delta < 0 {
		delta += worldSide
	}
	return delta
}
