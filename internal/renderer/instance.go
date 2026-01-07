package renderer

import (
	"image/color"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokx/pkg/gfx"
)

const floatsPerInstance = 12

func appendAABBInstance(dst []float32, aabb geom.AABB[uint32], style gfx.SpatialStyle) []float32 {
	minX := aabb.TopLeft.X
	minY := aabb.TopLeft.Y
	maxX := aabb.BottomRight.X
	maxY := aabb.BottomRight.Y
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}

	x0 := float32(minX)
	y0 := float32(minY)
	x1 := float32(maxX)
	y1 := float32(maxY)
	if x1 <= x0 || y1 <= y0 {
		return dst
	}

	fill := colorToFloat(style.Fill)
	stroke := colorToFloat(style.Stroke)
	dst = append(dst,
		x0, y0, x1, y1,
		fill[0], fill[1], fill[2], fill[3],
		stroke[0], stroke[1], stroke[2], stroke[3],
	)
	return dst
}

func colorToFloat(c color.Color) [4]float32 {
	if c == nil {
		return [4]float32{}
	}
	r, g, b, a := c.RGBA()
	const inv = 1.0 / 65535.0
	return [4]float32{
		float32(r) * inv,
		float32(g) * inv,
		float32(b) * inv,
		float32(a) * inv,
	}
}
