package platform

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
type TimeoutEvent struct{}
