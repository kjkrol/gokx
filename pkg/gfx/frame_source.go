package gfx

import (
	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/spatial"
)

type FrameSource interface {
	BuildFrame(pane *Pane, viewRect spatial.AABB, viewChanged bool, layers []*Layer) FramePlan
	ConsumeBucketDeltas(layer *Layer) []BucketDelta
	EntryAABB(layer *Layer, entryID uint64) (spatial.AABB, bool)
	AcknowledgeRendered(layer *Layer, bucketIndices []uint32)
}

type FramePlan struct {
	ViewRect       spatial.AABB
	ViewChanged    bool
	Layers         []LayerPlan
	CompositeRects []spatial.AABB
}

type LayerPlan struct {
	Layer         *Layer
	CacheRect     spatial.AABB
	BucketIndices []uint32
	BucketRect    func(uint32) geom.AABB[uint32]
}

type BucketDelta struct {
	Bucket  spatial.AABB
	Added   []uint64
	Removed []uint64
	Updated []uint64
}
