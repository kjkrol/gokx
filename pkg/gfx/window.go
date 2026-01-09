package gfx

import (
	"context"
	"time"

	"github.com/kjkrol/gokx/internal/platform"
)

type WindowConfig struct {
	PositionX         int
	PositionY         int
	Width             int
	Height            int
	BorderWidth       int
	Title             string
	World             WorldConfig
	ChannelBufferSize int
}

func (w WindowConfig) convert() platform.WindowConfig {
	return platform.WindowConfig{
		PositionX:   w.PositionX,
		PositionY:   w.PositionY,
		Width:       w.Width,
		Height:      w.Height,
		BorderWidth: w.BorderWidth,
		Title:       w.Title,
	}
}

type Window struct {
	platformWinWrapper platform.PlatformWindowWrapper
	renderer           Renderer
	defaultPane        *Pane
	panes              map[string]*Pane

	width  int
	height int

	eventLoop           *EventBus
	rendererRefreshRate time.Duration
	ecsRefreshRate      time.Duration

	layerObserver   LayerObserver
	nextPaneID      uint64
	drawableApplier DrawableEventsApplier
}

func NewWindow(conf WindowConfig, factory RendererFactory) *Window {
	platformConfig := conf.convert()
	window := Window{
		platformWinWrapper: platform.NewPlatformWindowWrapper(platformConfig),
		panes:              make(map[string]*Pane),
		width:              conf.Width,
		height:             conf.Height,
	}
	if window.platformWinWrapper == nil {
		panic("platform window wrapper is required")
	}
	window.defaultPane = newPane(
		&PaneConfig{
			Width:   conf.Width,
			Height:  conf.Height,
			OffsetX: 0,
			OffsetY: 0,
			World:   conf.World,
		},
		0,
	)
	if window.layerObserver != nil {
		window.defaultPane.SetLayerObserver(window.layerObserver)
	}
	if factory != nil {
		window.renderer = factory(&window)
	}

	window.eventLoop = NewEventLoop(
		conf.ChannelBufferSize,
		func(timeoutMs int) (Event, bool) {
			platformEvent := window.platformWinWrapper.NextEventTimeout(timeoutMs)
			if _, ok := platformEvent.(platform.TimeoutEvent); ok {
				return nil, false
			}
			return convert(platformEvent), true
		})

	window.nextPaneID = 1
	return &window
}

func (w *Window) AddPane(name string, conf *PaneConfig) *Pane {
	pane := newPane(conf, w.nextPaneID)
	w.nextPaneID++
	if w.layerObserver != nil {
		pane.SetLayerObserver(w.layerObserver)
	}
	w.panes[name] = pane
	return pane
}

func (w *Window) GetDefaultPane() *Pane {
	return w.defaultPane
}

func (w *Window) GetPaneByName(name string) *Pane {
	return w.panes[name]
}

func (w *Window) Panes() []*Pane {
	return w.panesSnapshot()
}

func (w *Window) Size() (int, int) {
	if w == nil {
		return 0, 0
	}
	return w.width, w.height
}

func (w *Window) Show() {
	w.platformWinWrapper.Show()
}

func (w *Window) RefreshRate(fps int) {
	w.rendererRefreshRate = time.Second / time.Duration(fps)
}

func (w *Window) ECSRefreshRate(rps int) {
	w.rendererRefreshRate = time.Second / time.Duration(rps)
}

func (w *Window) ListenEvents(dispather EventDispatcher) {
	dispatch := func(event Event) {
		w.applyDrawableEvent(event)
		if dispather != nil {
			dispather(event)
		}
	}

	renderUpdater := newRenderUpdater(w.rendererRefreshRate, func() {
		w.drawableApplier.FlushTouched()
		w.platformWinWrapper.BeginFrame()
		w.renderer.Render(w)
		w.platformWinWrapper.EndFrame()
	})
	ecsAdaptiveUpdater := newECSUpdater(w.ecsRefreshRate, func(d time.Duration) {})

	w.eventLoop.Run(dispatch, renderUpdater, ecsAdaptiveUpdater)
}

func (w *Window) Stop() {
	w.eventLoop.cancel()
}

func (w *Window) Close() {
	w.Stop()
	if w.renderer != nil {
		w.renderer.Close()
		w.renderer = nil
	}
	w.defaultPane.Close()
	w.defaultPane = nil
	for _, pane := range w.panes {
		pane.Close()
	}
	w.panes = nil
	w.platformWinWrapper.Close()

}

// TODO: sprawdzic czy uzywane; jesli nie usunac
func (w *Window) GLContext() any {
	if w == nil || w.platformWinWrapper == nil {
		return nil
	}
	return w.platformWinWrapper.GLContext()
}

// EmitEvent injects an event into the window loop (used by simulation).
func (w *Window) EmitEvent(event Event) {
	w.eventLoop.EmitEvent(event)
}

func (w *Window) Context() context.Context {
	if w == nil {
		return nil
	}
	return w.eventLoop.ctx
}

// TODO: ta metoda jest STASZLIWIE dziwna; postarac sie to usunac
func (w *Window) SetDrawableEventsApplier(applier DrawableEventsApplier) {
	if w == nil {
		return
	}
	w.drawableApplier = applier
}

func (w *Window) panesSnapshot() []*Pane {
	if w == nil {
		return nil
	}
	out := make([]*Pane, 0, len(w.panes)+1)
	if w.defaultPane != nil {
		out = append(out, w.defaultPane)
	}
	for _, pane := range w.panes {
		out = append(out, pane)
	}
	return out
}

func (w *Window) applyDrawableEvent(event Event) {
	applier := w.drawableApplier
	if applier == nil {
		return
	}
	flush := false
	switch e := event.(type) {
	case DrawableSetAdded:
		applier.ApplyAdded(e.Items)
		flush = true
	case DrawableSetRemoved:
		applier.ApplyRemoved(e.Items)
		flush = true
	case DrawableSetTranslated:
		applier.ApplyTranslated(e.Items)
		flush = true
	}

	// TODO: ta logika wola o pomste do nieba
	if flush {
		applier.FlushTouched()
	}
}
