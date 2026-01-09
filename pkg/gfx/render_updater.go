package gfx

import "time"

type renderUpdater struct {
	rendererRefreshRate time.Duration
	nextRenderTime      time.Time
	render              func()
}

func newRenderUpdater(
	rendererRefreshRate time.Duration,
	render func(),
) *renderUpdater {
	if rendererRefreshRate <= 0 {
		rendererRefreshRate = time.Second / 60
	}
	return &renderUpdater{
		rendererRefreshRate: rendererRefreshRate,
		nextRenderTime:      time.Now().Add(rendererRefreshRate),
		render:              render,
	}
}

func (r *renderUpdater) run() {
	if !time.Now().Before(r.nextRenderTime) {
		r.render()
		r.nextRenderTime = time.Now().Add(r.rendererRefreshRate)
	}
}
