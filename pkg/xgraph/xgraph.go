//go:build linux || windows || darwin

package xgraph

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
)

type Event interface{}

type Expose struct{}
type KeyPress struct {
	Code  uint64
	Label string
}
type KeyRelease struct {
	Code  uint64
	Label string
}
type ButtonPress struct {
	Button uint32
	X, Y   int
}
type ButtonRelease struct {
	Button uint32
	X, Y   int
}
type MotionNotify struct {
	X, Y int
}
type EnterNotify struct{}
type LeaveNotify struct{}
type CreateNotify struct{}
type DestroyNotify struct{}
type ClientMessage struct{}
type UnexpectedEvent struct{}

type WindowConfig struct {
	PositionX   int
	PositionY   int
	Width       int
	Height      int
	BorderWidth int
	Title       string
}

type platformWindowWrapper interface {
	show()
	close()
	nextEvent() Event
	newPlatformImageWrapper(img *image.RGBA, offsetX, offsetY int) platformImageWrapper
}

type platformImageWrapper interface {
	update(rect image.Rectangle)
	delete()
}

type PaneConfig struct {
	Width, Height    int
	OffsetX, OffsetY int
}

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

func (l *Layer) Draw(drawable Drawable) {
	l.mu.Lock()
	defer l.mu.Unlock()
	drawable.Draw(l)
}

func (l *Layer) Erase(drawable Drawable) {
	l.mu.Lock()
	defer l.mu.Unlock()
	drawable.Erase(l)
}
