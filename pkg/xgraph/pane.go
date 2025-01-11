package xgraph

import (
	"image"
	"image/color"
	"image/draw"
	"sync"
)

type Pane struct {
	Config             *PaneConfig
	layers             []*Layer
	img                *image.RGBA
	offscreenImg       *image.RGBA
	dirtyRects         []*image.Rectangle
	mu                 sync.Mutex // Mutex to protect the dirty flag
	platformImgWrapper platformImageWrapper
}

func newPane(
	conf *PaneConfig,
	newPlatformImageWrapper func(img *image.RGBA, offsetX, offsetY int) platformImageWrapper,
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

	// Clear the offscreen buffer only once
	for _, rect := range copiedDirtyRects {
		draw.Draw(p.offscreenImg, *rect, &image.Uniform{C: color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)
	}

	// Combine all layers onto the offscreen buffer
	for i := range p.layers {
		if i > 0 {
			break
		}
		for _, rect := range copiedDirtyRects {
			combineLayers(p.offscreenImg, p.layers[i], rect)
		}
	}

	// Copy the offscreen buffer to the main image
	for _, rect := range copiedDirtyRects {
		draw.Draw(p.img, *rect, p.offscreenImg, rect.Min, draw.Src)
	}

	// Update the on-screen image
	p.platformImgWrapper.update(p.img.Rect)
}

func combineLayers(target *image.RGBA, layer *Layer, rect *image.Rectangle) {
	layer.mu.Lock()
	defer layer.mu.Unlock()
	// Convert layer's image format if needed
	convertRGBAToBGRA(layer.Img, rect)
	// Draw the layer onto the target
	draw.Draw(target, *rect, layer.Img, rect.Min, draw.Over)
}

func convertRGBAToBGRA(img *image.RGBA, rect *image.Rectangle) {
	pix := img.Pix
	stride := img.Stride
	startX, startY := rect.Min.X-img.Rect.Min.X, rect.Min.Y-img.Rect.Min.Y
	endX, endY := rect.Max.X-img.Rect.Min.X, rect.Max.Y-img.Rect.Min.Y

	for y := startY; y < endY; y++ {
		offset := y*stride + startX*4
		for x := startX; x < endX; x++ {
			// Swap R and B channels
			pix[offset+0], pix[offset+2] = pix[offset+2], pix[offset+0]
			offset += 4
		}
	}
}

func (p *Pane) Close() {
	p.platformImgWrapper.delete()
	p.Config = nil
	for i := range p.layers {
		p.layers[i].Img = nil
	}
}

func (p *Pane) WindowToPaneCoords(x, y int) (int, int) {
	return x - p.Config.OffsetX, y - p.Config.OffsetY
}
