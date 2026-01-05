package gfx

import (
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/kjkrol/gokx/internal/platform"
)

type WindowConfig struct {
	PositionX   int
	PositionY   int
	Width       int
	Height      int
	BorderWidth int
	Title       string
	World       WorldConfig
}

func (w WindowConfig) convert() platform.WindowConfig {
	return platform.WindowConfig{PositionX: w.PositionX, PositionY: w.PositionY, Width: w.Width, Height: w.Height, BorderWidth: w.BorderWidth, Title: w.Title}
}

type Window struct {
	platformWinWrapper platform.PlatformWindowWrapper
	renderer           Renderer
	defaultPane        *Pane
	panes              map[string]*Pane
	rerfreshing        bool
	refreshDelay       time.Duration
	width              int
	height             int
	wg                 sync.WaitGroup
	ctx                context.Context
	cancel             context.CancelFunc

	updates       chan func()
	layerObserver LayerObserver
}

const maxEventWait = 50 * time.Millisecond

func NewWindow(conf WindowConfig, factory RendererFactory) *Window {
	platformConfig := conf.convert()
	window := Window{
		platformWinWrapper: platform.NewPlatformWindowWrapper(platformConfig),
		panes:              make(map[string]*Pane),
		updates:            make(chan func(), 1024),
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
	)
	if window.layerObserver != nil {
		window.defaultPane.SetLayerObserver(window.layerObserver)
	}
	if factory != nil {
		window.renderer = factory(&window)
	}
	window.ctx, window.cancel = context.WithCancel(context.Background())
	window.rerfreshing = false
	return &window
}

func (w *Window) AddPane(name string, conf *PaneConfig) *Pane {
	pane := newPane(conf)
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
	if fps <= 0 {
		fps = 60
	}
	// zapamiętaj w oknie docelowy FPS
	ms := int(math.Abs(float64(1000.0 / fps)))
	w.refreshDelay = time.Duration(ms) * time.Millisecond
}

func (w *Window) Stop() {
	w.cancel()
}

func (w *Window) Close() {
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

func (w *Window) SetRenderer(renderer Renderer) {
	if w == nil {
		return
	}
	if w.renderer != nil {
		w.renderer.Close()
	}
	w.renderer = renderer
}

func (w *Window) GLContext() any {
	if w == nil || w.platformWinWrapper == nil {
		return nil
	}
	return w.platformWinWrapper.GLContext()
}

func (w *Window) ListenEvents(handleEvent func(event Event), strategy EventsConsumerStrategy) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	delay := w.refreshDelay
	if delay == 0 {
		delay = time.Second / 60 // domyślnie 60 FPS
	}
	if strategy == nil {
		strategy = DrainAll()
	}
	poll := func(timeoutMs int) (Event, bool) {
		platformEvent := w.platformWinWrapper.NextEventTimeout(timeoutMs)
		if _, ok := platformEvent.(platform.TimeoutEvent); ok {
			return nil, false
		}
		return convert(platformEvent), true
	}

	nextRender := time.Now().Add(delay)

	for {
		select {
		case <-w.ctx.Done():
			w.wg.Wait()
			return
		default:
			now := time.Now()
			timeout := nextRender.Sub(now)
			if timeout < 0 {
				timeout = 0
			}
			if timeout > maxEventWait {
				timeout = maxEventWait
			}

			timeoutMs := int(timeout / time.Millisecond)
			if timeout > 0 && timeoutMs == 0 {
				timeoutMs = 1
			}

			strategy.Consume(poll, handleEvent, timeoutMs)

			// sprawdź czy czas na render
			now = time.Now()
			if !now.Before(nextRender) {
				// 🔹 wykonaj wszystkie oczekujące update’y
				for {
					select {
					case upd := <-w.updates:
						upd()
					default:
						goto doneUpdates
					}
				}
			doneUpdates:

				w.platformWinWrapper.BeginFrame()
				if w.renderer != nil {
					w.renderer.Render(w)
				}
				nextRender = now.Add(delay)
				w.platformWinWrapper.EndFrame()
			}
		}
	}
}

func (w *Window) StartAnimation(animation *Animation) {
	animation.Run(w.ctx, 0, &w.wg, w.updates)
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

func (w *Window) SetLayerObserver(observer LayerObserver) {
	if w == nil {
		return
	}
	w.layerObserver = observer
	if w.defaultPane != nil {
		w.defaultPane.SetLayerObserver(observer)
	}
	for _, pane := range w.panes {
		if pane != nil {
			pane.SetLayerObserver(observer)
		}
	}
}
