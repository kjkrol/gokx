package gfx

import "time"

type ecsUpdater struct {
	lastTime      time.Time
	fixedTimeStep time.Duration
	accumulator   time.Duration
	update        func(time.Duration)
}

func newECSUpdater(fixedTimeStep time.Duration, update func(time.Duration)) *ecsUpdater {
	if fixedTimeStep <= 0 {
		fixedTimeStep = time.Second / 120
	}
	return &ecsUpdater{
		lastTime:      time.Now(),
		fixedTimeStep: fixedTimeStep,
		accumulator:   time.Duration(0),
		update:        update,
	}
}

func (u *ecsUpdater) run() time.Duration {
	workStart := time.Now()
	frameTime := workStart.Sub(u.lastTime)
	u.lastTime = workStart
	// Zabezpieczenie przed "Spiralą Śmierci" (Spiral of Death).
	// Jeśli gra się zawiesiła (np. breakpoint) lub komputer bardzo zwolnił,
	// nie chcemy symulować 1000 kroków fizyki naraz. Ograniczamy nadrabianie do np. 250ms.
	if frameTime > 250*time.Millisecond {
		frameTime = 250 * time.Millisecond
	}
	u.accumulator += frameTime
	for u.accumulator >= u.fixedTimeStep {
		u.update(u.fixedTimeStep)
		u.accumulator -= u.fixedTimeStep
	}

	return time.Since(workStart)
}
