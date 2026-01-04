package gfx

type EventsConsumerStrategy interface {
	Consume(poll func(timeoutMs int) (Event, bool), handle func(Event), timeoutMs int) int
}

type DrainAllStrategy struct{}

func (DrainAllStrategy) Consume(poll func(timeoutMs int) (Event, bool), handle func(Event), timeoutMs int) int {
	count := 0
	event, ok := poll(timeoutMs)
	if !ok {
		return 0
	}
	handle(event)
	count++
	for {
		event, ok = poll(0)
		if !ok {
			return count
		}
		handle(event)
		count++
	}
}

type DrainMaxStrategy struct {
	Max int
}

func (s DrainMaxStrategy) Consume(poll func(timeoutMs int) (Event, bool), handle func(Event), timeoutMs int) int {
	max := s.Max
	if max <= 0 {
		max = 1
	}
	count := 0
	event, ok := poll(timeoutMs)
	if !ok {
		return 0
	}
	handle(event)
	count++
	for count < max {
		event, ok = poll(0)
		if !ok {
			return count
		}
		handle(event)
		count++
	}
	return count
}

func DrainAll() EventsConsumerStrategy {
	return DrainAllStrategy{}
}

func DrainMax(max int) EventsConsumerStrategy {
	return DrainMaxStrategy{Max: max}
}
