package xgraph

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/kjkrol/gokx/internal/platform"
)

type PaneConfig struct {
	Width, Height    int
	OffsetX, OffsetY int
}

type Pane struct {
	Config             *PaneConfig
	layers             []*Layer
	img                *image.RGBA
	offscreenImg       *image.RGBA
	dirtyRects         []*image.Rectangle
	mu                 sync.Mutex // Mutex to protect the dirty flag
	platformImgWrapper platform.PlatformImageWrapper
}

func newPane(
	conf *PaneConfig,
	newPlatformImageWrapper func(img *image.RGBA, offsetX, offsetY int) platform.PlatformImageWrapper,
) *Pane {
	img := image.NewRGBA(image.Rect(0, 0, conf.Width, conf.Height))
	imageWrapper := newPlatformImageWrapper(img, conf.OffsetX, conf.OffsetY)
	offscreenImg := image.NewRGBA(image.Rect(0, 0, conf.Width, conf.Height))
	layers := make([]*Layer, 1)
	pane := Pane{
		Config:             conf,
		layers:             layers,
		dirtyRects:         []*image.Rectangle{&offscreenImg.Rect},
		platformImgWrapper: imageWrapper,
		img:                img,
		offscreenImg:       offscreenImg,
	}
	layer := NewLayer(conf.Width, conf.Height, &pane)
	layers[0] = &layer
	return &pane
}

func (p *Pane) AddLayer(num int) bool {
	if num < 0 || num > len(p.layers) {
		return false
	}
	layer := NewLayer(p.Config.Width, p.Config.Height, p)
	p.layers = append(p.layers, &layer)
	return true
}

func (p *Pane) GetLayer(num int) *Layer {
	return p.layers[num]
}

func (p *Pane) MarkToRefresh(rect *image.Rectangle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Intersect rect with p.img.Rect to ensure it is within bounds
	clippedRect := rect.Intersect(p.img.Rect)

	// Only add the rectangle if it still has non-zero size
	if !clippedRect.Empty() {
		p.dirtyRects = append(p.dirtyRects, &clippedRect)
	}
}

func (p *Pane) CopyDirtyRects() []*image.Rectangle {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dirtyRects == nil {
		return nil
	}
	rects := make([]*image.Rectangle, len(p.dirtyRects))
	copy(rects, p.dirtyRects)
	p.dirtyRects = nil
	return rects
}

func (p *Pane) Refresh() {
	copiedDirtyRects := p.CopyDirtyRects()
	if copiedDirtyRects == nil {
		return
	}

	// oblicz bounding box
	minRect := *copiedDirtyRects[0]
	for _, r := range copiedDirtyRects[1:] {
		minRect = minRect.Union(*r)
	}

	// wyczyść tylko bounding box w offscreen
	draw.Draw(
		p.offscreenImg,
		minRect,
		&image.Uniform{C: color.RGBA{0, 0, 0, 0}},
		image.Point{},
		draw.Src,
	)

	// wyrenderuj wszystkie warstwy w tym prostokącie
	for i := range p.layers {
		combineLayers(p.offscreenImg, p.layers[i], &minRect)
	}

	// skopiuj tylko bounding box do p.img
	draw.Draw(p.img, minRect, p.offscreenImg, minRect.Min, draw.Src)

	// update GPU tylko dla tego obszaru
	p.platformImgWrapper.Update(minRect)
}

func combineLayers(target *image.RGBA, layer *Layer, rect *image.Rectangle) {
	layer.mu.Lock()
	defer layer.mu.Unlock()
	draw.Draw(target, *rect, layer.Img, rect.Min, draw.Over)
}

func (p *Pane) Close() {
	p.platformImgWrapper.Delete()
	p.Config = nil
	for i := range p.layers {
		p.layers[i].Img = nil
	}
}

func (p *Pane) WindowToPaneCoords(x, y int) (int, int) {
	return x - p.Config.OffsetX, y - p.Config.OffsetY
}
