package gfx

type Renderer interface {
	Render(w *Window)
	Close()
}

type RendererFactory func(w *Window) Renderer
