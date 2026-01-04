package grid

import "github.com/kjkrol/gokg/pkg/geom"

func cacheRectForView(viewRect geom.AABB[int], bucketSize, marginBuckets, worldSize int) geom.AABB[int] {
	if bucketSize <= 0 {
		return viewRect
	}
	margin := marginBuckets * bucketSize
	minX := alignDown(viewRect.TopLeft.X, bucketSize) - margin
	minY := alignDown(viewRect.TopLeft.Y, bucketSize) - margin
	maxX := alignUp(viewRect.BottomRight.X, bucketSize) + margin
	maxY := alignUp(viewRect.BottomRight.Y, bucketSize) + margin
	width := maxX - minX
	height := maxY - minY
	originX := minX
	originY := minY
	if worldSize > 0 {
		originX = wrapInt(originX, worldSize)
		originY = wrapInt(originY, worldSize)
	}
	return geom.NewAABB(
		geom.NewVec(originX, originY),
		geom.NewVec(originX+width, originY+height),
	)
}

func rectEquals(a, b geom.AABB[int]) bool {
	return a.TopLeft == b.TopLeft && a.BottomRight == b.BottomRight
}

func diffRects(oldRect, newRect geom.AABB[int]) []geom.AABB[int] {
	if rectEmpty(newRect) {
		return nil
	}
	if rectEmpty(oldRect) {
		return []geom.AABB[int]{newRect}
	}
	inter, ok := rectIntersect(oldRect, newRect)
	if !ok {
		return []geom.AABB[int]{newRect}
	}
	out := make([]geom.AABB[int], 0, 4)
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

func rectIntersect(a, b geom.AABB[int]) (geom.AABB[int], bool) {
	minX := max(a.TopLeft.X, b.TopLeft.X)
	minY := max(a.TopLeft.Y, b.TopLeft.Y)
	maxX := min(a.BottomRight.X, b.BottomRight.X)
	maxY := min(a.BottomRight.Y, b.BottomRight.Y)
	if maxX <= minX || maxY <= minY {
		return geom.AABB[int]{}, false
	}
	return geom.NewAABB(geom.NewVec(minX, minY), geom.NewVec(maxX, maxY)), true
}

func rectEmpty(aabb geom.AABB[int]) bool {
	return aabb.BottomRight.X <= aabb.TopLeft.X || aabb.BottomRight.Y <= aabb.TopLeft.Y
}

func alignDown(value, step int) int {
	return divFloor(value, step) * step
}

func alignUp(value, step int) int {
	return divCeil(value, step) * step
}

func divFloor(a, b int) int {
	if b <= 0 {
		return 0
	}
	if a >= 0 {
		return a / b
	}
	return -(((-a) + b - 1) / b)
}

func divCeil(a, b int) int {
	if b <= 0 {
		return 0
	}
	if a >= 0 {
		return (a + b - 1) / b
	}
	return -((-a) / b)
}

func wrapInt(val, size int) int {
	if size <= 0 {
		return val
	}
	mod := val % size
	if mod < 0 {
		mod += size
	}
	return mod
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
