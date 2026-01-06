package gfx

import (
	"github.com/kjkrol/gokg/pkg/spatial"
)

type LayerObserver interface {
	OnDrawableAdded(layer *Layer, drawable *Drawable, id uint64)
	OnDrawableRemoved(layer *Layer, drawable *Drawable, id uint64)
	OnLayerDirtyRect(layer *Layer, rect spatial.AABB)
}
