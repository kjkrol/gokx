package gfx

import (
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokx/internal/platform"
)

type Event interface{}

type Expose struct{}
type KeyPress struct {
	Code  uint64
	Label string
}
type KeyRelease struct {
	Code  uint64
	Label string
}
type ButtonPress struct {
	Button uint32
	X, Y   int
}
type ButtonRelease struct {
	Button uint32
	X, Y   int
}
type MotionNotify struct {
	X, Y int
}
type EnterNotify struct{}
type LeaveNotify struct{}
type CreateNotify struct{}
type DestroyNotify struct{}
type ClientMessage struct{}
type MouseWheel struct {
	DeltaX float64
	DeltaY float64
	X, Y   int
}
type UnexpectedEvent struct{}

type DrawableAdd struct {
	PaneID     uint64
	LayerID    uint64
	DrawableID uint64
	AABB       plane.AABB[uint32]
}

type DrawableRemove struct {
	PaneID     uint64
	LayerID    uint64
	DrawableID uint64
}

type DrawableTranslate struct {
	PaneID     uint64
	LayerID    uint64
	DrawableID uint64
	Old        plane.AABB[uint32]
	New        plane.AABB[uint32]
}

type DrawableSetAdded struct {
	Items []DrawableAdd
}

type DrawableSetRemoved struct {
	Items []DrawableRemove
}

type DrawableSetTranslated struct {
	Items []DrawableTranslate
}

// DrawableEventsApplier consumes bulk drawable events and flushes pending changes.
type DrawableEventsApplier interface {
	ApplyAdded(items []DrawableAdd)
	ApplyRemoved(items []DrawableRemove)
	ApplyTranslated(items []DrawableTranslate)
	FlushTouched()
}

func convert(event platform.Event) Event {
	switch e := event.(type) {
	case platform.KeyPress:
		return KeyPress{Code: e.Code, Label: e.Label}
	case platform.KeyRelease:
		return KeyRelease{Code: e.Code, Label: e.Label}
	case platform.ButtonPress:
		return ButtonPress{Button: e.Button, X: e.X, Y: e.Y}
	case platform.ButtonRelease:
		return ButtonRelease{Button: e.Button, X: e.X, Y: e.Y}
	case platform.MotionNotify:
		return MotionNotify{X: e.X, Y: e.Y}
	case platform.EnterNotify:
		return EnterNotify{}
	case platform.LeaveNotify:
		return LeaveNotify{}
	case platform.CreateNotify:
		return CreateNotify{}
	case platform.DestroyNotify:
		return DestroyNotify{}
	case platform.ClientMessage:
		return ClientMessage{}
	case platform.MouseWheel:
		return MouseWheel{DeltaX: e.DeltaX, DeltaY: e.DeltaY, X: e.X, Y: e.Y}
	default:
		return UnexpectedEvent{}
	}
}
