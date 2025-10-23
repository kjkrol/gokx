package xgraph

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
}

func (w WindowConfig) convert() platform.WindowConfig {
	return platform.WindowConfig{PositionX: w.PositionX, PositionY: w.PositionY, Width: w.Width, Height: w.Height, BorderWidth: w.BorderWidth, Title: w.Title}
}

type Window struct {
	platformWinWrapper platform.PlatformWindowWrapper
	defaultPane        *Pane
	panes              map[string]*Pane
	rerfreshing        bool
	refreshDelay       time.Duration
	wg                 sync.WaitGroup
	ctx                context.Context
	cancel             context.CancelFunc
}

const maxEventWait = 50 * time.Millisecond

func NewWindow(conf WindowConfig) *Window {
	platformConfig := conf.convert()
	window := Window{
		platformWinWrapper: platform.NewPlatformWindowWrapper(platformConfig),
		panes:              make(map[string]*Pane),
	}
	window.defaultPane = newPane(
		&PaneConfig{
			Width:   conf.Width,
			Height:  conf.Height,
			OffsetX: 0,
			OffsetY: 0,
		},
		window.platformWinWrapper.NewPlatformImageWrapper)
	window.ctx, window.cancel = context.WithCancel(context.Background())
	window.rerfreshing = false
	return &window
}

func (w *Window) AddPane(name string, conf *PaneConfig) *Pane {
	pane := newPane(conf, w.platformWinWrapper.NewPlatformImageWrapper)
	w.panes[name] = pane
	return pane
}

func (w *Window) GetDefaultPane() *Pane {
	return w.defaultPane
}

func (w *Window) GetPaneByName(name string) *Pane {
	return w.panes[name]
}

func (w *Window) Show() {
	w.platformWinWrapper.Show()
	w.defaultPane.Refresh()
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
	w.defaultPane.Close()
	w.defaultPane = nil
	for _, pane := range w.panes {
		pane.Close()
	}
	w.panes = nil
	w.platformWinWrapper.Close()

}

func (w *Window) ListenEvents(handleEvent func(event Event)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	delay := w.refreshDelay
	if delay == 0 {
		delay = time.Second / 60 // domyślnie 60 FPS
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

			platformEvent := w.platformWinWrapper.NextEventTimeout(timeoutMs)

			switch ev := platformEvent.(type) {
			case platform.TimeoutEvent:
				// brak eventu — nic nie robimy tutaj
			default:
				event := convert(ev)
				handleEvent(event)
			}

			// sprawdź czy czas na render
			now = time.Now()
			if !now.Before(nextRender) {
				w.GetDefaultPane().Refresh()
				for _, pane := range w.panes {
					pane.Refresh()
				}
				nextRender = now.Add(delay)
			}
		}
	}
}

func (w *Window) StartAnimation(animation *Animation) {
	animation.Run(w.ctx, 0, &w.wg)
}
