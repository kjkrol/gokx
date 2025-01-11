package xgraph

import (
	"image"
	"image/color"
)

func drawPoint(img *image.RGBA, cord image.Point, color color.Color) {
	img.Set(cord.X, cord.Y, color)
}

func drawPolygon(img *image.RGBA, points []image.Point, fillColor color.Color) {

	// Draw the edges of the polygon
	for i := 0; i < len(points); i++ {
		start := points[i]
		end := points[(i+1)%len(points)] // Wrap around to connect the last point to the first
		points := bresenhamLine(start, end)
		drawLine(img, points, fillColor)
	}
}

func fillPolygon(img *image.RGBA, points []image.Point, col color.Color) {
	// Find the bounds of the polygon
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points {
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}

	// For each scan line, find intersections with the edges
	for y := minY; y <= maxY; y++ {
		var intersections []int
		for i := 0; i < len(points); i++ {
			p1 := points[i]
			p2 := points[(i+1)%len(points)] // Wrap around

			if (p1.Y <= y && p2.Y > y) || (p1.Y > y && p2.Y <= y) { // Edge crosses the scan line
				x := p1.X + (y-p1.Y)*(p2.X-p1.X)/(p2.Y-p1.Y) // Interpolate the x-coordinate
				intersections = append(intersections, x)
			}
		}

		// Sort intersections to fill between pairs
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

// Utility functions
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
