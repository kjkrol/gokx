package grid

import (
	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/spatial"
)

func cacheRectForView(viewRect spatial.AABB, bucketSize uint32, marginBuckets int, worldSize uint32) spatial.AABB {
	if bucketSize == 0 {
		return viewRect
	}
	margin := bucketSize * uint32(marginBuckets)
	minX := alignDown(viewRect.TopLeft.X, bucketSize)
	minY := alignDown(viewRect.TopLeft.Y, bucketSize)
	maxX := alignUp(viewRect.BottomRight.X, bucketSize)
	maxY := alignUp(viewRect.BottomRight.Y, bucketSize)
	if margin != 0 {
		if worldSize == 0 {
			if minX > margin {
				minX -= margin
			} else {
				minX = 0
			}
			if minY > margin {
				minY -= margin
			} else {
				minY = 0
			}
		} else {
			minX -= margin
			minY -= margin
		}
		maxX += margin
		maxY += margin
	}
	width := maxX - minX
	height := maxY - minY
	originX := minX
	originY := minY
	if worldSize > 0 {
		originX = wrapUint(originX, worldSize)
		originY = wrapUint(originY, worldSize)
	}
	return geom.NewAABB(
		geom.NewVec(originX, originY),
		geom.NewVec(originX+width, originY+height),
	)
}

func rectEquals(a, b spatial.AABB) bool {
	return a.TopLeft == b.TopLeft && a.BottomRight == b.BottomRight
}

func diffRects(oldRect, newRect spatial.AABB) []spatial.AABB {
	if rectEmpty(newRect) {
		return nil
	}
	if rectEmpty(oldRect) {
		return []spatial.AABB{newRect}
	}
	inter, ok := geom.IntersectStrict(oldRect, newRect)
	if !ok {
		return []spatial.AABB{newRect}
	}
	out := make([]spatial.AABB, 0, 4)
	top := geom.NewAABB(geom.NewVec(newRect.TopLeft.X, newRect.TopLeft.Y), geom.NewVec(newRect.BottomRight.X, inter.TopLeft.Y))
	if !rectEmpty(top) {
		out = append(out, top)
	}
	bottom := geom.NewAABB(geom.NewVec(newRect.TopLeft.X, inter.BottomRight.Y), geom.NewVec(newRect.BottomRight.X, newRect.BottomRight.Y))
	if !rectEmpty(bottom) {
		out = append(out, bottom)
	}
	left := geom.NewAABB(geom.NewVec(newRect.TopLeft.X, inter.TopLeft.Y), geom.NewVec(inter.TopLeft.X, inter.BottomRight.Y))
	if !rectEmpty(left) {
		out = append(out, left)
	}
	right := geom.NewAABB(geom.NewVec(inter.BottomRight.X, inter.TopLeft.Y), geom.NewVec(newRect.BottomRight.X, inter.BottomRight.Y))
	if !rectEmpty(right) {
		out = append(out, right)
	}
	return out
}

func rectEmpty(aabb spatial.AABB) bool {
	return aabb.BottomRight.X <= aabb.TopLeft.X || aabb.BottomRight.Y <= aabb.TopLeft.Y
}

func rectIntersectsAny(rect spatial.AABB, others []geom.AABB[uint32]) bool {
	for _, other := range others {
		if _, ok := geom.IntersectStrict(rect, other); ok {
			return true
		}
	}
	return false
}

func alignDown(value, step uint32) uint32 {
	if step == 0 {
		return 0
	}
	return (value / step) * step
}

func alignUp(value, step uint32) uint32 {
	if step == 0 {
		return 0
	}
	return ((value + step - 1) / step) * step
}

func wrapUint(val, size uint32) uint32 {
	if size == 0 {
		return val
	}
	return val % size
}
