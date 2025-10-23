package gfx

import (
	"image"
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
	imgSurface         platform.Surface
	img                *image.RGBA
	offscreenSurface   platform.Surface
	offscreenImg       *image.RGBA
	dirtyBounds        image.Rectangle
	dirty              bool
	mu                 sync.Mutex
	compMu             sync.Mutex
	layerComposites    []platform.Surface
	compositeDirty     []bool
	platformImgWrapper platform.PlatformImageWrapper
	surfaceFactory     platform.SurfaceFactory
}

func newPane(
	conf *PaneConfig,
	surfaceFactory platform.SurfaceFactory,
	newPlatformImageWrapper func(img *image.RGBA, offsetX, offsetY int) platform.PlatformImageWrapper,
) *Pane {
	if surfaceFactory == nil {
		surfaceFactory = platform.DefaultSurfaceFactory()
	}
	imgSurface := surfaceFactory.New(conf.Width, conf.Height)
	img := imgSurface.RGBA()
	imageWrapper := newPlatformImageWrapper(img, conf.OffsetX, conf.OffsetY)
	offscreenSurface := surfaceFactory.New(conf.Width, conf.Height)
	offscreenImg := offscreenSurface.RGBA()
	layers := make([]*Layer, 1)
	pane := Pane{
		Config:             conf,
		layers:             layers,
		dirtyBounds:        offscreenImg.Rect,
		dirty:              true,
		layerComposites:    make([]platform.Surface, 1),
		compositeDirty:     []bool{true},
		platformImgWrapper: imageWrapper,
		imgSurface:         imgSurface,
		img:                img,
		offscreenSurface:   offscreenSurface,
		offscreenImg:       offscreenImg,
		surfaceFactory:     surfaceFactory,
	}
	layer := NewLayerDefault(conf.Width, conf.Height, &pane)
	layer.idx = 0
	layers[0] = layer
	pane.layerComposites[0] = offscreenSurface
	return &pane
}

func (p *Pane) AddLayer(num int) bool {
	if num < 0 || num > len(p.layers) {
		return false
	}
	layer := NewLayerDefault(p.Config.Width, p.Config.Height, p)
	layer.idx = len(p.layers)
	p.layers = append(p.layers, layer)

	p.mu.Lock()
	if !p.dirty {
		p.dirtyBounds = p.img.Rect
		p.dirty = true
	} else {
		p.dirtyBounds = p.dirtyBounds.Union(p.img.Rect)
	}
	p.mu.Unlock()

	p.compMu.Lock()
	if len(p.layerComposites) > 0 {
		lastIdx := len(p.layerComposites) - 1
		if p.layerComposites[lastIdx] == p.offscreenSurface {
			rect := p.img.Rect
			p.layerComposites[lastIdx] = p.surfaceFactory.New(rect.Dx(), rect.Dy())
		}
	}
	p.layerComposites = append(p.layerComposites, p.offscreenSurface)
	p.compositeDirty = append(p.compositeDirty, true)
	for i := range p.compositeDirty {
		p.compositeDirty[i] = true
	}
	p.compMu.Unlock()
	return true
}

func (p *Pane) GetLayer(num int) *Layer {
	return p.layers[num]
}

func (p *Pane) MarkToRefresh(rect *image.Rectangle) {
	p.mu.Lock()
	defer p.mu.Unlock()
	clippedRect := rect.Intersect(p.img.Rect)
	if !clippedRect.Empty() {
		if !p.dirty {
			p.dirtyBounds = clippedRect
			p.dirty = true
			return
		}
		p.dirtyBounds = p.dirtyBounds.Union(clippedRect)
	}
}

func (p *Pane) markLayerDirty(idx int) {
	p.compMu.Lock()
	defer p.compMu.Unlock()
	if idx < 0 || idx >= len(p.compositeDirty) {
		return
	}
	for i := idx; i < len(p.compositeDirty); i++ {
		p.compositeDirty[i] = true
	}
}

func (p *Pane) takeDirtyBounds() (image.Rectangle, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.dirty {
		return image.Rectangle{}, false
	}
	rect := p.dirtyBounds
	p.dirty = false
	return rect, true
}

func (p *Pane) Refresh() {
	minRect, dirty := p.takeDirtyBounds()
	if !dirty {
		return
	}

	p.compMu.Lock()
	start := len(p.layerComposites)
	for i, needs := range p.compositeDirty {
		if needs {
			start = i
			break
		}
	}
	if start == len(p.layerComposites) {
		start = 0
	}

	p.rebuildComposites(minRect, start)
	finalComposite := p.ensureComposite(len(p.layerComposites) - 1)
	p.compMu.Unlock()

	if finalComposite != nil {
		draw.Draw(p.img, minRect, finalComposite.RGBA(), minRect.Min, draw.Src)
	}
	p.platformImgWrapper.Update(minRect)
}

func (p *Pane) ensureComposite(idx int) platform.Surface {
	if idx < 0 || idx >= len(p.layerComposites) {
		return nil
	}
	if p.layerComposites[idx] == nil {
		if idx == len(p.layerComposites)-1 {
			p.layerComposites[idx] = p.offscreenSurface
		} else {
			rect := p.img.Rect
			p.layerComposites[idx] = p.surfaceFactory.New(rect.Dx(), rect.Dy())
		}
	}
	return p.layerComposites[idx]
}

func (p *Pane) rebuildComposites(rect image.Rectangle, start int) {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(p.layers); i++ {
		layer := p.layers[i]
		if layer == nil {
			continue
		}
		layer.render(rect)

		dst := p.ensureComposite(i)
		if dst == nil {
			continue
		}

		dstImg := dst.RGBA()
		clipped := rect.Intersect(dstImg.Bounds())
		if clipped.Empty() {
			p.compositeDirty[i] = false
			continue
		}

		if i == 0 {
			layer.mu.Lock()
			draw.Draw(dstImg, clipped, layer.Img, clipped.Min, draw.Src)
			layer.mu.Unlock()
		} else {
			prev := p.ensureComposite(i - 1)
			if prev != nil {
				draw.Draw(dstImg, clipped, prev.RGBA(), clipped.Min, draw.Src)
			}
			layer.mu.Lock()
			draw.Draw(dstImg, clipped, layer.Img, clipped.Min, draw.Over)
			layer.mu.Unlock()
		}
		p.compositeDirty[i] = false
	}
}

func (p *Pane) Close() {
	p.platformImgWrapper.Delete()
	p.Config = nil
	for i := range p.layers {
		p.layers[i].Img = nil
	}
	p.imgSurface = nil
	p.offscreenSurface = nil
	p.surfaceFactory = nil
	p.layerComposites = nil
	p.compositeDirty = nil
}

func (p *Pane) WindowToPaneCoords(x, y int) (int, int) {
	return x - p.Config.OffsetX, y - p.Config.OffsetY
}
