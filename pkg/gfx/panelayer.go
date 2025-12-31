package gfx

import (
	"image/color"
	"sync"
)

type Layer struct {
	pane          *Pane
	mu            sync.Mutex
	idx           int
	drawables     []*Drawable
	background    color.Color
	instanceData  []float32
	instanceCount int
	dirty         bool
	batchDepth    int
	batchedDirty  bool
}

func NewLayer(pane *Pane) *Layer {
	layer := &Layer{
		pane:      pane,
		drawables: make([]*Drawable, 0),
		dirty:     true,
	}
	return layer
}

func NewLayerDefault(pane *Pane) *Layer {
	return NewLayer(pane)
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) SetBackground(color color.Color) {
	l.mu.Lock()
	l.background = color
	l.markDirtyLocked()
	l.mu.Unlock()
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
			l.markDirtyLocked()
			l.mu.Unlock()
			return
		}
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	l.markDirtyLocked()
	l.mu.Unlock()
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
	l.markDirtyLocked()
	l.mu.Unlock()
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
	l.mu.Unlock()

	mutate()

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		return
	}
	l.markDirtyLocked()
	l.mu.Unlock()
}

func (l *Layer) beginBatch() {
	l.mu.Lock()
	l.batchDepth++
	l.mu.Unlock()
}

func (l *Layer) endBatch() {
	l.mu.Lock()
	if l.batchDepth > 0 {
		l.batchDepth--
		if l.batchDepth == 0 && l.batchedDirty {
			l.dirty = true
			l.batchedDirty = false
		}
	}
	l.mu.Unlock()
}

func (l *Layer) Batch(fn func()) {
	l.beginBatch()
	defer l.endBatch()
	fn()
}

func (l *Layer) markDirtyLocked() {
	if l.batchDepth > 0 {
		l.batchedDirty = true
		return
	}
	l.dirty = true
}

func (l *Layer) consumeInstances(force bool) (data []float32, count int, bg color.Color, dirty bool) {
	l.mu.Lock()
	if !l.dirty && !force {
		bg = l.background
		l.mu.Unlock()
		return nil, 0, bg, false
	}
	l.instanceData = l.instanceData[:0]
	for _, drawable := range l.drawables {
		if drawable == nil {
			continue
		}
		l.instanceData = appendInstanceData(l.instanceData, drawable.AABB, drawable.Style)
	}
	data = append([]float32(nil), l.instanceData...)
	count = len(data) / floatsPerInstance
	l.instanceCount = count
	l.dirty = false
	bg = l.background
	l.mu.Unlock()
	return data, count, bg, true
}
