package renderer

import "github.com/kjkrol/gokx/pkg/gfx"

func NewRendererFactory(conf RendererConfig, source gfx.FrameSource) gfx.RendererFactory {
	return func(w *gfx.Window) gfx.Renderer {
		return newRenderer(w, conf, source)
	}
}
