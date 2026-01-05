package gfx

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
)

type PaneConfig struct {
	Width, Height    int
	OffsetX, OffsetY int
	World            WorldConfig
}

type Pane struct {
	Config         *PaneConfig
	layers         []*Layer
	viewport       *Viewport
	onLayerCreated func(*Layer)
	layerObserver  LayerObserver
	mu             sync.Mutex
}

func newPane(conf *PaneConfig) *Pane {
	layers := make([]*Layer, 1)
	conf.World = normalizeWorldConfig(conf.World, conf.Width, conf.Height)
	worldSide := conf.World.WorldResolution.Side()
	pane := Pane{
		Config: conf,
		layers: layers,
	}
	pane.viewport = NewViewport(
		geom.NewVec(worldSide, worldSide),
		geom.NewVec(uint32(conf.Width), uint32(conf.Height)),
		conf.World.WorldWrap,
	)
	layer := NewLayerDefault(&pane)
	layer.idx = 0
	if pane.layerObserver != nil {
		layer.SetObserver(pane.layerObserver)
	}
	layers[0] = layer
	return &pane
}

func (p *Pane) AddLayer(num int) bool {
	if num < 0 || num > len(p.layers) {
		return false
	}
	layer := NewLayerDefault(p)
	layer.idx = len(p.layers)
	if p.layerObserver != nil {
		layer.SetObserver(p.layerObserver)
	}

	p.mu.Lock()
	p.layers = append(p.layers, layer)
	p.mu.Unlock()
	if p.onLayerCreated != nil {
		p.onLayerCreated(layer)
	}
	return true
}

func (p *Pane) GetLayer(num int) *Layer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if num < 0 || num >= len(p.layers) {
		return nil
	}
	return p.layers[num]
}

func (p *Pane) Layers() []*Layer {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Layer, len(p.layers))
	copy(out, p.layers)
	return out
}

func (p *Pane) Close() {
	p.Config = nil
	p.viewport = nil
	p.onLayerCreated = nil
	p.mu.Lock()
	for i := range p.layers {
		p.layers[i] = nil
	}
	p.layers = nil
	p.mu.Unlock()
}

func (p *Pane) WindowToPaneCoords(x, y int) (int, int) {
	return x - p.Config.OffsetX, y - p.Config.OffsetY
}

func (p *Pane) WindowToWorldCoords(x, y int) (uint32, uint32) {
	px, py := p.WindowToPaneCoords(x, y)
	if p.viewport == nil {
		return clampIntToUint(px), clampIntToUint(py)
	}
	origin := p.viewport.Origin()
	wx := clampIntToUint(px) + origin.X
	wy := clampIntToUint(py) + origin.Y
	if p.viewport.Wrap() {
		world := p.viewport.WorldSize()
		wx = wrapUint(wx, world.X)
		wy = wrapUint(wy, world.Y)
	}
	return wx, wy
}

func (p *Pane) Viewport() *Viewport {
	return p.viewport
}

func (p *Pane) SetLayerObserver(observer LayerObserver) {
	p.layerObserver = observer
	p.mu.Lock()
	for _, layer := range p.layers {
		if layer != nil {
			layer.SetObserver(observer)
		}
	}
	p.mu.Unlock()
}

func (p *Pane) SetLayerCreatedHandler(fn func(*Layer)) {
	p.onLayerCreated = fn
}

func wrapUint(val, size uint32) uint32 {
	if size == 0 {
		return val
	}
	return val % size
}

func clampIntToUint(val int) uint32 {
	if val <= 0 {
		return 0
	}
	return uint32(val)
}
