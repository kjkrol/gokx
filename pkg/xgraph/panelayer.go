package xgraph

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
)

type Layer struct {
	Img          *image.RGBA
	pane         *Pane
	mu           sync.Mutex
	idx          int
	dirtyRects   []image.Rectangle
	batchedRects []image.Rectangle
	batchDepth   int
}

func NewLayer(width, height int, pane *Pane) Layer {
	return Layer{Img: image.NewRGBA(image.Rect(0, 0, width, height)), pane: pane}
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) SetBackground(color color.Color) {
	l.mu.Lock()
	draw.Draw(l.Img, l.Img.Bounds(), &image.Uniform{color}, image.Point{}, draw.Src)
	l.queueDirtyRectLocked(l.Img.Bounds())
	l.flushLocked()
}

func (l *Layer) Draw(drawable *DrawableSpatial) {
	l.mu.Lock()
	drawable.Draw(l)
	l.flushLocked()
}

func (l *Layer) Erase(drawable *DrawableSpatial) {
	l.mu.Lock()
	drawable.Erase(l)
	l.flushLocked()
}

func (l *Layer) queueDirtyRectLocked(rect image.Rectangle) {
	l.dirtyRects = append(l.dirtyRects, rect)
}

func (l *Layer) drainDirtyLocked() []image.Rectangle {
	if len(l.dirtyRects) == 0 {
		return nil
	}
	rects := make([]image.Rectangle, len(l.dirtyRects))
	copy(rects, l.dirtyRects)
	l.dirtyRects = l.dirtyRects[:0]
	return rects
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
