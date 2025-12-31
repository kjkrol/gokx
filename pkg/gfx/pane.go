package gfx

import "sync"

type PaneConfig struct {
	Width, Height    int
	OffsetX, OffsetY int
}

type Pane struct {
	Config *PaneConfig
	layers []*Layer
	mu     sync.Mutex
}

func newPane(conf *PaneConfig) *Pane {
	layers := make([]*Layer, 1)
	pane := Pane{
		Config: conf,
		layers: layers,
	}
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
