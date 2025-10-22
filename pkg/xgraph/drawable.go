package xgraph

import (
	"image"
	"image/color"

	"github.com/kjkrol/gokg/pkg/geometry"
)

type SpatialStyle struct {
	Fill   color.Color
	Stroke color.Color
}

type DrawableSpatial struct {
	Shape geometry.Spatial[int]
	Style SpatialStyle
}

var transparentColor = color.RGBA{0, 0, 0, 0}

func (d *DrawableSpatial) Draw(layer *Layer) {
	if d == nil || d.Shape == nil {
		return
	}
	d.drawSpatial(layer)
	markSpatial(layer, d.Shape)
}

func (d *DrawableSpatial) Erase(layer *Layer) {
	original := d.Style
	d.Style = SpatialStyle{transparentColor, transparentColor}
	d.Draw(layer)
	d.Style = original
}

func (d *DrawableSpatial) drawSpatial(layer *Layer) {
	switch s := d.Shape.(type) {
	case *geometry.Vec[int]:
		drawSpatialVec(layer, *s, d.Style)
	case *geometry.Line[int]:
		drawSpatialLine(layer, s.Start, s.End, d.Style)
	case *geometry.Polygon[int]:
		drawSpatialPolygon(layer, s.Points(), d.Style)
	case *geometry.Rectangle[int]:
		drawSpatialRectangle(layer, *s, d.Style)
	default:
		drawSpatialRectangle(layer, d.Shape.Bounds(), d.Style)
	}
}

func drawSpatialVec(layer *Layer, v geometry.Vec[int], style SpatialStyle) {
	if style.Stroke == nil {
		return
	}
	drawPoint(layer.Img, vecToImagePoint(v), style.Stroke)
}

func drawSpatialLine(layer *Layer, start, end geometry.Vec[int], style SpatialStyle) {
	if style.Stroke == nil {
		return
	}
	points := bresenhamLine(vecToImagePoint(start), vecToImagePoint(end))
	drawLine(layer.Img, points, style.Stroke)
}

func drawSpatialPolygon(layer *Layer, vertices []geometry.Vec[int], style SpatialStyle) {
	if len(vertices) < 3 {
		return
	}
	points := vecsToImagePoints(vertices)
	if style.Fill != nil {
		fillPolygon(layer.Img, points, style.Fill)
	}
	if style.Stroke != nil {
		drawPolygonOutline(layer.Img, points, style.Stroke)
	}
}

func drawSpatialRectangle(layer *Layer, rect geometry.Rectangle[int], style SpatialStyle) {
	topLeft := rect.TopLeft
	bottomRight := rect.BottomRight
	vertices := []geometry.Vec[int]{
		{X: topLeft.X, Y: topLeft.Y},
		{X: bottomRight.X, Y: topLeft.Y},
		{X: bottomRight.X, Y: bottomRight.Y},
		{X: topLeft.X, Y: bottomRight.Y},
	}
	drawSpatialPolygon(layer, vertices, style)
}

func markSpatial(layer *Layer, shape geometry.Spatial[int]) {
	rect := geometryRectToImageRect(shape.Bounds())
	layer.GetPane().MarkToRefresh(&rect)
}

func geometryRectToImageRect(r geometry.Rectangle[int]) image.Rectangle {
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

func drawPoint(img *image.RGBA, cord image.Point, color color.Color) {
	img.Set(cord.X, cord.Y, color)
}

func drawPolygonOutline(img *image.RGBA, points []image.Point, stroke color.Color) {
	for i := 0; i < len(points); i++ {
		start := points[i]
		end := points[(i+1)%len(points)]
		linePoints := bresenhamLine(start, end)
		drawLine(img, linePoints, stroke)
	}
}

func fillPolygon(img *image.RGBA, points []image.Point, col color.Color) {
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	for y := minY; y <= maxY; y++ {
		var intersections []int
		for i := 0; i < len(points); i++ {
			p1 := points[i]
			p2 := points[(i+1)%len(points)]
			if (p1.Y <= y && p2.Y > y) || (p1.Y > y && p2.Y <= y) {
				x := p1.X + (y-p1.Y)*(p2.X-p1.X)/(p2.Y-p1.Y)
				intersections = append(intersections, x)
			}
		}

		sortInts(intersections)
		for i := 0; i < len(intersections); i += 2 {
			if i+1 < len(intersections) {
				for x := intersections[i]; x <= intersections[i+1]; x++ {
					img.Set(x, y, col)
				}
			}
		}
	}
}

func drawLine(img *image.RGBA, points []image.Point, col color.Color) {
	for _, p := range points {
		img.Set(p.X, p.Y, col)
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

	points := []image.Point{}
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func sortInts(slice []int) {
	for i := 0; i < len(slice); i++ {
		for j := i + 1; j < len(slice); j++ {
			if slice[i] > slice[j] {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}
