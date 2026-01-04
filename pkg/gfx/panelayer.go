package gfx

import (
	"image/color"
	"sync"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokx/pkg/grid"
)

type Layer struct {
	pane          *Pane
	mu            sync.RWMutex
	idx           int
	drawables     []*Drawable
	background    color.Color
	instanceData  []float32
	instanceCount int
	ranges        map[*Drawable]instanceRange
	pending       []instanceUpdate
	fullRebuild   bool
	needsRedraw   bool
	batchDepth    int
	batchedDirty  bool
	gridRegistry  *grid.MultiBucketGridManager
	gridManager   *grid.BucketGridManager
	gridConfig    grid.LayerConfig
	gridConfigSet bool
	idSeq         uint64
	idByDrawable  map[*Drawable]uint64
	drawableByID  map[uint64]*Drawable
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

func (l *Layer) Background() color.Color {
	l.mu.RLock()
	bg := l.background
	l.mu.RUnlock()
	return bg
}

func (l *Layer) SetBackground(color color.Color) {
	l.mu.Lock()
	l.background = color
	manager := l.gridManager
	pane := l.pane
	l.markDirtyLocked()
	l.mu.Unlock()
	if manager != nil && pane != nil && pane.viewport != nil {
		world := pane.viewport.WorldSize()
		manager.QueueDirtyRect(geom.NewAABBAt(geom.NewVec(0, 0), world.X, world.Y))
	}
}

func (l *Layer) AddDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}
	if drawable.layer != nil && drawable.layer != l {
		drawable.layer.RemoveDrawable(drawable)
	}

	l.mu.Lock()
	for _, existing := range l.drawables {
		if existing == drawable {
			l.markDirtyLocked()
			l.mu.Unlock()
			return
		}
	}
	l.drawables = append(l.drawables, drawable)
	drawable.attach(l)
	id := l.ensureDrawableIDLocked(drawable)
	manager := l.gridManager
	if manager != nil {
		l.markDirtyLocked()
		l.mu.Unlock()
		manager.QueueInsert(id, drawable.AABB)
		return
	}
	if l.fullRebuild {
		l.markDirtyLocked()
		l.mu.Unlock()
		return
	}
	if l.ranges == nil {
		l.ranges = make(map[*Drawable]instanceRange)
	}
	l.appendDrawableLocked(drawable)
	l.markDirtyLocked()
	l.mu.Unlock()
}

func (l *Layer) RemoveDrawable(drawable *Drawable) {
	if drawable == nil {
		return
	}

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		return
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
	drawable.detach()
	id := l.idByDrawable[drawable]
	delete(l.idByDrawable, drawable)
	delete(l.drawableByID, id)
	manager := l.gridManager
	if l.ranges != nil {
		delete(l.ranges, drawable)
	}
	l.markFullRebuildLocked()
	l.mu.Unlock()
	if manager != nil && id != 0 {
		manager.QueueRemove(id)
	}
}

func (l *Layer) Drawables() []*Drawable {
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
	if drawable.layer != l {
		mutate()
		return
	}

	l.mu.RLock()
	if drawable.layer != l {
		l.mu.RUnlock()
		mutate()
		return
	}
	manager := l.gridManager
	id := l.idByDrawable[drawable]
	l.mu.RUnlock()

	mutate()

	if manager != nil {
		l.mu.RLock()
		if drawable.layer != l {
			l.mu.RUnlock()
			return
		}
		id = l.idByDrawable[drawable]
		l.mu.RUnlock()
		if id != 0 {
			manager.QueueUpdate(id, drawable.AABB, true)
		}
		return
	}

	l.mu.Lock()
	if drawable.layer != l {
		l.mu.Unlock()
		return
	}
	if l.fullRebuild {
		l.markDirtyLocked()
		l.mu.Unlock()
		return
	}
	if l.ranges == nil {
		l.markFullRebuildLocked()
		l.mu.Unlock()
		return
	}
	rangeInfo, ok := l.ranges[drawable]
	if !ok {
		l.markFullRebuildLocked()
		l.mu.Unlock()
		return
	}
	newData := appendInstanceData(nil, drawable.AABB, drawable.Style)
	newCount := len(newData) / floatsPerInstance
	if newCount != rangeInfo.count {
		l.markFullRebuildLocked()
		l.mu.Unlock()
		return
	}
	if len(newData) > 0 {
		copy(l.instanceData[rangeInfo.start:rangeInfo.start+len(newData)], newData)
		l.pending = append(l.pending, instanceUpdate{
			offset: rangeInfo.start * 4,
			data:   newData,
		})
	}
	l.markDirtyLocked()
	l.mu.Unlock()
}

func (l *Layer) beginBatch() {
	l.mu.Lock()
	l.batchDepth++
	l.mu.Unlock()
}

func (l *Layer) endBatch() {
	l.mu.Lock()
	if l.batchDepth > 0 {
		l.batchDepth--
		if l.batchDepth == 0 && l.batchedDirty {
			l.needsRedraw = true
			l.batchedDirty = false
		}
	}
	l.mu.Unlock()
}

func (l *Layer) Batch(fn func()) {
	l.beginBatch()
	defer l.endBatch()
	fn()
}

func (l *Layer) markDirtyLocked() {
	if l.batchDepth > 0 {
		l.batchedDirty = true
		return
	}
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

func (l *Layer) consumeInstances(force bool) (fullData []float32, updates []instanceUpdate, count int, bg color.Color, dirty bool) {
	l.mu.Lock()
	if !l.needsRedraw && !force {
		bg = l.background
		count = l.instanceCount
		l.mu.Unlock()
		return nil, nil, count, bg, false
	}
	if force || l.fullRebuild || l.ranges == nil {
		l.instanceData = l.instanceData[:0]
		l.instanceCount = 0
		if l.ranges == nil {
			l.ranges = make(map[*Drawable]instanceRange, len(l.drawables))
		} else {
			for key := range l.ranges {
				delete(l.ranges, key)
			}
		}
		for _, drawable := range l.drawables {
			if drawable == nil {
				continue
			}
			data := appendInstanceData(nil, drawable.AABB, drawable.Style)
			start := len(l.instanceData)
			l.instanceData = append(l.instanceData, data...)
			instCount := len(data) / floatsPerInstance
			l.ranges[drawable] = instanceRange{start: start, count: instCount}
			l.instanceCount += instCount
		}
		fullData = append([]float32(nil), l.instanceData...)
		count = l.instanceCount
		bg = l.background
		l.fullRebuild = false
		l.pending = nil
		l.needsRedraw = false
		l.mu.Unlock()
		return fullData, nil, count, bg, true
	}

	if len(l.pending) > 0 {
		updates = append([]instanceUpdate(nil), l.pending...)
	}
	count = l.instanceCount
	bg = l.background
	l.pending = nil
	l.needsRedraw = false
	l.mu.Unlock()
	return nil, updates, count, bg, true
}

func (l *Layer) snapshotInstances() (data []float32, count int) {
	l.mu.RLock()
	data = append([]float32(nil), l.instanceData...)
	count = l.instanceCount
	l.mu.RUnlock()
	return data, count
}

func (l *Layer) SetGridConfig(cfg grid.LayerConfig) error {
	l.mu.Lock()
	l.gridConfig = cfg
	l.gridConfigSet = true
	registry := l.gridRegistry
	l.mu.Unlock()
	if registry == nil {
		return nil
	}
	manager, err := registry.Register(l, cfg)
	if err != nil {
		return err
	}
	l.attachGridManager(registry, manager)
	return nil
}

func (l *Layer) GridManager() *grid.BucketGridManager {
	l.mu.RLock()
	manager := l.gridManager
	l.mu.RUnlock()
	return manager
}

func (l *Layer) DrawableByID(id uint64) *Drawable {
	l.mu.RLock()
	drawable := l.drawableByID[id]
	l.mu.RUnlock()
	return drawable
}

func (l *Layer) attachGridManager(registry *grid.MultiBucketGridManager, manager *grid.BucketGridManager) {
	l.mu.Lock()
	l.gridRegistry = registry
	l.gridManager = manager
	if l.idByDrawable == nil {
		l.idByDrawable = make(map[*Drawable]uint64)
	}
	if l.drawableByID == nil {
		l.drawableByID = make(map[uint64]*Drawable)
	}
	drawables := append([]*Drawable(nil), l.drawables...)
	l.mu.Unlock()

	if manager == nil {
		return
	}
	for _, drawable := range drawables {
		if drawable == nil {
			continue
		}
		id := l.ensureDrawableID(drawable)
		manager.QueueInsert(id, drawable.AABB)
	}
}

func (l *Layer) ensureDrawableID(drawable *Drawable) uint64 {
	l.mu.Lock()
	id := l.ensureDrawableIDLocked(drawable)
	l.mu.Unlock()
	return id
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
	l.idSeq++
	id := l.idSeq
	l.idByDrawable[drawable] = id
	l.drawableByID[id] = drawable
	return id
}
