package grid

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
)

type GridLevelPlan struct {
	Key uint64
	BucketPlan
}

type FramePlan struct {
	ViewRect       spatial.AABB
	ViewChanged    bool
	GridLevels     []GridLevelPlan
	CompositeRects []spatial.AABB
}

type MultiBucketGridManager struct {
	mu                      sync.RWMutex
	space                   plane.Space2D[uint32]
	resoltuion              spatial.Resolution
	defaultBucketResolution spatial.Resolution
	defaultBucketCapacity   int
	marginBuckets           int
	managers                map[uint64]*BucketGridManager
}

func NewMultiBucketGridManager(
	space plane.Space2D[uint32],
	resoltuion spatial.Resolution,
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
		resoltuion:              resoltuion,
		defaultBucketResolution: defaultBucketResolution,
		defaultBucketCapacity:   defaultBucketCapacity,
		marginBuckets:           marginBuckets,
		managers:                make(map[uint64]*BucketGridManager),
	}
}

func (m *MultiBucketGridManager) Register(key uint64, cfg GridLevelConfig) (*BucketGridManager, error) {
	if cfg.Resoltuion == 0 {
		cfg.Resoltuion = m.resoltuion
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

func (m *MultiBucketGridManager) Manager(key uint64) *BucketGridManager {
	m.mu.RLock()
	manager := m.managers[key]
	m.mu.RUnlock()
	return manager
}

func (m *MultiBucketGridManager) MarginBuckets() int {
	return m.marginBuckets
}

func (m *MultiBucketGridManager) BuildFrame(viewRect spatial.AABB, viewChanged bool, keys []uint64) FramePlan {
	gridLevels := make([]GridLevelPlan, 0, len(keys))
	for _, key := range keys {
		manager := m.Manager(key)
		if manager == nil {
			continue
		}
		plan := manager.Plan(viewRect, m.marginBuckets)
		gridLevels = append(gridLevels, GridLevelPlan{Key: key, BucketPlan: plan})
	}

	composite := make([]spatial.AABB, 0, 16)
	if viewChanged {
		viewSize := rectSize(viewRect)
		if viewSize.X > 0 && viewSize.Y > 0 {
			composite = append(composite, geom.NewAABBAt(geom.NewVec[uint32](0, 0), viewSize.X, viewSize.Y))
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
		for _, gridLevel := range gridLevels {
			if gridLevel.BucketRect == nil {
				continue
			}
			for _, idx := range gridLevel.BucketIndices {
				bucket := gridLevel.BucketRect(idx)
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
		GridLevels:     gridLevels,
		CompositeRects: composite,
	}
}

func rectSize(rect spatial.AABB) geom.Vec[uint32] {
	return geom.NewVec(rect.BottomRight.X-rect.TopLeft.X, rect.BottomRight.Y-rect.TopLeft.Y)
}

func (m *MultiBucketGridManager) worldSideForView() uint32 {
	if m.space == nil {
		return 0
	}
	if m.space.Name() != "Toroidal2D" {
		return 0
	}
	return m.resoltuion.Side()
}

func intersectWithView(space plane.Space2D[uint32], bucket, viewRect spatial.AABB) (spatial.AABB, bool) {
	if space == nil {
		return geom.Intersection(bucket, viewRect)
	}
	wrapped := wrapViewRect(space, viewRect)
	for _, frag := range wrapped {
		if inter, ok := geom.Intersection(bucket, frag); ok {
			return inter, true
		}
	}
	return spatial.AABB{}, false
}

func wrapViewRect(space plane.Space2D[uint32], viewRect spatial.AABB) []spatial.AABB {
	wrapped := space.WrapAABB(viewRect)
	out := make([]spatial.AABB, 0, 4)
	out = append(out, wrapped.AABB)
	wrapped.VisitFragments(func(_ plane.FragPosition, frag geom.AABB[uint32]) bool {
		out = append(out, frag)
		return true
	})
	return out
}

func toViewRect(rect spatial.AABB, viewOrigin geom.Vec[uint32], worldSide uint32) spatial.AABB {
	x0 := mapCoord(rect.TopLeft.X, viewOrigin.X, worldSide)
	y0 := mapCoord(rect.TopLeft.Y, viewOrigin.Y, worldSide)
	x1 := mapCoord(rect.BottomRight.X, viewOrigin.X, worldSide)
	y1 := mapCoord(rect.BottomRight.Y, viewOrigin.Y, worldSide)
	return geom.NewAABB(geom.NewVec(x0, y0), geom.NewVec(x1, y1))
}

func mapCoord(value, origin, worldSide uint32) uint32 {
	if worldSide == 0 {
		if value >= origin {
			return value - origin
		}
		return 0
	}
	if value >= origin {
		return value - origin
	}
	return value + worldSide - origin
}
