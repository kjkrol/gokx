package gfx

import (
	"context"
	"runtime"
	"sync"
	"time"
)

type EventDispatcher func(Event)
type EventPooler func(int) (Event, bool)

type EventBus struct {
	wg                   sync.WaitGroup
	ctx                  context.Context
	cancel               context.CancelFunc
	platformEventsPooler EventPooler
	events               chan Event
}

func NewEventLoop(bufferSize int, platformEventsPool EventPooler) *EventBus {
	if bufferSize == 0 {
		bufferSize = 1024
	}
	eventLoop := EventBus{
		events:               make(chan Event, bufferSize),
		platformEventsPooler: platformEventsPool,
	}
	eventLoop.ctx, eventLoop.cancel = context.WithCancel(context.Background())
	return &eventLoop
}

func (el *EventBus) EmitEvent(event Event) {
	if el == nil || event == nil {
		return
	}
	select {
	case el.events <- event:
	default:
		// drop if buffer full to avoid blocking producer
	}
}

func (el *EventBus) Run(
	dispatcher EventDispatcher,
	renderUpdater *renderUpdater,
	ecsUpdater *ecsUpdater,
) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	adaptiveDuration := 1 * time.Millisecond

	for {
		select {
		case <-el.ctx.Done():
			el.wg.Wait()
			return
		default:
			timeoutMs := eventPoolTimeout(renderUpdater.nextRenderTime, adaptiveDuration)
			el.consumeEvents(dispatcher, timeoutMs)

			actualWorkDuration := ecsUpdater.run()
			adaptiveDuration = updateAdaptiveDuration(adaptiveDuration, actualWorkDuration)
			renderUpdater.run()
		}
	}
}

func updateAdaptiveDuration(adaptiveDuration, lastWorkDuration time.Duration) time.Duration {
	return time.Duration(
		0.95*float64(adaptiveDuration) + 0.05*float64(lastWorkDuration),
	)
}

func (el *EventBus) consumeEvents(
	handle func(Event),
	timeoutMs int,
) int {
	count := 0
	for {
		select {
		case event := <-el.events:
			handle(event)
			count++
		default:
			event, ok := el.platformEventsPooler(timeoutMs)
			if ok {
				handle(event)
				count++
			}
			return count
		}
	}
}

func eventPoolTimeout(
	nextRender time.Time,
	avgUpdateDuration time.Duration,
) int {
	now := time.Now()
	durationUntilRender := nextRender.Sub(now)
	waitDuration := durationUntilRender - avgUpdateDuration
	if waitDuration < 0 {
		waitDuration = 0
	}
	return int(waitDuration.Milliseconds())
}
