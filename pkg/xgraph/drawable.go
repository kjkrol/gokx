package xgraph

import (
	"image"
	"image/color"
)

type Drawable interface {
	Draw(layer *Layer)
	Erase(layer *Layer)
}

// ---------------------------------------------------------
// Point represents a single point on the screen

type Point struct {
	Cord  image.Point
	Color color.Color
}

func (p *Point) Draw(layer *Layer) {
	drawPoint(layer.Img, p.Cord, p.Color)
	imgBounds := layer.Img.Bounds()
	rect := rasterizePointToRect(p.Cord, imgBounds)
	layer.GetPane().MarkToRefresh(rect)
}

func (p *Point) Erase(layer *Layer) {
	tmp := p.Color
	p.Color = color.RGBA{0, 0, 0, 0}
	p.Draw(layer)
	p.Color = tmp
}

// ---------------------------------------------------------

// ---------------------------------------------------------
// Line represents a line on the screen

type Line struct {
	Start, End image.Point
	Color      color.Color
}

func (l *Line) Draw(layer *Layer) {
	points := bresenhamLine(l.Start, l.End)
	drawLine(layer.Img, points, l.Color)
	imgBounds := layer.Img.Bounds()
	rects := rasterizeLineToRects(points, imgBounds)
	for _, rect := range rects {
		layer.GetPane().MarkToRefresh(rect)
	}
}
func (l *Line) Erase(layer *Layer) {
	tmp := l.Color
	l.Color = color.RGBA{0, 0, 0, 0}
	l.Draw(layer)
	l.Color = tmp
}

// ---------------------------------------------------------

// ---------------------------------------------------------
// Polygon represents a polygon on the screen

type Polygon struct {
	Points      []image.Point
	FillColor   color.Color
	BorderColor color.Color
}

func (p *Polygon) Draw(layer *Layer) {
	fillPolygon(layer.Img, p.Points, p.FillColor)
	drawPolygon(layer.Img, p.Points, p.BorderColor)
	pane := layer.GetPane()
	rect := rasterizePolygonToRects(p.Points)
	pane.MarkToRefresh(&rect)
}

func (p *Polygon) Erase(layer *Layer) {
	tmp1 := p.FillColor
	tmp2 := p.BorderColor
	p.FillColor = color.RGBA{0, 0, 0, 0}
	p.BorderColor = color.RGBA{0, 0, 0, 0}
	p.Draw(layer)
	p.FillColor = tmp1
	p.BorderColor = tmp2
}

// ---------------------------------------------------------

func rasterizePointToRect(p image.Point, bounds image.Rectangle) *image.Rectangle {
	clampedX := clamp(p.X, bounds.Min.X, bounds.Max.X-1)
	clampedY := clamp(p.Y, bounds.Min.Y, bounds.Max.Y-1)
	rect := image.Rect(clampedX, clampedY, clampedX+1, clampedY+1)
	return &rect
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func rasterizeLineToRects(points []image.Point, bounds image.Rectangle) []*image.Rectangle {
	rects := make([]*image.Rectangle, len(points))
	for i, p := range points {
		rect := rasterizePointToRect(p, bounds)
		rects[i] = rect
	}
	return rects
}

func rasterizePolygonToRects(points []image.Point) image.Rectangle {
	if len(points) == 0 {
		return image.Rect(0, 0, 0, 0)
	}
	minX, minY := points[0].X, points[0].Y
	maxX, maxY := points[0].X, points[0].Y

	for _, p := range points {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	// Create and return the bounding rectangle
	return image.Rect(minX, minY, maxX+1, maxY+1)
}
