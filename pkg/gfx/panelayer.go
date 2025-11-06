package gfx

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/kjkrol/gokg/pkg/geometry"
	"github.com/kjkrol/gokx/internal/platform"
)

const defaultDirtyRectCapacity = 64

type Layer struct {
	surface         platform.Surface
	Img             *image.RGBA
	pane            *Pane
	mu              sync.Mutex
	idx             int
	dirtyRectCap    int
	dirtyRects      []image.Rectangle
	flushRects      []image.Rectangle
	batchedRects    []image.Rectangle
	batchDepth      int
	drawables       []*Drawable
	background      color.Color
	needsFullRender bool
}

func NewLayer(width, height int, pane *Pane, dirtyCap int) *Layer {
	if dirtyCap <= 0 {
		dirtyCap = defaultDirtyRectCapacity
	}
	layer := &Layer{
		surface:         platform.NewRGBASurface(width, height),
		pane:            pane,
		dirtyRectCap:    dirtyCap,
		needsFullRender: false,
	}
	layer.dirtyRects = make([]image.Rectangle, 0, dirtyCap)
	layer.flushRects = make([]image.Rectangle, dirtyCap)
	layer.drawables = make([]*Drawable, 0)
	layer.Img = layer.surface.RGBA()
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
	dst := l.surface.RGBA()
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color}, image.Point{}, draw.Src)
	l.Img = dst
	l.needsFullRender = true
	l.queueDirtyRectsLocked(dst.Bounds())
	l.flushLocked()
}

func (l *Layer) AddDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != l {
		drawable.layer.RemoveDrawable(drawable)
	}

	l.mu.Lock()
	for _, existing := range l.drawables {
		if existing == drawable {
			l.queueSpatialDirtyLocked(drawable.PlaneBox)
			l.flushLocked()
			return
		}
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	paintDrawableSurface(l.surface, drawable)
	l.queueSpatialDirtyLocked(drawable.PlaneBox)
	l.flushLocked()
}

func (l *Layer) RemoveDrawable(drawable *Drawable) {
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
	l.queueSpatialDirtyLocked(drawable.PlaneBox)
	l.flushLocked()
}

func (l *Layer) Drawables() []*Drawable {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*Drawable, len(l.drawables))
	copy(out, l.drawables)
	return out
}

func (l *Layer) ModifyDrawable(drawable *Drawable, mutate func()) {
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
	oldRects := shapeToImageRectangle(drawable.PlaneBox)
	l.mu.Unlock()

	mutate()

	newRects := shapeToImageRectangle(drawable.PlaneBox)

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

	if l.surface == nil {
		return
	}
	dst := l.surface.RGBA()
	area := rect.Intersect(dst.Bounds())
	if area.Empty() {
		return
	}

	src := image.Image(transparentFill)
	if l.background != nil {
		src = image.NewUniform(l.background)
	}
	draw.Draw(dst, area, src, image.Point{}, draw.Src)
	l.Img = dst

	for _, drawable := range l.drawables {
		if drawable == nil {
			continue
		}
		rects := shapeToImageRectangle(drawable.PlaneBox)
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
		paintDrawableSurface(l.surface, drawable)
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

func (l *Layer) queueSpatialDirtyLocked(shape geometry.PlaneBox[int]) {
	rects := shapeToImageRectangle(shape)
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
