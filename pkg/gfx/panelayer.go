package gfx

import (
	"image/color"

	"github.com/kjkrol/gokg/pkg/geom"
)

type Layer struct {
	pane         *Pane
	idx          int
	drawables    []*Drawable
	background   color.Color
	observer     LayerObserver
	idByDrawable map[*Drawable]uint64
	drawableByID map[uint64]*Drawable
}

func NewLayer(pane *Pane) *Layer {
	layer := &Layer{
		pane:      pane,
		drawables: make([]*Drawable, 0),
	}
	return layer
}

func NewLayerDefault(pane *Pane) *Layer {
	return NewLayer(pane)
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) ID() uint64 {
	return uint64(l.idx)
}

func (l *Layer) Background() color.Color {
	bg := l.background
	return bg
}

func (l *Layer) SetBackground(color color.Color) {
	l.background = color
	observer := l.observer
	pane := l.pane
	if observer != nil && pane != nil && pane.viewport != nil {
		world := pane.viewport.WorldSize()
		observer.OnLayerDirtyRect(l, geom.NewAABBAt(geom.NewVec[uint32](0, 0), world.X, world.Y))
	}
}

func (l *Layer) AddDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != l {
		drawable.layer.RemoveDrawable(drawable)
	}
	if l.containsDrawable(drawable) {
		return
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	id := l.ensureDrawableIDLocked(drawable)
	if l.observer != nil && id != 0 {
		l.observer.OnDrawableAdded(l, drawable, id)
	}
}

func (l *Layer) RemoveDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	if !l.containsDrawable(drawable) && l.idByDrawable[drawable] == 0 {
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
	if drawable.layer == l {
		drawable.detach()
	}
	id := l.idByDrawable[drawable]
	delete(l.idByDrawable, drawable)
	delete(l.drawableByID, id)
	if l.observer != nil && id != 0 {
		l.observer.OnDrawableRemoved(l, drawable, id)
	}
}

func (l *Layer) Drawables() []*Drawable {
	out := make([]*Drawable, len(l.drawables))
	copy(out, l.drawables)
	return out
}

// ApplyUpdateWithoutObserver updates drawable data without emitting observer events.
func (l *Layer) ApplyUpdateWithoutObserver(drawable *Drawable, mutate func()) {
	if drawable == nil || mutate == nil || drawable.layer != l {
		return
	}
	mutate()
}

func (l *Layer) containsDrawable(drawable *Drawable) bool {
	for _, existing := range l.drawables {
		if existing == drawable {
			return true
		}
	}
	return false
}

func (l *Layer) SetObserver(observer LayerObserver) {
	l.observer = observer
}

func (l *Layer) DrawableID(drawable *Drawable) (uint64, bool) {
	id := l.idByDrawable[drawable]
	return id, id != 0
}

func (l *Layer) DrawableByID(id uint64) *Drawable {
	drawable := l.drawableByID[id]
	return drawable
}

func (l *Layer) ensureDrawableIDLocked(drawable *Drawable) uint64 {
	if drawable == nil {
		return 0
	}
	if l.idByDrawable == nil {
		l.idByDrawable = make(map[*Drawable]uint64)
	}
	if l.drawableByID == nil {
		l.drawableByID = make(map[uint64]*Drawable)
	}
	if id := l.idByDrawable[drawable]; id != 0 {
		return id
	}
	id := drawable.ID
	if id == 0 {
		id = NextDrawableID()
		drawable.ID = id
	}
	l.idByDrawable[drawable] = id
	l.drawableByID[id] = drawable
	return id
}
