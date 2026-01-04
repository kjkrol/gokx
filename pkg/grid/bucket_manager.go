package grid

import (
	"fmt"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
)

const defaultOpsBuffer = 4096

type LayerConfig struct {
	WorldResolution  spatial.Resolution
	BucketResolution spatial.Resolution
	BucketCapacity   int
}

type BucketPlan struct {
	CacheRect geom.AABB[int]
	Buckets   []geom.AABB[int]
}

type BucketDelta struct {
	Bucket  uint32
	Added   []uint64
	Removed []uint64
	Updated []uint64
}

type BucketGridManager struct {
	index            spatial.Index
	space            plane.Space2D[int]
	worldResolution  spatial.Resolution
	bucketResolution spatial.Resolution
	bucketSize       uint32
	gridSide         uint32
	opsCh            chan op
	entries          map[uint64]entryCache
	entryAABB        map[uint64]geom.AABB[uint32]
	bucketDeltas     map[uint32]*bucketDelta
	dirty            map[uint32]struct{}
	dirtyList        []uint32
	cacheRect        geom.AABB[int]
	cacheValid       bool
	worldMax         int
}

type entryCache struct {
	mask uint8
}

type bucketDelta struct {
	added   map[uint64]struct{}
	removed map[uint64]struct{}
	updated map[uint64]struct{}
}

type opKind uint8

const (
	opInsert opKind = iota
	opRemove
	opUpdate
	opDirtyRect
)

type op struct {
	kind      opKind
	id        uint64
	aabb      plane.AABB[int]
	rect      geom.AABB[int]
	markDirty bool
}

func NewBucketGridManager(space plane.Space2D[int], cfg LayerConfig) (*BucketGridManager, error) {
	if space == nil {
		return nil, fmt.Errorf("space is required")
	}
	if cfg.WorldResolution == 0 {
		return nil, fmt.Errorf("world resolution is required")
	}
	if cfg.BucketResolution == 0 {
		return nil, fmt.Errorf("bucket resolution is required")
	}
	if cfg.WorldResolution < cfg.BucketResolution {
		return nil, fmt.Errorf("bucket resolution must be <= world resolution")
	}
	if cfg.BucketCapacity <= 0 {
		cfg.BucketCapacity = 2
	}
	index, err := spatial.NewBucketGrid(
		cfg.WorldResolution,
		cfg.BucketResolution,
		spatial.WithBucketCapacity(cfg.BucketCapacity),
	)
	if err != nil {
		return nil, err
	}
	worldSide := cfg.WorldResolution.Side()
	bucketSide := cfg.BucketResolution.Side()
	gridSide := worldSide / bucketSide
	worldMax := int(worldSide) - 1
	if worldMax < 0 {
		worldMax = 0
	}
	manager := &BucketGridManager{
		index:            spatial.SyncIndex(index),
		space:            space,
		worldResolution:  cfg.WorldResolution,
		bucketResolution: cfg.BucketResolution,
		bucketSize:       bucketSide,
		gridSide:         gridSide,
		opsCh:            make(chan op, defaultOpsBuffer),
		entries:          make(map[uint64]entryCache),
		entryAABB:        make(map[uint64]geom.AABB[uint32]),
		bucketDeltas:     make(map[uint32]*bucketDelta),
		dirty:            make(map[uint32]struct{}),
		worldMax:         worldMax,
	}
	return manager, nil
}

func (m *BucketGridManager) WorldResolution() spatial.Resolution {
	return m.worldResolution
}

func (m *BucketGridManager) BucketResolution() spatial.Resolution {
	return m.bucketResolution
}

func (m *BucketGridManager) BucketSize() uint32 {
	return m.bucketSize
}

func (m *BucketGridManager) BucketIndex(rect geom.AABB[int]) (uint32, bool) {
	if rect.BottomRight.X <= rect.TopLeft.X || rect.BottomRight.Y <= rect.TopLeft.Y {
		return 0, false
	}
	if rect.TopLeft.X < 0 || rect.TopLeft.Y < 0 {
		return 0, false
	}
	x := uint32(rect.TopLeft.X) / m.bucketSize
	y := uint32(rect.TopLeft.Y) / m.bucketSize
	if x >= m.gridSide || y >= m.gridSide {
		return 0, false
	}
	return y*m.gridSide + x, true
}

func (m *BucketGridManager) worldSideForCache() int {
	if m.space == nil {
		return 0
	}
	if m.space.Name() != "Toroidal2D" {
		return 0
	}
	return int(m.worldResolution.Side())
}

func (m *BucketGridManager) ConsumeBucketDeltas() []BucketDelta {
	if len(m.bucketDeltas) == 0 {
		return nil
	}
	out := make([]BucketDelta, 0, len(m.bucketDeltas))
	for idx, delta := range m.bucketDeltas {
		out = append(out, BucketDelta{
			Bucket:  idx,
			Added:   deltaKeys(delta.added),
			Removed: deltaKeys(delta.removed),
			Updated: deltaKeys(delta.updated),
		})
	}
	for idx := range m.bucketDeltas {
		delete(m.bucketDeltas, idx)
	}
	return out
}

func (m *BucketGridManager) recordBucketDelta(idx uint32) *bucketDelta {
	delta, ok := m.bucketDeltas[idx]
	if ok {
		return delta
	}
	delta = &bucketDelta{}
	m.bucketDeltas[idx] = delta
	return delta
}

func (d *bucketDelta) add(id uint64) {
	if d.added == nil {
		d.added = make(map[uint64]struct{})
	}
	if d.removed != nil {
		delete(d.removed, id)
	}
	if d.updated != nil {
		delete(d.updated, id)
	}
	d.added[id] = struct{}{}
}

func (d *bucketDelta) remove(id uint64) {
	if d.added != nil {
		if _, ok := d.added[id]; ok {
			delete(d.added, id)
			return
		}
	}
	if d.removed == nil {
		d.removed = make(map[uint64]struct{})
	}
	if d.updated != nil {
		delete(d.updated, id)
	}
	d.removed[id] = struct{}{}
}

func (d *bucketDelta) update(id uint64) {
	if d.added != nil {
		if _, ok := d.added[id]; ok {
			return
		}
	}
	if d.removed != nil {
		if _, ok := d.removed[id]; ok {
			return
		}
	}
	if d.updated == nil {
		d.updated = make(map[uint64]struct{})
	}
	d.updated[id] = struct{}{}
}

func deltaKeys(set map[uint64]struct{}) []uint64 {
	if len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (m *BucketGridManager) QueueInsert(id uint64, aabb plane.AABB[int]) {
	m.opsCh <- op{kind: opInsert, id: id, aabb: aabb, markDirty: true}
}

func (m *BucketGridManager) QueueRemove(id uint64) {
	m.opsCh <- op{kind: opRemove, id: id}
}

func (m *BucketGridManager) QueueUpdate(id uint64, aabb plane.AABB[int], markDirty bool) {
	m.opsCh <- op{kind: opUpdate, id: id, aabb: aabb, markDirty: markDirty}
}

func (m *BucketGridManager) QueueDirtyRect(rect geom.AABB[int]) {
	m.opsCh <- op{kind: opDirtyRect, rect: rect}
}

func (m *BucketGridManager) Plan(viewRect geom.AABB[int], marginBuckets int) BucketPlan {
	m.Flush()
	worldSide := m.worldSideForCache()
	if worldSide > 0 {
		viewW := viewRect.BottomRight.X - viewRect.TopLeft.X
		viewH := viewRect.BottomRight.Y - viewRect.TopLeft.Y
		if viewW >= worldSide && viewH >= worldSide {
			worldSide = 0
		}
	}
	cacheRect := cacheRectForView(viewRect, int(m.bucketSize), marginBuckets, worldSide)
	if !m.cacheValid || !rectEquals(cacheRect, m.cacheRect) {
		if m.cacheValid {
			for _, rect := range diffRects(m.cacheRect, cacheRect) {
				m.MarkRectDirty(rect)
			}
		} else {
			m.MarkRectDirty(cacheRect)
		}
		m.cacheRect = cacheRect
		m.cacheValid = true
	}
	return BucketPlan{
		CacheRect: cacheRect,
		Buckets:   m.collectDirtyBuckets(cacheRect),
	}
}

func (m *BucketGridManager) Flush() {
	for {
		select {
		case op := <-m.opsCh:
			switch op.kind {
			case opInsert:
				m.applyInsert(op.id, op.aabb, op.markDirty)
			case opRemove:
				m.applyRemove(op.id)
			case opUpdate:
				m.applyUpdate(op.id, op.aabb, op.markDirty)
			case opDirtyRect:
				m.MarkRectDirty(op.rect)
			}
		default:
			return
		}
	}
}

func (m *BucketGridManager) EntryAABB(entryID uint64) (geom.AABB[int], bool) {
	aabb, ok := m.entryAABB[entryID]
	if !ok {
		return geom.AABB[int]{}, false
	}
	return geom.NewAABB(
		geom.NewVec(int(aabb.TopLeft.X), int(aabb.TopLeft.Y)),
		geom.NewVec(int(aabb.BottomRight.X), int(aabb.BottomRight.Y)),
	), true
}

func (m *BucketGridManager) QueryRange(aabb geom.AABB[int], collector func(uint64)) int {
	var count int
	m.wrapRect(aabb, func(rect geom.AABB[uint32]) {
		count += m.index.QueryRange(rect, collector)
	})
	return count
}

func (m *BucketGridManager) MarkRectDirty(rect geom.AABB[int]) {
	m.wrapRect(rect, func(aabb geom.AABB[uint32]) {
		m.markDirtyAABB(aabb)
	})
}

func (m *BucketGridManager) applyInsert(id uint64, shape plane.AABB[int], markDirty bool) {
	mask, frags := m.buildFragments(shape)
	if mask == 0 {
		return
	}
	entries := make([]spatial.Entry, 0, 4)
	for idx := 0; idx < len(frags); idx++ {
		if mask&(1<<idx) == 0 {
			continue
		}
		entryID := entryID(id, uint8(idx))
		entries = append(entries, spatial.Entry{
			AABB: frags[idx],
			Id:   entryID,
		})
		m.entryAABB[entryID] = frags[idx]
		m.recordBucketAdds(entryID, frags[idx])
	}
	if len(entries) > 0 {
		m.index.BulkInsert(entries)
		m.entries[id] = entryCache{mask: mask}
	}
	if markDirty {
		for idx := 0; idx < len(frags); idx++ {
			if mask&(1<<idx) == 0 {
				continue
			}
			m.markDirtyAABB(frags[idx])
		}
	}
}

func (m *BucketGridManager) applyRemove(id uint64) {
	cache, ok := m.entries[id]
	if !ok {
		return
	}
	entries := make([]spatial.Entry, 0, 4)
	for idx := 0; idx < 4; idx++ {
		if cache.mask&(1<<idx) == 0 {
			continue
		}
		entryID := entryID(id, uint8(idx))
		aabb, ok := m.entryAABB[entryID]
		if !ok {
			continue
		}
		m.recordBucketRemovals(entryID, aabb)
		entries = append(entries, spatial.Entry{
			AABB: aabb,
			Id:   entryID,
		})
		delete(m.entryAABB, entryID)
		m.markDirtyAABB(aabb)
	}
	if len(entries) > 0 {
		m.index.BulkRemove(entries)
	}
	delete(m.entries, id)
}

func (m *BucketGridManager) applyUpdate(id uint64, shape plane.AABB[int], markDirty bool) {
	oldCache, ok := m.entries[id]
	if !ok {
		m.applyInsert(id, shape, markDirty)
		return
	}
	newMask, newFrags := m.buildFragments(shape)
	if newMask == 0 {
		m.applyRemove(id)
		return
	}
	if oldCache.mask == newMask {
		moves := spatial.NewEntriesMove(4)
		for idx := 0; idx < len(newFrags); idx++ {
			if newMask&(1<<idx) == 0 {
				continue
			}
			entryID := entryID(id, uint8(idx))
			oldAABB := m.entryAABB[entryID]
			newAABB := newFrags[idx]
			m.recordBucketUpdates(entryID, oldAABB, newAABB)
			moves.Append(entryID, oldAABB, newAABB)
			m.entryAABB[entryID] = newAABB
			if markDirty {
				m.markDirtyAABB(oldAABB)
				m.markDirtyAABB(newAABB)
			}
		}
		if len(moves.Old) > 0 {
			m.index.BulkMove(moves)
		}
		return
	}

	m.applyRemove(id)
	m.applyInsert(id, shape, markDirty)
}

func (m *BucketGridManager) buildFragments(shape plane.AABB[int]) (uint8, [4]geom.AABB[uint32]) {
	var frags [4]geom.AABB[uint32]
	mask := uint8(0)
	if base, ok := m.indexAABB(shape.AABB); ok {
		frags[0] = base
		mask |= 1
	}
	shape.VisitFragments(func(pos plane.FragPosition, aabb geom.AABB[int]) bool {
		idx := uint8(pos) + 1
		if frag, ok := m.indexAABB(aabb); ok {
			frags[idx] = frag
			mask |= 1 << idx
		}
		return true
	})
	return mask, frags
}

func (m *BucketGridManager) wrapRect(aabb geom.AABB[int], visit func(geom.AABB[uint32])) {
	wrapped := m.space.WrapAABB(aabb)
	if idxAABB, ok := m.indexAABB(wrapped.AABB); ok {
		visit(idxAABB)
	}
	wrapped.VisitFragments(func(_ plane.FragPosition, frag geom.AABB[int]) bool {
		if idxAABB, ok := m.indexAABB(frag); ok {
			visit(idxAABB)
		}
		return true
	})
}

func (m *BucketGridManager) markDirtyAABB(aabb geom.AABB[uint32]) {
	x1 := aabb.TopLeft.X >> m.bucketResolution
	y1 := aabb.TopLeft.Y >> m.bucketResolution
	x2 := aabb.BottomRight.X >> m.bucketResolution
	y2 := aabb.BottomRight.Y >> m.bucketResolution

	max := m.gridSide - 1
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
			idx := uint32(y*m.gridSide + x)
			if _, ok := m.dirty[idx]; ok {
				continue
			}
			m.dirty[idx] = struct{}{}
			m.dirtyList = append(m.dirtyList, idx)
		}
	}
}

func (m *BucketGridManager) recordBucketAdds(entryID uint64, aabb geom.AABB[uint32]) {
	m.forEachBucketIndex(aabb, func(idx uint32) {
		m.recordBucketDelta(idx).add(entryID)
	})
}

func (m *BucketGridManager) recordBucketRemovals(entryID uint64, aabb geom.AABB[uint32]) {
	m.forEachBucketIndex(aabb, func(idx uint32) {
		m.recordBucketDelta(idx).remove(entryID)
	})
}

func (m *BucketGridManager) recordBucketUpdates(entryID uint64, oldAABB, newAABB geom.AABB[uint32]) {
	if oldAABB == newAABB {
		m.forEachBucketIndex(newAABB, func(idx uint32) {
			m.recordBucketDelta(idx).update(entryID)
		})
		return
	}
	oldBuckets := m.bucketIndexSet(oldAABB)
	newBuckets := m.bucketIndexSet(newAABB)
	for idx := range oldBuckets {
		if _, ok := newBuckets[idx]; ok {
			m.recordBucketDelta(idx).update(entryID)
		} else {
			m.recordBucketDelta(idx).remove(entryID)
		}
	}
	for idx := range newBuckets {
		if _, ok := oldBuckets[idx]; !ok {
			m.recordBucketDelta(idx).add(entryID)
		}
	}
}

func (m *BucketGridManager) bucketIndexSet(aabb geom.AABB[uint32]) map[uint32]struct{} {
	out := make(map[uint32]struct{})
	m.forEachBucketIndex(aabb, func(idx uint32) {
		out[idx] = struct{}{}
	})
	return out
}

func (m *BucketGridManager) forEachBucketIndex(aabb geom.AABB[uint32], fn func(uint32)) {
	x1 := aabb.TopLeft.X >> m.bucketResolution
	y1 := aabb.TopLeft.Y >> m.bucketResolution
	x2 := aabb.BottomRight.X >> m.bucketResolution
	y2 := aabb.BottomRight.Y >> m.bucketResolution

	max := m.gridSide - 1
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
			fn(uint32(y*m.gridSide + x))
		}
	}
}

func (m *BucketGridManager) collectDirtyBuckets(cacheRect geom.AABB[int]) []geom.AABB[int] {
	if len(m.dirtyList) == 0 {
		return nil
	}
	fragments := make([]geom.AABB[uint32], 0, 4)
	m.wrapRect(cacheRect, func(aabb geom.AABB[uint32]) {
		fragments = append(fragments, aabb)
	})
	out := make([]geom.AABB[int], 0, len(m.dirtyList))
	keep := m.dirtyList[:0]
	for _, idx := range m.dirtyList {
		bucket := m.bucketRect(idx)
		if rectIntersectsAny(bucket, fragments) {
			out = append(out, bucketToInt(bucket))
			delete(m.dirty, idx)
		} else {
			keep = append(keep, idx)
		}
	}
	m.dirtyList = keep
	return out
}

func (m *BucketGridManager) bucketRect(idx uint32) geom.AABB[uint32] {
	x := idx % m.gridSide
	y := idx / m.gridSide
	minX := x * m.bucketSize
	minY := y * m.bucketSize
	maxX := minX + m.bucketSize
	maxY := minY + m.bucketSize
	return geom.NewAABB(
		geom.NewVec(minX, minY),
		geom.NewVec(maxX, maxY),
	)
}

func rectIntersectsAny(rect geom.AABB[uint32], regions []geom.AABB[uint32]) bool {
	for _, region := range regions {
		if rect.Intersects(region) {
			return true
		}
	}
	return false
}

func bucketToInt(rect geom.AABB[uint32]) geom.AABB[int] {
	return geom.NewAABB(
		geom.NewVec(int(rect.TopLeft.X), int(rect.TopLeft.Y)),
		geom.NewVec(int(rect.BottomRight.X), int(rect.BottomRight.Y)),
	)
}

func (m *BucketGridManager) indexAABB(aabb geom.AABB[int]) (geom.AABB[uint32], bool) {
	minX := clamp(aabb.TopLeft.X, 0, m.worldMax)
	minY := clamp(aabb.TopLeft.Y, 0, m.worldMax)
	maxX := clamp(aabb.BottomRight.X, 0, m.worldMax)
	maxY := clamp(aabb.BottomRight.Y, 0, m.worldMax)
	if maxX < minX || maxY < minY {
		return geom.AABB[uint32]{}, false
	}
	return geom.NewAABB(
		geom.NewVec(uint32(minX), uint32(minY)),
		geom.NewVec(uint32(maxX), uint32(maxY)),
	), true
}

func entryID(id uint64, frag uint8) uint64 {
	return (id << 2) | uint64(frag&0x3)
}
