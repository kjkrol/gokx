package xgraph

import (
	"context"
	"math"
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
	wg                 sync.WaitGroup
	ctx                context.Context
	cancel             context.CancelFunc
}

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
}

func (w *Window) Refresh(fps int) {
	if w.rerfreshing {
		return
	}
	w.rerfreshing = true
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ms := int(math.Abs(float64(1000.0 / fps)))
		duration := time.Duration(ms) * time.Millisecond
		ticker := time.NewTicker(duration)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case <-w.ctx.Done():
				return
			default:
				w.GetDefaultPane().Refresh()
				for _, pane := range w.panes {
					pane.Refresh()
				}
			}
		}
	}()
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
	for {
		select {
		case <-w.ctx.Done():
			w.wg.Wait()
			return
		default:
			platformEvent := w.platformWinWrapper.NextEvent()
			event := convert(platformEvent)
			handleEvent(event)
		}
	}
}

func (w *Window) StartAnimation(animation *Animation) {
	animation.Run(w.ctx, 0, &w.wg)
}
