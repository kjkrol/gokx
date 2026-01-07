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
	rerfreshing        bool
	refreshDelay       time.Duration
	width              int
	height             int
	wg                 sync.WaitGroup
	ctx                context.Context
	cancel             context.CancelFunc

	updates         chan func()
	internalEvents  chan Event
	layerObserver   LayerObserver
	nextPaneID      uint64
	drawableApplier DrawableEventsApplier
}

const maxEventWait = 50 * time.Millisecond

func NewWindow(conf WindowConfig, factory RendererFactory) *Window {
	platformConfig := conf.convert()
	bufferSize := conf.ChannelBufferSize
	if bufferSize == 0 {
		bufferSize = 1024
	}
	window := Window{
		platformWinWrapper: platform.NewPlatformWindowWrapper(platformConfig),
		panes:              make(map[string]*Pane),
		updates:            make(chan func(), bufferSize),
		internalEvents:     make(chan Event, bufferSize),
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
	window.ctx, window.cancel = context.WithCancel(context.Background())
	window.rerfreshing = false
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

func (w *Window) GLContext() any {
	if w == nil || w.platformWinWrapper == nil {
		return nil
	}
	return w.platformWinWrapper.GLContext()
}

// ScheduleUpdate enqueues a callback to be executed in the window loop before rendering.
func (w *Window) ScheduleUpdate(fn func()) {
	if w == nil || fn == nil {
		return
	}
	select {
	case w.updates <- fn:
	default:
		// drop if buffer full to avoid blocking producer
	}
}

// EmitEvent injects an event into the window loop (used by simulation).
func (w *Window) EmitEvent(event Event) {
	if w == nil || event == nil {
		return
	}
	select {
	case w.internalEvents <- event:
	default:
		// drop if buffer full to avoid blocking producer
	}
}

func (w *Window) Context() context.Context {
	if w == nil {
		return nil
	}
	return w.ctx
}

func (w *Window) SetDrawableEventsApplier(applier DrawableEventsApplier) {
	if w == nil {
		return
	}
	w.drawableApplier = applier
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

	dispatch := func(event Event) {
		w.applyDrawableEvent(event)
		if handleEvent != nil {
			handleEvent(event)
		}
	}

	poll := func(timeoutMs int) (Event, bool) {
		select {
		case ev := <-w.internalEvents:
			return ev, true
		default:
		}
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

			strategy.Consume(poll, dispatch, timeoutMs)

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

				if w.drawableApplier != nil {
					w.drawableApplier.FlushTouched()
				}

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
	if flush {
		applier.FlushTouched()
	}
}
