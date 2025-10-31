package gfx

import (
	"image"
	"image/color"
	"sort"

	"github.com/kjkrol/gokg/pkg/geometry"
	"github.com/kjkrol/gokx/internal/platform"
)

type SpatialStyle struct {
	Fill   color.Color
	Stroke color.Color
}

type DrawableSpatial struct {
	Shape geometry.Shape[int]
	Style SpatialStyle
	layer *Layer
}

var (
	transparentColor = color.RGBA{0, 0, 0, 0}
	transparentFill  = image.NewUniform(transparentColor)
)

func (d *DrawableSpatial) Update(mutator func(shape geometry.Shape[int])) {
	if d == nil || mutator == nil {
		return
	}
	if d.layer == nil {
		mutator(d.Shape)
		return
	}
	d.layer.ModifyDrawable(d, func() {
		mutator(d.Shape)
	})
}

func (d *DrawableSpatial) attach(layer *Layer) {
	d.layer = layer
}

func (d *DrawableSpatial) detach() {
	d.layer = nil
}

func paintDrawableSurface(surface platform.Surface, drawable *DrawableSpatial) {
	if surface == nil || drawable == nil || drawable.Shape == nil {
		return
	}
	paintShapeSurface(surface, drawable.Style, drawable.Shape)
	fragments := drawable.Shape.Fragments()
	for _, fragment := range fragments {
		if fragment == nil {
			continue
		}
		paintShapeSurface(surface, drawable.Style, fragment)
	}
}

func paintShapeSurface(surface platform.Surface, style SpatialStyle, shape geometry.Shape[int]) {
	switch s := shape.(type) {
	case *geometry.Vec[int]:
		point := rasterizeVec(*s)
		paintVecSurface(surface, point, style)
	case *geometry.Line[int]:
		points := rasterizeLine(vecToImagePoint(s.Start), vecToImagePoint(s.End))
		paintLineSurface(surface, points, style)
	case *geometry.Polygon[int]:
		points := rasterizePolygon(s.Points())
		paintPolygonSurface(surface, points, style)
	default:
		// do nothing
	}
}

func rasterizeVec(v geometry.Vec[int]) image.Point {
	return vecToImagePoint(v)
}

func paintVecSurface(surface platform.Surface, point image.Point, style SpatialStyle) {
	if style.Stroke == nil {
		return
	}
	surface.Set(point.X, point.Y, style.Stroke)
}

func rasterizeLine(start, end image.Point) []image.Point {
	return bresenhamLine(start, end)
}

func paintLineSurface(surface platform.Surface, points []image.Point, style SpatialStyle) {
	if style.Stroke == nil || len(points) == 0 {
		return
	}
	paintLinePixelsSurface(surface, points, style.Stroke)
}

func rasterizePolygon(vertices []geometry.Vec[int]) []image.Point {
	if len(vertices) == 0 {
		return nil
	}
	return vecsToImagePoints(vertices)
}

func paintPolygonSurface(surface platform.Surface, points []image.Point, style SpatialStyle) {
	if len(points) < 3 {
		return
	}
	if style.Fill != nil {
		fillPolygonSurface(surface, points, style.Fill)
	}
	if style.Stroke != nil {
		paintPolygonOutlineSurface(surface, points, style.Stroke)
	}
}

func paintPolygonOutlineSurface(surface platform.Surface, points []image.Point, stroke color.Color) {
	if stroke == nil {
		return
	}
	for i := 0; i < len(points); i++ {
		start := points[i]
		end := points[(i+1)%len(points)]
		paintLinePixelsSurface(surface, rasterizeLine(start, end), stroke)
	}
}

func fillPolygonSurface(surface platform.Surface, points []image.Point, col color.Color) {
	if col == nil || len(points) == 0 {
		return
	}
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	intersections := make([]int, len(points))

	for y := minY; y <= maxY; y++ {
		count := 0
		for i := 0; i < len(points); i++ {
			p1 := points[i]
			p2 := points[(i+1)%len(points)]
			if (p1.Y <= y && p2.Y > y) || (p1.Y > y && p2.Y <= y) {
				denominator := p2.Y - p1.Y
				if denominator == 0 {
					continue
				}
				x := p1.X + (y-p1.Y)*(p2.X-p1.X)/denominator
				intersections[count] = x
				count++
			}
		}

		if count < 2 {
			continue
		}

		sort.Ints(intersections[:count])
		for i := 0; i+1 < count; i += 2 {
			startX := intersections[i]
			endX := intersections[i+1]
			for x := startX; x <= endX; x++ {
				surface.Set(x, y, col)
			}
		}
	}
}

func paintLinePixelsSurface(surface platform.Surface, points []image.Point, col color.Color) {
	for _, p := range points {
		surface.Set(p.X, p.Y, col)
	}
}

func bresenhamLine(start, end image.Point) []image.Point {
	x0, y0 := start.X, start.Y
	x1, y1 := end.X, end.Y

	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := -1
	sy := -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	points := make([]image.Point, 0, max(dx, dy)+1)
	for {
		points = append(points, image.Pt(x0, y0))
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
	return points
}

func vecToImagePoint(v geometry.Vec[int]) image.Point {
	return image.Pt(v.X, v.Y)
}

func vecsToImagePoints(vecs []geometry.Vec[int]) []image.Point {
	points := make([]image.Point, len(vecs))
	for i, v := range vecs {
		points[i] = vecToImagePoint(v)
	}
	return points
}

func aabbToImageRect(r geometry.AABB[int]) image.Rectangle {
	minX := r.TopLeft.X
	minY := r.TopLeft.Y
	maxX := r.BottomRight.X
	maxY := r.BottomRight.Y
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func shapeToImageRectangle(shape geometry.Shape[int]) []image.Rectangle {
	if shape == nil {
		return nil
	}
	rects := make([]image.Rectangle, 0, 1)
	mainRect := aabbToImageRect(shape.Bounds())
	if !mainRect.Empty() {
		rects = append(rects, mainRect)
	}
	fragments := shape.Fragments()
	if len(fragments) == 0 {
		return rects
	}
	for _, fragment := range fragments {
		if fragment == nil {
			continue
		}
		rect := aabbToImageRect(fragment.Bounds())
		if rect.Empty() {
			continue
		}
		rects = append(rects, rect)
	}
	return rects
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
