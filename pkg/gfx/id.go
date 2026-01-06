package gfx

import "sync/atomic"

var drawableIDSeq uint64

// NextDrawableID returns a globally unique drawable ID.
func NextDrawableID() uint64 {
	return atomic.AddUint64(&drawableIDSeq, 1)
}
