package xgraph

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
)

type Layer struct {
	Img  *image.RGBA
	pane *Pane
	mu   sync.Mutex
}

func NewLayer(width, height int, pane *Pane) Layer {
	return Layer{Img: image.NewRGBA(image.Rect(0, 0, width, height)), pane: pane}
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) SetBackground(color color.Color) {
	draw.Draw(l.Img, l.Img.Bounds(), &image.Uniform{color}, image.Point{}, draw.Src)
}

func (l *Layer) Draw(drawable *DrawableSpatial) {
	l.mu.Lock()
	defer l.mu.Unlock()
	drawable.Draw(l)
}

func (l *Layer) Erase(drawable *DrawableSpatial) {
	l.mu.Lock()
	defer l.mu.Unlock()
	drawable.Erase(l)
}
