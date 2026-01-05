package gridbridge

import (
	"fmt"
	"sync"

	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
	"github.com/kjkrol/gokx/pkg/gfx"
	"github.com/kjkrol/gokx/pkg/grid"
)

type Bridge struct {
	mu           sync.RWMutex
	paneManagers map[*gfx.Pane]*grid.MultiBucketGridManager
	layerConfigs map[*gfx.Layer]grid.GridLevelConfig
	layerKeys    map[*gfx.Layer]uint64
	keyLayers    map[uint64]*gfx.Layer
	keySeq       uint64
}

func NewBridge() *Bridge {
	return &Bridge{
		paneManagers: make(map[*gfx.Pane]*grid.MultiBucketGridManager),
		layerConfigs: make(map[*gfx.Layer]grid.GridLevelConfig),
		layerKeys:    make(map[*gfx.Layer]uint64),
		keyLayers:    make(map[uint64]*gfx.Layer),
	}
}

func (b *Bridge) AttachPane(pane *gfx.Pane, manager *grid.MultiBucketGridManager) {
	if pane == nil || manager == nil {
		return
	}
	b.mu.Lock()
	b.paneManagers[pane] = manager
	b.mu.Unlock()

	pane.SetLayerObserver(b)
	pane.SetLayerCreatedHandler(func(layer *gfx.Layer) {
		_ = b.registerLayer(pane, layer)
	})

	for _, layer := range pane.Layers() {
		_ = b.registerLayer(pane, layer)
	}
}

func (b *Bridge) SetLayerConfig(layer *gfx.Layer, cfg grid.GridLevelConfig) error {
	if layer == nil {
		return fmt.Errorf("layer is nil")
	}
	if b.isLayerRegistered(layer) {
		return fmt.Errorf("layer already registered")
	}
	b.mu.Lock()
	b.layerConfigs[layer] = cfg
	b.mu.Unlock()
	return nil
}

func (b *Bridge) OnDrawableAdded(layer *gfx.Layer, drawable *gfx.Drawable, id uint64) {
	manager := b.layerManager(layer)
	if manager == nil || drawable == nil || id == 0 {
		return
	}
	manager.QueueInsert(id, drawable.AABB)
}

func (b *Bridge) OnDrawableRemoved(layer *gfx.Layer, _ *gfx.Drawable, id uint64) {
	manager := b.layerManager(layer)
	if manager == nil || id == 0 {
		return
	}
	manager.QueueRemove(id)
}

func (b *Bridge) OnDrawableUpdated(layer *gfx.Layer, drawable *gfx.Drawable, id uint64, _, newAABB plane.AABB[uint32]) {
	manager := b.layerManager(layer)
	if manager == nil || drawable == nil || id == 0 {
		return
	}
	manager.QueueUpdate(id, newAABB, true)
}

func (b *Bridge) OnLayerDirtyRect(layer *gfx.Layer, rect spatial.AABB) {
	manager := b.layerManager(layer)
	if manager == nil {
		return
	}
	manager.QueueDirtyRect(rect)
}

func (b *Bridge) BuildFrame(pane *gfx.Pane, viewRect spatial.AABB, viewChanged bool, layers []*gfx.Layer) gfx.FramePlan {
	out := gfx.FramePlan{
		ViewRect:    viewRect,
		ViewChanged: viewChanged,
	}
	manager := b.paneManager(pane)
	if manager == nil {
		return out
	}
	keys := make([]uint64, 0, len(layers))
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		key := b.layerKey(layer)
		if key == 0 {
			continue
		}
		keys = append(keys, key)
	}
	frame := manager.BuildFrame(viewRect, viewChanged, keys)
	out.ViewRect = frame.ViewRect
	out.ViewChanged = frame.ViewChanged
	if len(frame.CompositeRects) > 0 {
		out.CompositeRects = append([]spatial.AABB(nil), frame.CompositeRects...)
	}
	if len(frame.GridLevels) == 0 {
		return out
	}
	out.Layers = make([]gfx.LayerPlan, 0, len(frame.GridLevels))
	for _, gridLevelPlan := range frame.GridLevels {
		layer := b.layerByKey(gridLevelPlan.Key)
		if layer == nil {
			continue
		}
		buckets := gridLevelPlan.Buckets
		if len(buckets) > 0 {
			buckets = append([]spatial.AABB(nil), buckets...)
		}
		out.Layers = append(out.Layers, gfx.LayerPlan{
			Layer:     layer,
			CacheRect: gridLevelPlan.CacheRect,
			Buckets:   buckets,
		})
	}
	return out
}

func (b *Bridge) ConsumeBucketDeltas(layer *gfx.Layer) []gfx.BucketDelta {
	manager := b.layerManager(layer)
	if manager == nil {
		return nil
	}
	deltas := manager.ConsumeBucketDeltas()
	if len(deltas) == 0 {
		return nil
	}
	out := make([]gfx.BucketDelta, 0, len(deltas))
	for _, delta := range deltas {
		out = append(out, gfx.BucketDelta{
			Bucket:  delta.Bucket,
			Added:   delta.Added,
			Removed: delta.Removed,
			Updated: delta.Updated,
		})
	}
	return out
}

func (b *Bridge) EntryAABB(layer *gfx.Layer, entryID uint64) (spatial.AABB, bool) {
	manager := b.layerManager(layer)
	if manager == nil {
		return spatial.AABB{}, false
	}
	return manager.EntryAABB(entryID)
}

func (b *Bridge) paneManager(pane *gfx.Pane) *grid.MultiBucketGridManager {
	b.mu.RLock()
	manager := b.paneManagers[pane]
	b.mu.RUnlock()
	return manager
}

func (b *Bridge) registerLayer(pane *gfx.Pane, layer *gfx.Layer) error {
	if pane == nil || layer == nil {
		return nil
	}
	manager := b.paneManager(pane)
	if manager == nil {
		return nil
	}
	key := b.layerKey(layer)
	if key == 0 {
		return nil
	}
	if existing := manager.Manager(key); existing != nil {
		return nil
	}
	b.mu.RLock()
	cfg, hasCfg := b.layerConfigs[layer]
	b.mu.RUnlock()
	if !hasCfg {
		cfg = grid.GridLevelConfig{}
	}
	gridMgr, err := manager.Register(key, cfg)
	if err != nil {
		return err
	}

	for _, drawable := range layer.Drawables() {
		if drawable == nil {
			continue
		}
		if id, ok := layer.DrawableID(drawable); ok {
			gridMgr.QueueInsert(id, drawable.AABB)
		}
	}
	return nil
}

func (b *Bridge) layerKey(layer *gfx.Layer) uint64 {
	if layer == nil {
		return 0
	}
	b.mu.RLock()
	key := b.layerKeys[layer]
	b.mu.RUnlock()
	if key != 0 {
		return key
	}
	b.mu.Lock()
	key = b.layerKeys[layer]
	if key == 0 {
		b.keySeq++
		key = b.keySeq
		b.layerKeys[layer] = key
		b.keyLayers[key] = layer
	}
	b.mu.Unlock()
	return key
}

func (b *Bridge) layerByKey(key uint64) *gfx.Layer {
	if key == 0 {
		return nil
	}
	b.mu.RLock()
	layer := b.keyLayers[key]
	b.mu.RUnlock()
	return layer
}

func (b *Bridge) layerManager(layer *gfx.Layer) *grid.BucketGridManager {
	if layer == nil {
		return nil
	}
	pane := layer.GetPane()
	if pane == nil {
		return nil
	}
	manager := b.paneManager(pane)
	if manager == nil {
		return nil
	}
	key := b.layerKey(layer)
	if key == 0 {
		return nil
	}
	return manager.Manager(key)
}

func (b *Bridge) isLayerRegistered(layer *gfx.Layer) bool {
	if layer == nil {
		return false
	}
	pane := layer.GetPane()
	if pane == nil {
		return false
	}
	manager := b.paneManager(pane)
	if manager == nil {
		return false
	}
	b.mu.RLock()
	key := b.layerKeys[layer]
	b.mu.RUnlock()
	if key == 0 {
		return false
	}
	return manager.Manager(key) != nil
}
