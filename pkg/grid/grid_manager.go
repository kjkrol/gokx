package grid

import (
	"fmt"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
)

type GridLevelConfig struct {
	Resoltuion       spatial.Resolution
	BucketResolution spatial.Resolution
	BucketCapacity   int
}

type BucketPlan struct {
	CacheRect spatial.AABB
	Buckets   []spatial.AABB
}

type BucketDelta = spatial.BucketDelta

type BucketGridManager struct {
	index          *spatial.GridIndexManager
	cacheWorldSide uint32
	dirty          dirtyState
}

type dirtyState struct {
	bucketResolution spatial.Resolution
	bucketSize       uint32
	gridSide         uint32
	dirty            map[uint32]struct{}
	dirtyList        []uint32
	cacheRect        spatial.AABB
	cacheValid       bool
}

func NewBucketGridManager(space plane.Space2D[uint32], cfg GridLevelConfig) (*BucketGridManager, error) {
	if space == nil {
		return nil, fmt.Errorf("space is required")
	}
	index, err := spatial.NewGridIndexManager(space, spatial.GridIndexConfig{
		WorldResolution:  cfg.Resoltuion,
		BucketResolution: cfg.BucketResolution,
		BucketCapacity:   cfg.BucketCapacity,
	})
	if err != nil {
		return nil, err
	}
	manager := &BucketGridManager{index: index}
	if space.Name() == "Toroidal2D" {
		manager.cacheWorldSide = cfg.Resoltuion.Side()
	}
	bucketResolution := cfg.BucketResolution
	if bucketResolution == 0 {
		bucketResolution = spatial.NewResolution(6)
	}
	bucketSize := bucketResolution.Side()
	gridSide := cfg.Resoltuion.Side() / bucketSize
	manager.dirty = dirtyState{
		bucketResolution: bucketResolution,
		bucketSize:       bucketSize,
		gridSide:         gridSide,
		dirty:            make(map[uint32]struct{}),
	}
	return manager, nil
}

func (m *BucketGridManager) ConsumeBucketDeltas() []BucketDelta {
	if m.index == nil {
		return nil
	}
	return m.index.ConsumeBucketDeltas()
}

func (m *BucketGridManager) QueueInsert(id uint64, aabb plane.AABB[uint32]) {
	if m.index == nil {
		return
	}
	m.index.QueueInsert(id, planeAABBToSpatial(aabb))
}

func (m *BucketGridManager) QueueRemove(id uint64) {
	if m.index == nil {
		return
	}
	m.index.QueueRemove(id)
}

func (m *BucketGridManager) QueueUpdate(id uint64, aabb plane.AABB[uint32], markDirty bool) {
	if m.index == nil {
		return
	}
	m.index.QueueUpdate(id, planeAABBToSpatial(aabb), markDirty)
}

func (m *BucketGridManager) QueueDirtyRect(rect spatial.AABB) {
	m.MarkRectDirty(rect)
}

func (m *BucketGridManager) Plan(viewRect spatial.AABB, marginBuckets int) BucketPlan {
	m.Flush()
	worldSide := m.cacheWorldSide
	if worldSide > 0 {
		viewW := viewRect.BottomRight.X - viewRect.TopLeft.X
		viewH := viewRect.BottomRight.Y - viewRect.TopLeft.Y
		if viewW >= worldSide && viewH >= worldSide {
			worldSide = 0
		}
	}
	cacheRect := cacheRectForView(viewRect, m.dirty.bucketSize, marginBuckets, worldSide)
	if !m.dirty.cacheValid || !rectEquals(cacheRect, m.dirty.cacheRect) {
		if m.dirty.cacheValid {
			for _, rect := range diffRects(m.dirty.cacheRect, cacheRect) {
				m.MarkRectDirty(rect)
			}
		} else {
			m.MarkRectDirty(cacheRect)
		}
		m.dirty.cacheRect = cacheRect
		m.dirty.cacheValid = true
	}
	return BucketPlan{
		CacheRect: cacheRect,
		Buckets:   m.collectDirtyBuckets(cacheRect),
	}
}

func (m *BucketGridManager) Flush() {
	if m.index == nil {
		return
	}
	m.index.Flush(m.dirty.markDirtyAABB)
}

func (m *BucketGridManager) EntryAABB(entryID uint64) (spatial.AABB, bool) {
	if m.index == nil {
		return spatial.AABB{}, false
	}
	return m.index.EntryAABB(entryID)
}

func (m *BucketGridManager) QueryRange(aabb spatial.AABB, collector func(uint64)) int {
	if m.index == nil {
		return 0
	}
	return m.index.QueryRange(aabb, collector)
}

func (m *BucketGridManager) MarkRectDirty(rect spatial.AABB) {
	if m.index == nil {
		return
	}
	m.index.VisitWrappedAABB(rect, func(aabb spatial.AABB) {
		m.dirty.markDirtyAABB(aabb)
	})
}

func (m *BucketGridManager) collectDirtyBuckets(cacheRect spatial.AABB) []spatial.AABB {
	if len(m.dirty.dirtyList) == 0 {
		return nil
	}
	fragments := make([]geom.AABB[uint32], 0, 4)
	if m.index != nil {
		m.index.VisitWrappedAABB(cacheRect, func(aabb spatial.AABB) {
			fragments = append(fragments, aabb)
		})
	}
	out := make([]spatial.AABB, 0, len(m.dirty.dirtyList))
	keep := m.dirty.dirtyList[:0]
	for _, idx := range m.dirty.dirtyList {
		bucket := m.bucketRect(idx)
		if rectIntersectsAny(bucket, fragments) {
			out = append(out, bucket)
			delete(m.dirty.dirty, idx)
		} else {
			keep = append(keep, idx)
		}
	}
	m.dirty.dirtyList = keep
	return out
}

func (m *BucketGridManager) bucketRect(idx uint32) geom.AABB[uint32] {
	x := idx % m.dirty.gridSide
	y := idx / m.dirty.gridSide
	minX := x * m.dirty.bucketSize
	minY := y * m.dirty.bucketSize
	maxX := minX + m.dirty.bucketSize
	maxY := minY + m.dirty.bucketSize
	return geom.NewAABB(
		geom.NewVec(minX, minY),
		geom.NewVec(maxX, maxY),
	)
}

func (d *dirtyState) markDirtyAABB(aabb spatial.AABB) {
	x1 := aabb.TopLeft.X >> d.bucketResolution
	y1 := aabb.TopLeft.Y >> d.bucketResolution
	x2 := aabb.BottomRight.X >> d.bucketResolution
	y2 := aabb.BottomRight.Y >> d.bucketResolution

	max := d.gridSide - 1
	if x1 > max {
		x1 = max
	}
	if y1 > max {
		y1 = max
	}
	if x2 > max {
		x2 = max
	}
	if y2 > max {
		y2 = max
	}

	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			idx := uint32(y*d.gridSide + x)
			if _, ok := d.dirty[idx]; ok {
				continue
			}
			d.dirty[idx] = struct{}{}
			d.dirtyList = append(d.dirtyList, idx)
		}
	}
}

func planeAABBToSpatial(shape plane.AABB[uint32]) spatial.AABB {
	base := shape.AABB
	minX := base.TopLeft.X
	minY := base.TopLeft.Y
	width := base.BottomRight.X - base.TopLeft.X
	height := base.BottomRight.Y - base.TopLeft.Y

	var extraW, extraH uint32
	shape.VisitFragments(func(pos plane.FragPosition, frag geom.AABB[uint32]) bool {
		switch pos {
		case plane.FRAG_RIGHT, plane.FRAG_BOTTOM_RIGHT:
			w := frag.BottomRight.X - frag.TopLeft.X
			if w > extraW {
				extraW = w
			}
		}
		switch pos {
		case plane.FRAG_BOTTOM, plane.FRAG_BOTTOM_RIGHT:
			h := frag.BottomRight.Y - frag.TopLeft.Y
			if h > extraH {
				extraH = h
			}
		}
		return true
	})

	width += extraW
	height += extraH
	maxX := minX + width
	maxY := minY + height
	return spatial.NewAABB(
		spatial.NewVec(minX, minY),
		spatial.NewVec(maxX, maxY),
	)
}
