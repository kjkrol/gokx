package xgraph

import (
	"context"
	"sync"
	"time"
)

type Animation struct {
	Layer     *Layer
	Duration  time.Duration
	Drawables []*DrawableSpatial
	Evolve    func()
	runnung   bool
}

func NewAnimation(layer *Layer, duration time.Duration, drawables []*DrawableSpatial, evolve func()) *Animation {
	return &Animation{
		Layer:     layer,
		Duration:  duration,
		Drawables: drawables,
		Evolve:    evolve,
		runnung:   false,
	}
}

func (a *Animation) Run(ctx context.Context, id int, wg *sync.WaitGroup) {
	if a.runnung {
		return
	}
	a.runnung = true
	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(a.Duration)
		defer ticker.Stop()

		for range ticker.C {
			select {
			case <-ctx.Done():
				return
			default:
				if a.Layer == nil || a.Evolve == nil {
					continue
				}
				a.Layer.Batch(func() {
					a.Evolve()
				})
			}
		}
	}()
}
