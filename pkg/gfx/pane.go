package gfx

import (
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokx/pkg/grid"
)

type PaneConfig struct {
	Width, Height    int
	OffsetX, OffsetY int
	Grid             GridConfig
}

type Pane struct {
	Config *PaneConfig
	layers []*Layer
	viewport       *grid.Viewport
	onLayerCreated func(*Layer)
	mu             sync.Mutex
}

func newPane(conf *PaneConfig) *Pane {
	layers := make([]*Layer, 1)
	conf.Grid = normalizeGridConfig(conf.Grid, conf.Width, conf.Height)
	worldSide := int(conf.Grid.WorldResolution.Side())
	pane := Pane{
		Config: conf,
		layers: layers,
	}
	pane.viewport = grid.NewViewport(
		geom.NewVec(worldSide, worldSide),
		geom.NewVec(conf.Width, conf.Height),
		conf.Grid.WorldWrap,
	)
	layer := NewLayerDefault(&pane)
	layer.idx = 0
	layers[0] = layer
	return &pane
}

func (p *Pane) AddLayer(num int) bool {
	if num < 0 || num > len(p.layers) {
		return false
	}
	layer := NewLayerDefault(p)
	layer.idx = len(p.layers)

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

func (p *Pane) WindowToWorldCoords(x, y int) (int, int) {
	px, py := p.WindowToPaneCoords(x, y)
	if p.viewport == nil {
		return px, py
	}
	origin := p.viewport.Origin()
	wx := px + origin.X
	wy := py + origin.Y
	if p.viewport.Wrap() {
		world := p.viewport.WorldSize()
		wx = wrapInt(wx, world.X)
		wy = wrapInt(wy, world.Y)
	}
	return wx, wy
}

func (p *Pane) Viewport() *grid.Viewport {
	return p.viewport
}

func (p *Pane) setLayerObserver(fn func(*Layer)) {
	p.onLayerCreated = fn
}

func wrapInt(val, size int) int {
	if size <= 0 {
		return val
	}
	mod := val % size
	if mod < 0 {
		mod += size
	}
	return mod
}
