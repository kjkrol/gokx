package gfx

import "github.com/kjkrol/gokg/pkg/spatial"

type FrameSource interface {
	BuildFrame(pane *Pane, viewRect spatial.AABB, viewChanged bool, layers []*Layer) FramePlan
	ConsumeBucketDeltas(layer *Layer) []BucketDelta
	EntryAABB(layer *Layer, entryID uint64) (spatial.AABB, bool)
}

type FramePlan struct {
	ViewRect       spatial.AABB
	ViewChanged    bool
	Layers         []LayerPlan
	CompositeRects []spatial.AABB
}

type LayerPlan struct {
	Layer     *Layer
	CacheRect spatial.AABB
	Buckets   []spatial.AABB
}

type BucketDelta struct {
	Bucket  spatial.AABB
	Added   []uint64
	Removed []uint64
	Updated []uint64
}
