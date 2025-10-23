package gfx

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/kjkrol/gokg/pkg/geometry"
)

const defaultDirtyRectCapacity = 64

type Layer struct {
	Img             *image.RGBA
	pane            *Pane
	mu              sync.Mutex
	idx             int
	dirtyRectCap    int
	dirtyRects      []image.Rectangle
	flushRects      []image.Rectangle
	batchedRects    []image.Rectangle
	batchDepth      int
	drawables       []*DrawableSpatial
	background      color.Color
	needsFullRender bool
}

func NewLayer(width, height int, pane *Pane, dirtyCap int) *Layer {
	if dirtyCap <= 0 {
		dirtyCap = defaultDirtyRectCapacity
	}
	layer := &Layer{
		Img:             image.NewRGBA(image.Rect(0, 0, width, height)),
		pane:            pane,
		dirtyRectCap:    dirtyCap,
		needsFullRender: false,
	}
	layer.dirtyRects = make([]image.Rectangle, 0, dirtyCap)
	layer.flushRects = make([]image.Rectangle, dirtyCap)
	layer.drawables = make([]*DrawableSpatial, 0)
	return layer
}

func NewLayerDefault(width, height int, pane *Pane) *Layer {
	return NewLayer(width, height, pane, defaultDirtyRectCapacity)
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) SetBackground(color color.Color) {
	l.mu.Lock()
	l.background = color
	draw.Draw(l.Img, l.Img.Bounds(), &image.Uniform{color}, image.Point{}, draw.Src)
	l.needsFullRender = true
	l.queueDirtyRectsLocked(l.Img.Bounds())
	l.flushLocked()
}

func (l *Layer) AddDrawable(drawable *DrawableSpatial) {
	if drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != l {
		drawable.layer.RemoveDrawable(drawable)
	}

	l.mu.Lock()
	for _, existing := range l.drawables {
		if existing == drawable {
			l.queueSpatialDirtyLocked(drawable.Shape)
			l.flushLocked()
			return
		}
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	paintDrawable(l.Img, drawable)
	l.queueSpatialDirtyLocked(drawable.Shape)
	l.flushLocked()
}

func (l *Layer) RemoveDrawable(drawable *DrawableSpatial) {
	if drawable == nil {
		return
	}

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		return
	}

	idx := -1
	for i, existing := range l.drawables {
		if existing == drawable {
			idx = i
			break
		}
	}
	if idx >= 0 {
		l.drawables = append(l.drawables[:idx], l.drawables[idx+1:]...)
	}
	drawable.detach()
	l.needsFullRender = true
	l.queueSpatialDirtyLocked(drawable.Shape)
	l.flushLocked()
}

func (l *Layer) Drawables() []*DrawableSpatial {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*DrawableSpatial, len(l.drawables))
	copy(out, l.drawables)
	return out
}

func (l *Layer) ModifyDrawable(drawable *DrawableSpatial, mutate func()) {
	if drawable == nil || mutate == nil {
		return
	}
	if drawable.layer != l {
		mutate()
		return
	}

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		mutate()
		return
	}
	oldRects := spatialRectangles(drawable.Shape)
	l.mu.Unlock()

	mutate()

	newRects := spatialRectangles(drawable.Shape)

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		return
	}
	l.needsFullRender = true
	l.queueRectsLocked(oldRects...)
	l.queueRectsLocked(newRects...)
	l.flushLocked()
}

func (l *Layer) render(rect image.Rectangle) {
	l.mu.Lock()
	if !l.needsFullRender {
		l.mu.Unlock()
		return
	}
	defer func() {
		l.needsFullRender = false
		l.mu.Unlock()
	}()

	if l.Img == nil {
		return
	}
	area := rect.Intersect(l.Img.Bounds())
	if area.Empty() {
		return
	}

	src := image.Image(transparentFill)
	if l.background != nil {
		src = image.NewUniform(l.background)
	}
	draw.Draw(l.Img, area, src, image.Point{}, draw.Src)

	for _, drawable := range l.drawables {
		if drawable == nil || drawable.Shape == nil {
			continue
		}
		rects := spatialRectangles(drawable.Shape)
		intersects := false
		for _, r := range rects {
			if !r.Intersect(area).Empty() {
				intersects = true
				break
			}
		}
		if !intersects {
			continue
		}
		paintDrawable(l.Img, drawable)
	}
}

func (l *Layer) queueDirtyRectsLocked(rects ...image.Rectangle) {
	for _, rect := range rects {
		if rect.Empty() {
			continue
		}
		l.dirtyRects = append(l.dirtyRects, rect)
	}
}

func (l *Layer) queueRectsLocked(rects ...image.Rectangle) {
	l.queueDirtyRectsLocked(rects...)
}

func (l *Layer) queueSpatialDirtyLocked(shape geometry.Spatial[int]) {
	rects := spatialRectangles(shape)
	l.queueDirtyRectsLocked(rects...)
}

func (l *Layer) drainDirtyLocked() []image.Rectangle {
	if len(l.dirtyRects) == 0 {
		return nil
	}
	n := len(l.dirtyRects)
	if cap(l.flushRects) < n {
		newCap := n
		if newCap < l.dirtyRectCap {
			newCap = l.dirtyRectCap
		}
		l.flushRects = make([]image.Rectangle, newCap)
	}
	copy(l.flushRects[:n], l.dirtyRects)
	l.dirtyRects = l.dirtyRects[:0]
	return l.flushRects[:n:n]
}

func (l *Layer) flushDirtyRects(rects []image.Rectangle) {
	if len(rects) == 0 || l == nil || l.pane == nil {
		return
	}
	l.pane.markLayerDirty(l.idx)
	for i := range rects {
		rect := rects[i]
		l.pane.MarkToRefresh(&rect)
	}
}

func (l *Layer) flushLocked() {
	rects := l.drainDirtyLocked()
	if l.batchDepth > 0 {
		l.batchedRects = append(l.batchedRects, rects...)
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
	l.flushDirtyRects(rects)
}

func (l *Layer) beginBatch() {
	l.mu.Lock()
	l.batchDepth++
	l.mu.Unlock()
}

func (l *Layer) endBatch() {
	var rects []image.Rectangle
	l.mu.Lock()
	if l.batchDepth > 0 {
		l.batchDepth--
		if l.batchDepth == 0 && len(l.batchedRects) > 0 {
			rects = append(rects, l.batchedRects...)
			l.batchedRects = l.batchedRects[:0]
		}
	}
	l.mu.Unlock()
	if len(rects) > 0 {
		l.flushDirtyRects(rects)
	}
}

func (l *Layer) Batch(fn func()) {
	l.beginBatch()
	defer l.endBatch()
	fn()
}

func (l *Layer) SetDirtyRectCapacity(capacity int) {
	if capacity <= 0 {
		capacity = defaultDirtyRectCapacity
	}
	l.mu.Lock()
	l.dirtyRectCap = capacity
	if cap(l.dirtyRects) < capacity {
		newSlice := make([]image.Rectangle, len(l.dirtyRects), capacity)
		copy(newSlice, l.dirtyRects)
		l.dirtyRects = newSlice
	}
	if cap(l.flushRects) < capacity {
		l.flushRects = make([]image.Rectangle, capacity)
	} else if len(l.flushRects) > capacity {
		l.flushRects = l.flushRects[:capacity]
	}
	if cap(l.batchedRects) < capacity {
		newBatch := make([]image.Rectangle, len(l.batchedRects), capacity)
		copy(newBatch, l.batchedRects)
		l.batchedRects = newBatch
	}
	l.mu.Unlock()
}
