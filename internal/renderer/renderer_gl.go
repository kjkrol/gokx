//go:build !js

package renderer

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokx/pkg/gfx"
)

type renderer struct {
	shaderSource string
	initialized  bool

	colorProgram     uint32
	compositeProgram uint32
	quadVbo          uint32
	compositeVao     uint32

	colorViewportUniform     int32
	colorOriginUniform       int32
	colorWorldUniform        int32
	compositeViewportUniform int32
	compositeRectUniform     int32
	compositeTexUniform      int32
	compositeTexRectUniform  int32

	layerStates map[*gfx.Layer]*layerState
	paneViews   map[*gfx.Pane]uint64
	paneStates  map[*gfx.Pane]*paneState
	source      gfx.FrameSource
}

type layerState struct {
	texture uint32
	fbo     uint32
	width   int
	height  int
	buckets map[geom.AABB[uint32]]*bucketState
}

type bucketState struct {
	vao         uint32
	instanceVbo uint32
	instanceCap int
	entries     []uint64
	index       map[uint64]int
	data        []float32
}

type paneState struct {
	texture uint32
	fbo     uint32
	width   int
	height  int
}

func newRenderer(_ *gfx.Window, conf RendererConfig, source gfx.FrameSource) *renderer {
	return &renderer{
		shaderSource: conf.ShaderSource,
		layerStates:  make(map[*gfx.Layer]*layerState),
		paneViews:    make(map[*gfx.Pane]uint64),
		paneStates:   make(map[*gfx.Pane]*paneState),
		source:       source,
	}
}

func (r *renderer) Render(w *gfx.Window) {
	if w == nil || r.source == nil {
		return
	}
	defaultPane := w.GetDefaultPane()
	if defaultPane == nil || defaultPane.Config == nil {
		return
	}
	r.ensureInit()

	width, height := w.Size()
	if width <= 0 || height <= 0 {
		return
	}

	panes := w.Panes()
	type paneFrame struct {
		pane       *gfx.Pane
		layers     []*gfx.Layer
		plan       gfx.FramePlan
		layerPlans map[*gfx.Layer]gfx.LayerPlan
		worldSize  geom.Vec[uint32]
	}
	paneFrames := make([]paneFrame, 0, len(panes))

	for _, pane := range panes {
		if pane == nil || pane.Config == nil {
			continue
		}
		view := pane.Viewport()
		if view == nil {
			continue
		}
		layers := pane.Layers()
		viewVersion := view.Version()
		prevVersion, ok := r.paneViews[pane]
		viewChanged := !ok || prevVersion != viewVersion
		if viewChanged {
			r.paneViews[pane] = viewVersion
		}
		frame := r.source.BuildFrame(pane, view.Rect(), viewChanged, layers)
		layerPlans := make(map[*gfx.Layer]gfx.LayerPlan, len(frame.Layers))
		for _, layerPlan := range frame.Layers {
			if layerPlan.Layer == nil {
				continue
			}
			layerPlans[layerPlan.Layer] = layerPlan
		}
		worldSize := view.WorldSize()
		viewSize := view.Size()
		if !view.Wrap() || viewSize.X >= worldSize.X || viewSize.Y >= worldSize.Y {
			worldSize = geom.NewVec[uint32](0, 0)
		}
		for _, layer := range layers {
			plan, ok := layerPlans[layer]
			if !ok {
				continue
			}
			r.renderLayerBuckets(layer, plan, worldSize)
		}
		paneFrames = append(paneFrames, paneFrame{
			pane:       pane,
			layers:     layers,
			plan:       frame,
			layerPlans: layerPlans,
			worldSize:  worldSize,
		})
	}

	for _, frame := range paneFrames {
		if len(frame.plan.CompositeRects) == 0 {
			continue
		}
		r.compositePane(frame.pane, frame.layers, frame.layerPlans, frame.plan, frame.worldSize)
	}

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.Viewport(0, 0, int32(width), int32(height))
	gl.ClearColor(0, 0, 0, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(r.compositeProgram)
	gl.BindVertexArray(r.compositeVao)
	gl.Uniform2f(r.compositeViewportUniform, float32(width), float32(height))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Uniform1i(r.compositeTexUniform, 0)

	for _, frame := range paneFrames {
		state := r.paneStates[frame.pane]
		if state == nil || state.texture == 0 {
			continue
		}
		pane := frame.pane
		if pane == nil || pane.Config == nil {
			continue
		}
		x0 := float32(pane.Config.OffsetX)
		y0 := float32(pane.Config.OffsetY)
		x1 := float32(pane.Config.OffsetX + pane.Config.Width)
		y1 := float32(pane.Config.OffsetY + pane.Config.Height)
		gl.Uniform4f(r.compositeRectUniform, x0, y0, x1, y1)
		gl.Uniform4f(r.compositeTexRectUniform, 0, 0, 1, 1)
		gl.BindTexture(gl.TEXTURE_2D, state.texture)
		gl.DrawArrays(gl.TRIANGLES, 0, 6)
	}
}

func (r *renderer) Close() {
	if !r.initialized {
		return
	}
	for _, state := range r.layerStates {
		if state == nil {
			continue
		}
		if state.texture != 0 {
			gl.DeleteTextures(1, &state.texture)
		}
		if state.fbo != 0 {
			gl.DeleteFramebuffers(1, &state.fbo)
		}
		for _, bucket := range state.buckets {
			if bucket == nil {
				continue
			}
			if bucket.instanceVbo != 0 {
				gl.DeleteBuffers(1, &bucket.instanceVbo)
			}
			if bucket.vao != 0 {
				gl.DeleteVertexArrays(1, &bucket.vao)
			}
		}
	}
	for _, state := range r.paneStates {
		if state == nil {
			continue
		}
		if state.texture != 0 {
			gl.DeleteTextures(1, &state.texture)
		}
		if state.fbo != 0 {
			gl.DeleteFramebuffers(1, &state.fbo)
		}
	}
	if r.quadVbo != 0 {
		gl.DeleteBuffers(1, &r.quadVbo)
	}
	if r.compositeVao != 0 {
		gl.DeleteVertexArrays(1, &r.compositeVao)
	}
	if r.colorProgram != 0 {
		gl.DeleteProgram(r.colorProgram)
	}
	if r.compositeProgram != 0 {
		gl.DeleteProgram(r.compositeProgram)
	}
	r.layerStates = nil
	r.paneStates = nil
	r.initialized = false
}

func (r *renderer) ensureInit() {
	if r.initialized {
		return
	}
	if err := gl.Init(); err != nil {
		panic(fmt.Sprintf("gl.Init error: %v", err))
	}

	r.colorProgram = r.buildProgram("PASS_COLOR")
	r.compositeProgram = r.buildProgram("PASS_COMPOSITE")

	r.colorViewportUniform = gl.GetUniformLocation(r.colorProgram, gl.Str("uViewport\x00"))
	r.colorOriginUniform = gl.GetUniformLocation(r.colorProgram, gl.Str("uOrigin\x00"))
	r.colorWorldUniform = gl.GetUniformLocation(r.colorProgram, gl.Str("uWorld\x00"))
	r.compositeViewportUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uViewport\x00"))
	r.compositeRectUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uRect\x00"))
	r.compositeTexUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uTex\x00"))
	r.compositeTexRectUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uTexRect\x00"))

	r.initQuad()

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.Disable(gl.DEPTH_TEST)

	r.initialized = true
}

func (r *renderer) initQuad() {
	quad := []float32{
		0, 0,
		1, 0,
		1, 1,
		0, 0,
		1, 1,
		0, 1,
	}
	gl.GenBuffers(1, &r.quadVbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(quad)*4, gl.Ptr(quad), gl.STATIC_DRAW)

	gl.GenVertexArrays(1, &r.compositeVao)
	gl.BindVertexArray(r.compositeVao)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, gl.PtrOffset(0))
}

func (r *renderer) renderLayerBuckets(layer *gfx.Layer, plan gfx.LayerPlan, worldSize geom.Vec[uint32]) {
	if layer == nil || r.source == nil {
		return
	}
	cacheRect := plan.CacheRect
	if cacheRect.BottomRight.X <= cacheRect.TopLeft.X || cacheRect.BottomRight.Y <= cacheRect.TopLeft.Y {
		return
	}
	cacheWidth := int(cacheRect.BottomRight.X - cacheRect.TopLeft.X)
	cacheHeight := int(cacheRect.BottomRight.Y - cacheRect.TopLeft.Y)
	if cacheWidth <= 0 || cacheHeight <= 0 {
		return
	}
	state := r.ensureLayerState(layer, cacheWidth, cacheHeight)
	r.syncBucketStates(layer, state)
	if plan.BucketRect == nil || len(plan.BucketIndices) == 0 {
		return
	}
	bgColor := colorToFloat(layer.Background())

	gl.BindFramebuffer(gl.FRAMEBUFFER, state.fbo)
	gl.Viewport(0, 0, int32(state.width), int32(state.height))
	gl.UseProgram(r.colorProgram)
	gl.Uniform2f(r.colorViewportUniform, float32(state.width), float32(state.height))
	gl.Uniform2f(r.colorOriginUniform, float32(cacheRect.TopLeft.X), float32(cacheRect.TopLeft.Y))
	gl.Uniform2f(r.colorWorldUniform, float32(worldSize.X), float32(worldSize.Y))
	gl.Enable(gl.SCISSOR_TEST)

	for _, idx := range plan.BucketIndices {
		bucket := plan.BucketRect(idx)
		scissor := bucketScissor(bucket, cacheRect, worldSize, state.width, state.height)
		if scissor.W <= 0 || scissor.H <= 0 {
			continue
		}
		gl.Scissor(int32(scissor.X), int32(scissor.Y), int32(scissor.W), int32(scissor.H))
		gl.ClearColor(bgColor[0], bgColor[1], bgColor[2], bgColor[3])
		gl.Clear(gl.COLOR_BUFFER_BIT)

		bucketState := state.buckets[bucket]
		if bucketState == nil || len(bucketState.entries) == 0 {
			continue
		}
		gl.BindVertexArray(bucketState.vao)
		gl.DrawArraysInstanced(gl.TRIANGLES, 0, 6, int32(len(bucketState.entries)))
	}

	gl.Disable(gl.SCISSOR_TEST)
}

func (r *renderer) compositePane(pane *gfx.Pane, layers []*gfx.Layer, layerPlans map[*gfx.Layer]gfx.LayerPlan, frame gfx.FramePlan, worldSize geom.Vec[uint32]) {
	if pane == nil || pane.Config == nil {
		return
	}
	state := r.ensurePaneState(pane, pane.Config.Width, pane.Config.Height)
	if state == nil || state.texture == 0 {
		return
	}
	if len(frame.CompositeRects) == 0 {
		return
	}

	gl.BindFramebuffer(gl.FRAMEBUFFER, state.fbo)
	gl.Viewport(0, 0, int32(state.width), int32(state.height))
	gl.UseProgram(r.compositeProgram)
	gl.BindVertexArray(r.compositeVao)
	gl.Uniform2f(r.compositeViewportUniform, float32(state.width), float32(state.height))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Uniform1i(r.compositeTexUniform, 0)
	gl.Uniform4f(r.compositeRectUniform, 0, 0, float32(state.width), float32(state.height))

	gl.Enable(gl.SCISSOR_TEST)
	for _, rect := range frame.CompositeRects {
		scissor := paneScissor(rect, state.height)
		if scissor.W <= 0 || scissor.H <= 0 {
			continue
		}
		gl.Scissor(int32(scissor.X), int32(scissor.Y), int32(scissor.W), int32(scissor.H))
		gl.ClearColor(0, 0, 0, 0)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		for _, layer := range layers {
			plan, ok := layerPlans[layer]
			if !ok {
				continue
			}
			layerState := r.layerStates[layer]
			if layerState == nil || layerState.texture == 0 {
				continue
			}
			uv := texRect(frame.ViewRect, plan.CacheRect, worldSize)
			gl.Uniform4f(r.compositeTexRectUniform, uv[0], uv[1], uv[2], uv[3])
			gl.BindTexture(gl.TEXTURE_2D, layerState.texture)
			gl.DrawArrays(gl.TRIANGLES, 0, 6)
		}
	}
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *renderer) ensurePaneState(pane *gfx.Pane, width, height int) *paneState {
	state := r.paneStates[pane]
	if state == nil {
		state = &paneState{}
		gl.GenTextures(1, &state.texture)
		gl.GenFramebuffers(1, &state.fbo)
		r.paneStates[pane] = state
	}
	if state.width != width || state.height != height {
		state.width = width
		state.height = height
		r.resizePaneTexture(state)
	}
	return state
}

func (r *renderer) resizePaneTexture(state *paneState) {
	if state.width <= 0 || state.height <= 0 {
		return
	}
	gl.BindTexture(gl.TEXTURE_2D, state.texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(state.width), int32(state.height), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)

	gl.BindFramebuffer(gl.FRAMEBUFFER, state.fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, state.texture, 0)
}

type scissorRect struct {
	X int
	Y int
	W int
	H int
}

func bucketScissor(bucket, cacheRect geom.AABB[uint32], worldSize geom.Vec[uint32], cacheWidth, cacheHeight int) scissorRect {
	origin := cacheRect.TopLeft
	x0 := unwrapCoord(bucket.TopLeft.X, origin.X, worldSize.X)
	x1 := unwrapCoord(bucket.BottomRight.X, origin.X, worldSize.X)
	y0 := unwrapCoord(bucket.TopLeft.Y, origin.Y, worldSize.Y)
	y1 := unwrapCoord(bucket.BottomRight.Y, origin.Y, worldSize.Y)
	if worldSize.X > 0 && x1 < x0 {
		x1 += worldSize.X
	}
	if worldSize.Y > 0 && y1 < y0 {
		y1 += worldSize.Y
	}
	localX0 := int(x0 - origin.X)
	localX1 := int(x1 - origin.X)
	localY0 := int(y0 - origin.Y)
	localY1 := int(y1 - origin.Y)
	return scissorRect{
		X: localX0,
		Y: cacheHeight - localY1,
		W: localX1 - localX0,
		H: localY1 - localY0,
	}
}

func paneScissor(rect geom.AABB[uint32], paneHeight int) scissorRect {
	return scissorRect{
		X: int(rect.TopLeft.X),
		Y: paneHeight - int(rect.BottomRight.Y),
		W: int(rect.BottomRight.X - rect.TopLeft.X),
		H: int(rect.BottomRight.Y - rect.TopLeft.Y),
	}
}

func unwrapCoord(value, origin, worldSize uint32) uint32 {
	if worldSize > 0 && value < origin {
		return value + worldSize
	}
	return value
}

func texRect(viewRect, cacheRect geom.AABB[uint32], worldSize geom.Vec[uint32]) [4]float32 {
	viewW := viewRect.BottomRight.X - viewRect.TopLeft.X
	viewH := viewRect.BottomRight.Y - viewRect.TopLeft.Y
	cacheW := cacheRect.BottomRight.X - cacheRect.TopLeft.X
	cacheH := cacheRect.BottomRight.Y - cacheRect.TopLeft.Y
	if cacheW == 0 || cacheH == 0 {
		return [4]float32{0, 0, 1, 1}
	}
	offsetX := viewRect.TopLeft.X
	offsetY := viewRect.TopLeft.Y
	if viewRect.TopLeft.X >= cacheRect.TopLeft.X {
		offsetX = viewRect.TopLeft.X - cacheRect.TopLeft.X
	} else if worldSize.X > 0 {
		offsetX = viewRect.TopLeft.X + worldSize.X - cacheRect.TopLeft.X
	}
	if viewRect.TopLeft.Y >= cacheRect.TopLeft.Y {
		offsetY = viewRect.TopLeft.Y - cacheRect.TopLeft.Y
	} else if worldSize.Y > 0 {
		offsetY = viewRect.TopLeft.Y + worldSize.Y - cacheRect.TopLeft.Y
	}
	return [4]float32{
		float32(offsetX) / float32(cacheW),
		float32(offsetY) / float32(cacheH),
		float32(offsetX+viewW) / float32(cacheW),
		float32(offsetY+viewH) / float32(cacheH),
	}
}

func (r *renderer) ensureLayerState(layer *gfx.Layer, width, height int) *layerState {
	state := r.layerStates[layer]
	if state == nil {
		state = &layerState{
			buckets: make(map[geom.AABB[uint32]]*bucketState),
		}
		gl.GenTextures(1, &state.texture)
		gl.GenFramebuffers(1, &state.fbo)
		r.layerStates[layer] = state
	}
	if state.width != width || state.height != height {
		state.width = width
		state.height = height
		r.resizeLayerTexture(state)
	}
	return state
}

func (r *renderer) resizeLayerTexture(state *layerState) {
	gl.BindTexture(gl.TEXTURE_2D, state.texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(state.width), int32(state.height), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)

	gl.BindFramebuffer(gl.FRAMEBUFFER, state.fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, state.texture, 0)
}

func (r *renderer) syncBucketStates(layer *gfx.Layer, state *layerState) {
	if layer == nil || r.source == nil || state == nil {
		return
	}
	deltas := r.source.ConsumeBucketDeltas(layer)
	if len(deltas) == 0 {
		return
	}
	scratch := make([]float32, 0, floatsPerInstance)
	for _, delta := range deltas {
		bucket := r.ensureBucketState(state, delta.Bucket)
		if bucket == nil {
			continue
		}
		updates := make([]int, 0, len(delta.Added)+len(delta.Removed)+len(delta.Updated))
		for _, entryID := range delta.Removed {
			r.bucketRemoveEntry(bucket, entryID, &updates)
		}
		for _, entryID := range delta.Added {
			scratch = r.bucketAddEntry(layer, bucket, entryID, scratch, &updates)
		}
		for _, entryID := range delta.Updated {
			scratch = r.bucketUpdateEntry(layer, bucket, entryID, scratch, &updates)
		}
		required := len(bucket.entries) * floatsPerInstance * 4
		if r.ensureBucketCapacity(bucket, required) {
			r.uploadBucketFull(bucket)
		} else {
			r.uploadBucketUpdates(bucket, updates)
		}
	}
}

func (r *renderer) ensureBucketState(state *layerState, bucketRect geom.AABB[uint32]) *bucketState {
	if state.buckets == nil {
		state.buckets = make(map[geom.AABB[uint32]]*bucketState)
	}
	bucket := state.buckets[bucketRect]
	if bucket != nil {
		return bucket
	}
	bucket = &bucketState{
		index: make(map[uint64]int),
	}
	gl.GenVertexArrays(1, &bucket.vao)
	gl.GenBuffers(1, &bucket.instanceVbo)
	r.setupBucketVAO(bucket)
	state.buckets[bucketRect] = bucket
	return bucket
}

func (r *renderer) setupBucketVAO(bucket *bucketState) {
	gl.BindVertexArray(bucket.vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, gl.PtrOffset(0))

	gl.BindBuffer(gl.ARRAY_BUFFER, bucket.instanceVbo)
	stride := floatsPerInstance * 4
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 4, gl.FLOAT, false, int32(stride), gl.PtrOffset(0))
	gl.VertexAttribDivisor(1, 1)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, 4, gl.FLOAT, false, int32(stride), gl.PtrOffset(4*4))
	gl.VertexAttribDivisor(2, 1)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointer(3, 4, gl.FLOAT, false, int32(stride), gl.PtrOffset(8*4))
	gl.VertexAttribDivisor(3, 1)
}

func (r *renderer) bucketEntryData(layer *gfx.Layer, entryID uint64, scratch []float32) ([]float32, bool) {
	if layer == nil || r.source == nil {
		return scratch, false
	}
	frag, ok := r.source.EntryAABB(layer, entryID)
	if !ok {
		return scratch, false
	}
	drawable := layer.DrawableByID(entryID >> 2)
	if drawable == nil {
		return scratch, false
	}
	scratch = scratch[:0]
	scratch = appendAABBInstance(scratch, frag, drawable.Style)
	if len(scratch) != floatsPerInstance {
		return scratch, false
	}
	return scratch, true
}

func (r *renderer) bucketAddEntry(layer *gfx.Layer, bucket *bucketState, entryID uint64, scratch []float32, updates *[]int) []float32 {
	if bucket.index == nil {
		bucket.index = make(map[uint64]int)
	}
	if _, ok := bucket.index[entryID]; ok {
		return r.bucketUpdateEntry(layer, bucket, entryID, scratch, updates)
	}
	data, ok := r.bucketEntryData(layer, entryID, scratch)
	if !ok {
		return scratch
	}
	idx := len(bucket.entries)
	bucket.entries = append(bucket.entries, entryID)
	bucket.index[entryID] = idx
	start := idx * floatsPerInstance
	if len(bucket.data) != start {
		if len(bucket.data) > start {
			bucket.data = bucket.data[:start]
		} else {
			bucket.data = append(bucket.data, make([]float32, start-len(bucket.data))...)
		}
	}
	bucket.data = append(bucket.data, make([]float32, floatsPerInstance)...)
	copy(bucket.data[start:start+floatsPerInstance], data)
	*updates = append(*updates, idx)
	return scratch
}

func (r *renderer) bucketUpdateEntry(layer *gfx.Layer, bucket *bucketState, entryID uint64, scratch []float32, updates *[]int) []float32 {
	idx, ok := bucket.index[entryID]
	if !ok {
		return r.bucketAddEntry(layer, bucket, entryID, scratch, updates)
	}
	data, ok := r.bucketEntryData(layer, entryID, scratch)
	if !ok {
		return scratch
	}
	start := idx * floatsPerInstance
	if start+floatsPerInstance > len(bucket.data) {
		return scratch
	}
	copy(bucket.data[start:start+floatsPerInstance], data)
	*updates = append(*updates, idx)
	return scratch
}

func (r *renderer) bucketRemoveEntry(bucket *bucketState, entryID uint64, updates *[]int) {
	idx, ok := bucket.index[entryID]
	if !ok {
		return
	}
	lastIdx := len(bucket.entries) - 1
	if lastIdx < 0 {
		return
	}
	lastID := bucket.entries[lastIdx]
	delete(bucket.index, entryID)
	if idx != lastIdx {
		bucket.entries[idx] = lastID
		bucket.index[lastID] = idx
		start := idx * floatsPerInstance
		lastStart := lastIdx * floatsPerInstance
		if lastStart+floatsPerInstance <= len(bucket.data) && start+floatsPerInstance <= len(bucket.data) {
			copy(bucket.data[start:start+floatsPerInstance], bucket.data[lastStart:lastStart+floatsPerInstance])
			*updates = append(*updates, idx)
		}
	}
	bucket.entries = bucket.entries[:lastIdx]
	newLen := lastIdx * floatsPerInstance
	if newLen < 0 {
		newLen = 0
	}
	if newLen <= len(bucket.data) {
		bucket.data = bucket.data[:newLen]
	}
}

func (r *renderer) ensureBucketCapacity(bucket *bucketState, required int) bool {
	if required <= 0 {
		return false
	}
	if required <= bucket.instanceCap {
		return false
	}
	newCap := required
	if bucket.instanceCap > 0 {
		newCap = bucket.instanceCap * 2
		if newCap < required {
			newCap = required
		}
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, bucket.instanceVbo)
	gl.BufferData(gl.ARRAY_BUFFER, newCap, nil, gl.DYNAMIC_DRAW)
	bucket.instanceCap = newCap
	return true
}

func (r *renderer) uploadBucketFull(bucket *bucketState) {
	if len(bucket.data) == 0 {
		return
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, bucket.instanceVbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(bucket.data)*4, gl.Ptr(bucket.data))
}

func (r *renderer) uploadBucketUpdates(bucket *bucketState, updates []int) {
	if len(updates) == 0 {
		return
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, bucket.instanceVbo)
	for _, idx := range updates {
		start := idx * floatsPerInstance
		if start+floatsPerInstance > len(bucket.data) {
			continue
		}
		gl.BufferSubData(gl.ARRAY_BUFFER, start*4, floatsPerInstance*4, gl.Ptr(bucket.data[start:start+floatsPerInstance]))
	}
}

func (r *renderer) buildProgram(pass string) uint32 {
	vertexSource := r.buildShaderSource("VERTEX", pass)
	fragmentSource := r.buildShaderSource("FRAGMENT", pass)

	vertexShader, err := compileShader(gl.VERTEX_SHADER, vertexSource)
	if err != nil {
		panic(err)
	}
	fragmentShader, err := compileShader(gl.FRAGMENT_SHADER, fragmentSource)
	if err != nil {
		panic(err)
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		panic(fmt.Errorf("link error: %s", log))
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)
	return program
}

func (r *renderer) buildShaderSource(stage, pass string) string {
	var sb strings.Builder
	sb.WriteString("#version 330 core\n")
	sb.WriteString("#define " + stage + "\n")
	sb.WriteString("#define " + pass + "\n")
	sb.WriteString(r.shaderSource)
	if !strings.HasSuffix(r.shaderSource, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

func compileShader(shaderType uint32, source string) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source + "\x00")
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))
		return 0, fmt.Errorf("compile error: %s", log)
	}
	return shader, nil
}
