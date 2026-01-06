package gfx

import (
	"image/color"
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
)

type Layer struct {
	pane          *Pane
	mu            sync.RWMutex
	opsMu         sync.Mutex
	idx           int
	drawables     []*Drawable
	background    color.Color
	instanceData  []float32
	instanceCount int
	ranges        map[*Drawable]instanceRange
	pending       []instanceUpdate
	fullRebuild   bool
	needsRedraw   bool
	ops           []layerOp
	batchedOps    []layerOp
	batchDepth    int
	observer      LayerObserver
	idSeq         uint64
	idByDrawable  map[*Drawable]uint64
	drawableByID  map[uint64]*Drawable
}

type layerOpKind uint8

const (
	opAdd layerOpKind = iota
	opRemove
	opModify
)

type layerOp struct {
	kind     layerOpKind
	drawable *Drawable
	mutate   func()
}

type layerEventKind uint8

const (
	eventAdded layerEventKind = iota
	eventRemoved
	eventUpdated
)

type layerEvent struct {
	kind     layerEventKind
	drawable *Drawable
	id       uint64
	oldAABB  plane.AABB[uint32]
	newAABB  plane.AABB[uint32]
}

type LayerTx struct {
	layer *Layer
	ops   []layerOp
}

func NewLayer(pane *Pane) *Layer {
	layer := &Layer{
		pane:        pane,
		drawables:   make([]*Drawable, 0),
		needsRedraw: true,
	}
	return layer
}

func NewLayerDefault(pane *Pane) *Layer {
	return NewLayer(pane)
}

func (l *Layer) GetPane() *Pane {
	return l.pane
}

func (l *Layer) ID() uint64 {
	return uint64(l.idx)
}

func (l *Layer) Background() color.Color {
	l.mu.RLock()
	bg := l.background
	l.mu.RUnlock()
	return bg
}

func (l *Layer) SetBackground(color color.Color) {
	l.mu.Lock()
	l.background = color
	observer := l.observer
	pane := l.pane
	l.markDirtyLocked()
	l.mu.Unlock()
	if observer != nil && pane != nil && pane.viewport != nil {
		world := pane.viewport.WorldSize()
		observer.OnLayerDirtyRect(l, geom.NewAABBAt(geom.NewVec[uint32](0, 0), world.X, world.Y))
	}
}

func (l *Layer) AddDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != l {
		drawable.layer.RemoveDrawable(drawable)
	}
	l.enqueueOp(layerOp{kind: opAdd, drawable: drawable})
}

func (l *Layer) RemoveDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	l.enqueueOp(layerOp{kind: opRemove, drawable: drawable})
}

func (l *Layer) Drawables() []*Drawable {
	l.ProcessOps()
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Drawable, len(l.drawables))
	copy(out, l.drawables)
	return out
}

func (l *Layer) ModifyDrawable(drawable *Drawable, mutate func()) {
	if drawable == nil || mutate == nil {
		return
	}
	l.enqueueOp(layerOp{kind: opModify, drawable: drawable, mutate: mutate})
}

// Update batches drawable operations for a single ProcessOps pass.
func (l *Layer) Update(fn func(tx *LayerTx)) {
	if fn == nil {
		return
	}
	tx := LayerTx{layer: l}
	fn(&tx)
	if len(tx.ops) == 0 {
		return
	}
	l.enqueueOps(tx.ops)
}

func (tx *LayerTx) AddDrawable(drawable *Drawable) {
	if tx == nil || tx.layer == nil || drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != tx.layer {
		drawable.layer.RemoveDrawable(drawable)
	}
	tx.ops = append(tx.ops, layerOp{kind: opAdd, drawable: drawable})
}

func (tx *LayerTx) RemoveDrawable(drawable *Drawable) {
	if tx == nil || tx.layer == nil || drawable == nil {
		return
	}
	tx.ops = append(tx.ops, layerOp{kind: opRemove, drawable: drawable})
}

func (tx *LayerTx) ModifyDrawable(drawable *Drawable, mutate func()) {
	if tx == nil || tx.layer == nil || drawable == nil || mutate == nil {
		return
	}
	tx.ops = append(tx.ops, layerOp{kind: opModify, drawable: drawable, mutate: mutate})
}

// ProcessOps applies queued layer operations and dispatches observer events.
func (l *Layer) ProcessOps() {
	ops := l.drainOps()
	if len(ops) == 0 {
		return
	}
	var events []layerEvent
	l.mu.Lock()
	for _, op := range ops {
		events = l.applyOpLocked(op, events)
	}
	observer := l.observer
	l.mu.Unlock()
	if observer == nil || len(events) == 0 {
		return
	}
	for _, event := range events {
		switch event.kind {
		case eventAdded:
			observer.OnDrawableAdded(l, event.drawable, event.id)
		case eventRemoved:
			observer.OnDrawableRemoved(l, event.drawable, event.id)
		case eventUpdated:
			observer.OnDrawableUpdated(l, event.drawable, event.id, event.oldAABB, event.newAABB)
		}
	}
}

func (l *Layer) enqueueOp(op layerOp) {
	l.opsMu.Lock()
	if l.batchDepth > 0 {
		l.batchedOps = append(l.batchedOps, op)
		l.opsMu.Unlock()
		return
	}
	l.ops = append(l.ops, op)
	l.opsMu.Unlock()
}

func (l *Layer) enqueueOps(ops []layerOp) {
	if len(ops) == 0 {
		return
	}
	l.opsMu.Lock()
	if l.batchDepth > 0 {
		l.batchedOps = append(l.batchedOps, ops...)
		l.opsMu.Unlock()
		return
	}
	l.ops = append(l.ops, ops...)
	l.opsMu.Unlock()
}

func (l *Layer) drainOps() []layerOp {
	l.opsMu.Lock()
	if len(l.ops) == 0 {
		l.opsMu.Unlock()
		return nil
	}
	ops := l.ops
	l.ops = nil
	l.opsMu.Unlock()
	return ops
}

func (l *Layer) applyOpLocked(op layerOp, events []layerEvent) []layerEvent {
	switch op.kind {
	case opAdd:
		return l.applyAddLocked(op.drawable, events)
	case opRemove:
		return l.applyRemoveLocked(op.drawable, events)
	case opModify:
		return l.applyModifyLocked(op.drawable, op.mutate, events)
	default:
		return events
	}
}

func (l *Layer) applyAddLocked(drawable *Drawable, events []layerEvent) []layerEvent {
	if drawable == nil {
		return events
	}
	if l.containsDrawableLocked(drawable) {
		l.markDirtyLocked()
		return events
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	id := l.ensureDrawableIDLocked(drawable)
	if l.fullRebuild {
		l.markDirtyLocked()
		if l.observer != nil && id != 0 {
			events = append(events, layerEvent{kind: eventAdded, drawable: drawable, id: id})
		}
		return events
	}
	if l.ranges == nil {
		l.ranges = make(map[*Drawable]instanceRange)
	}
	l.appendDrawableLocked(drawable)
	l.markDirtyLocked()
	if l.observer != nil && id != 0 {
		events = append(events, layerEvent{kind: eventAdded, drawable: drawable, id: id})
	}
	return events
}

func (l *Layer) applyRemoveLocked(drawable *Drawable, events []layerEvent) []layerEvent {
	if drawable == nil {
		return events
	}
	if !l.containsDrawableLocked(drawable) && l.idByDrawable[drawable] == 0 {
		return events
	}
	idx := -1
	for i, existing := range l.drawables {
		if existing == drawable {
			idx = i
			break
		}
	}
	if idx >= 0 {
		l.drawables = append(l.drawables[:idx], l.drawables[idx+1:]...)
	}
	if drawable.layer == l {
		drawable.detach()
	}
	id := l.idByDrawable[drawable]
	delete(l.idByDrawable, drawable)
	delete(l.drawableByID, id)
	if l.ranges != nil {
		delete(l.ranges, drawable)
	}
	l.markFullRebuildLocked()
	if l.observer != nil && id != 0 {
		events = append(events, layerEvent{kind: eventRemoved, drawable: drawable, id: id})
	}
	return events
}

func (l *Layer) applyModifyLocked(drawable *Drawable, mutate func(), events []layerEvent) []layerEvent {
	if drawable == nil || mutate == nil {
		return events
	}
	if drawable.layer != l {
		return events
	}
	id := l.idByDrawable[drawable]
	oldAABB := drawable.AABB
	mutate()
	newAABB := drawable.AABB
	if l.observer != nil && id != 0 {
		events = append(events, layerEvent{
			kind:     eventUpdated,
			drawable: drawable,
			id:       id,
			oldAABB:  oldAABB,
			newAABB:  newAABB,
		})
	}
	if l.fullRebuild {
		l.markDirtyLocked()
		return events
	}
	if l.ranges == nil {
		l.markFullRebuildLocked()
		return events
	}
	rangeInfo, ok := l.ranges[drawable]
	if !ok {
		l.markFullRebuildLocked()
		return events
	}
	newData := appendInstanceData(nil, drawable.AABB, drawable.Style)
	newCount := len(newData) / floatsPerInstance
	if newCount != rangeInfo.count {
		l.markFullRebuildLocked()
		return events
	}
	if len(newData) > 0 {
		copy(l.instanceData[rangeInfo.start:rangeInfo.start+len(newData)], newData)
		l.pending = append(l.pending, instanceUpdate{
			offset: rangeInfo.start * 4,
			data:   newData,
		})
	}
	l.markDirtyLocked()
	return events
}

func (l *Layer) containsDrawableLocked(drawable *Drawable) bool {
	for _, existing := range l.drawables {
		if existing == drawable {
			return true
		}
	}
	return false
}

func (l *Layer) beginBatch() {
	l.opsMu.Lock()
	l.batchDepth++
	l.opsMu.Unlock()
}

func (l *Layer) endBatch() {
	l.opsMu.Lock()
	if l.batchDepth > 0 {
		l.batchDepth--
		if l.batchDepth == 0 && len(l.batchedOps) > 0 {
			l.ops = append(l.ops, l.batchedOps...)
			l.batchedOps = nil
		}
	}
	l.opsMu.Unlock()
}

func (l *Layer) Batch(fn func()) {
	l.beginBatch()
	defer l.endBatch()
	fn()
}

func (l *Layer) markDirtyLocked() {
	l.needsRedraw = true
}

func (l *Layer) markFullRebuildLocked() {
	l.fullRebuild = true
	l.pending = nil
	l.markDirtyLocked()
}

func (l *Layer) appendDrawableLocked(drawable *Drawable) {
	data := appendInstanceData(nil, drawable.AABB, drawable.Style)
	start := len(l.instanceData)
	l.instanceData = append(l.instanceData, data...)
	count := len(data) / floatsPerInstance
	l.ranges[drawable] = instanceRange{start: start, count: count}
	l.instanceCount += count
	if len(data) > 0 {
		l.pending = append(l.pending, instanceUpdate{
			offset: start * 4,
			data:   data,
		})
	}
}

func (l *Layer) SetObserver(observer LayerObserver) {
	l.mu.Lock()
	l.observer = observer
	l.mu.Unlock()
}

func (l *Layer) DrawableID(drawable *Drawable) (uint64, bool) {
	l.mu.RLock()
	id := l.idByDrawable[drawable]
	l.mu.RUnlock()
	return id, id != 0
}

func (l *Layer) DrawableByID(id uint64) *Drawable {
	l.mu.RLock()
	drawable := l.drawableByID[id]
	l.mu.RUnlock()
	return drawable
}

func (l *Layer) ensureDrawableIDLocked(drawable *Drawable) uint64 {
	if drawable == nil {
		return 0
	}
	if l.idByDrawable == nil {
		l.idByDrawable = make(map[*Drawable]uint64)
	}
	if l.drawableByID == nil {
		l.drawableByID = make(map[uint64]*Drawable)
	}
	if id := l.idByDrawable[drawable]; id != 0 {
		return id
	}
	id := drawable.ID
	if id == 0 {
		id = NextDrawableID()
		drawable.ID = id
	}
	l.idByDrawable[drawable] = id
	l.drawableByID[id] = drawable
	return id
}
