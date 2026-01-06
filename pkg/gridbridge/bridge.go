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
	panesByID    map[uint64]*gfx.Pane
	managerByPID map[uint64]*grid.MultiBucketGridManager
	layerConfigs map[*gfx.Layer]grid.GridLevelConfig
	touched      map[*grid.BucketGridManager]struct{}
}

func NewBridge() *Bridge {
	return &Bridge{
		paneManagers: make(map[*gfx.Pane]*grid.MultiBucketGridManager),
		panesByID:    make(map[uint64]*gfx.Pane),
		managerByPID: make(map[uint64]*grid.MultiBucketGridManager),
		layerConfigs: make(map[*gfx.Layer]grid.GridLevelConfig),
		touched:      make(map[*grid.BucketGridManager]struct{}),
	}
}

func (b *Bridge) AttachPane(pane *gfx.Pane, manager *grid.MultiBucketGridManager) {
	if pane == nil || manager == nil {
		return
	}
	b.mu.Lock()
	b.paneManagers[pane] = manager
	b.panesByID[pane.ID] = pane
	b.managerByPID[pane.ID] = manager
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
	b.markTouched(manager)
}

func (b *Bridge) OnDrawableRemoved(layer *gfx.Layer, _ *gfx.Drawable, id uint64) {
	manager := b.layerManager(layer)
	if manager == nil || id == 0 {
		return
	}
	manager.QueueRemove(id)
	b.markTouched(manager)
}

func (b *Bridge) OnDrawableUpdated(layer *gfx.Layer, drawable *gfx.Drawable, id uint64, _, newAABB plane.AABB[uint32]) {
	manager := b.layerManager(layer)
	if manager == nil || drawable == nil || id == 0 {
		return
	}
	manager.QueueUpdate(id, newAABB, true)
	b.markTouched(manager)
}

func (b *Bridge) OnLayerDirtyRect(layer *gfx.Layer, rect spatial.AABB) {
	manager := b.layerManager(layer)
	if manager == nil {
		return
	}
	manager.QueueDirtyRect(rect)
	b.markTouched(manager)
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
	keyToLayer := make(map[uint64]*gfx.Layer, len(layers))
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		key := layer.ID()
		keyToLayer[key] = layer
	}
	keys := make([]uint64, 0, len(layers))
	for _, layer := range layers {
		if layer == nil {
			continue
		}
		keys = append(keys, layer.ID())
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
		layer := keyToLayer[gridLevelPlan.Key]
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

func (b *Bridge) PaneManagerByID(paneID uint64) *grid.MultiBucketGridManager {
	b.mu.RLock()
	manager := b.managerByPID[paneID]
	b.mu.RUnlock()
	return manager
}

func (b *Bridge) PaneByID(paneID uint64) *gfx.Pane {
	b.mu.RLock()
	pane := b.panesByID[paneID]
	b.mu.RUnlock()
	return pane
}

func (b *Bridge) LayerManagerByID(paneID, layerID uint64) *grid.BucketGridManager {
	manager := b.PaneManagerByID(paneID)
	if manager == nil {
		return nil
	}
	return manager.Manager(layerID)
}

func (b *Bridge) registerLayer(pane *gfx.Pane, layer *gfx.Layer) error {
	if pane == nil || layer == nil {
		return nil
	}
	manager := b.paneManager(pane)
	if manager == nil {
		return nil
	}
	key := layer.ID()
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
	return manager.Manager(layer.ID())
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
	return manager.Manager(layer.ID()) != nil
}

func (b *Bridge) ApplyAdded(items []gfx.DrawableAdd) {
	for _, item := range items {
		manager := b.LayerManagerByID(item.PaneID, item.LayerID)
		if manager == nil || item.DrawableID == 0 {
			continue
		}
		manager.QueueInsert(item.DrawableID, item.AABB)
		b.markTouched(manager)
	}
}

func (b *Bridge) ApplyRemoved(items []gfx.DrawableRemove) {
	for _, item := range items {
		manager := b.LayerManagerByID(item.PaneID, item.LayerID)
		if manager == nil || item.DrawableID == 0 {
			continue
		}
		manager.QueueRemove(item.DrawableID)
		b.markTouched(manager)
	}
}

func (b *Bridge) ApplyTranslated(items []gfx.DrawableTranslate) {
	for _, item := range items {
		manager := b.LayerManagerByID(item.PaneID, item.LayerID)
		if manager == nil || item.DrawableID == 0 {
			continue
		}
		manager.QueueUpdate(item.DrawableID, item.New, true)
		b.markTouched(manager)
	}
}

func (b *Bridge) FlushTouched() {
	b.mu.Lock()
	touched := b.touched
	if len(touched) > 0 {
		b.touched = make(map[*grid.BucketGridManager]struct{}, len(touched))
	}
	b.mu.Unlock()
	for manager := range touched {
		if manager != nil {
			manager.Flush()
		}
	}
}

func (b *Bridge) markTouched(manager *grid.BucketGridManager) {
	if manager == nil {
		return
	}
	b.mu.Lock()
	b.touched[manager] = struct{}{}
	b.mu.Unlock()
}
