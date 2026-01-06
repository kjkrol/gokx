package sim

import (
	"context"
	"time"

	"github.com/kjkrol/gokx/pkg/gfx"
)

// StepFunc produces bulk drawable events for a single simulation tick.
type StepFunc func() (gfx.DrawableSetAdded, gfx.DrawableSetRemoved, gfx.DrawableSetTranslated)

type Simulation struct {
	Duration time.Duration
	Step     StepFunc
	running  bool
}

func New(duration time.Duration, step StepFunc) *Simulation {
	if duration <= 0 {
		duration = 50 * time.Millisecond
	}
	return &Simulation{
		Duration: duration,
		Step:     step,
	}
}

// Run starts the simulation loop and emits events via the provided emit function.
func (s *Simulation) Run(ctx context.Context, emit func(gfx.Event)) {
	if s == nil || s.running || s.Step == nil || emit == nil {
		return
	}
	s.running = true

	go func() {
		ticker := time.NewTicker(s.Duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				added, removed, translated := s.Step()
				if len(added.Items) > 0 {
					emit(added)
				}
				if len(removed.Items) > 0 {
					emit(removed)
				}
				if len(translated.Items) > 0 {
					emit(translated)
				}
			}
		}
	}()
}
