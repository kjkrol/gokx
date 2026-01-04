//go:build js && wasm

package gfx

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokx/internal/platform"
	"github.com/kjkrol/gokx/pkg/grid"
)

type renderer struct {
	shaderSource string
	gl           js.Value
	consts       glConsts
	initialized  bool

	colorProgram     js.Value
	compositeProgram js.Value
	quadVbo          js.Value
	compositeVao     js.Value

	colorViewportUniform     js.Value
	colorOriginUniform       js.Value
	colorWorldUniform        js.Value
	compositeViewportUniform js.Value
	compositeRectUniform     js.Value
	compositeTexUniform      js.Value
	compositeTexRectUniform  js.Value

	layerStates map[*Layer]*layerState
	paneViews   map[*Pane]uint64
	paneStates  map[*Pane]*paneState
}

type layerState struct {
	texture js.Value
	fbo     js.Value
	width   int
	height  int
	buckets map[uint32]*bucketState
}

type bucketState struct {
	vao         js.Value
	instanceVbo js.Value
	instanceCap int
	entries     []uint64
	index       map[uint64]int
	data        []float32
}

type paneState struct {
	texture js.Value
	fbo     js.Value
	width   int
	height  int
}

type glConsts struct {
	arrayBuffer      int
	staticDraw       int
	dynamicDraw      int
	floatType        int
	triangles        int
	framebuffer      int
	colorAttachment0 int
	texture2D        int
	rgba8            int
	rgba             int
	unsignedByte     int
	textureMinFilter int
	textureMagFilter int
	nearest          int
	clampToEdge      int
	colorBufferBit   int
	blend            int
	srcAlpha         int
	oneMinusSrcAlpha int
	compileStatus    int
	linkStatus       int
	vertexShader     int
	fragmentShader   int
	textureWrapS     int
	textureWrapT     int
	texture0         int
	scissorTest      int
}

func newRenderer(wrapper platform.PlatformWindowWrapper, conf RendererConfig) *renderer {
	glAny := wrapper.GLContext()
	gl, ok := glAny.(js.Value)
	if !ok || gl.IsUndefined() || gl.IsNull() {
		panic("webgl2 context is required")
	}
	return &renderer{
		shaderSource: conf.ShaderSource,
		gl:           gl,
		layerStates:  make(map[*Layer]*layerState),
		paneViews:    make(map[*Pane]uint64),
		paneStates:   make(map[*Pane]*paneState),
	}
}

func (r *renderer) Render(w *Window) {
	if w == nil || w.defaultPane == nil || w.defaultPane.Config == nil {
		return
	}
	r.ensureInit()

	width := w.width
	height := w.height
	if width <= 0 || height <= 0 {
		return
	}

	panes := w.panesSnapshot()
	type paneFrame struct {
		pane       *Pane
		layers     []*Layer
		plan       grid.FramePlan
		layerPlans map[*Layer]grid.LayerPlan
		worldSize  geom.Vec[int]
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
		manager := w.gridForPane(pane)
		if manager == nil {
			continue
		}
		layers := pane.Layers()
		keys := make([]any, 0, len(layers))
		for _, layer := range layers {
			keys = append(keys, layer)
		}
		viewVersion := view.Version()
		prevVersion, ok := r.paneViews[pane]
		viewChanged := !ok || prevVersion != viewVersion
		if viewChanged {
			r.paneViews[pane] = viewVersion
		}
		frame := manager.BuildFrame(view.Rect(), viewChanged, keys)
		layerPlans := make(map[*Layer]grid.LayerPlan, len(frame.Layers))
		for _, layerPlan := range frame.Layers {
			layer, ok := layerPlan.Key.(*Layer)
			if !ok || layer == nil {
				continue
			}
			layerPlans[layer] = layerPlan
		}
		worldSize := view.WorldSize()
		viewSize := view.Size()
		if !view.Wrap() || viewSize.X >= worldSize.X || viewSize.Y >= worldSize.Y {
			worldSize = geom.NewVec(0, 0)
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

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, js.Null())
	r.gl.Call("viewport", 0, 0, width, height)
	r.gl.Call("clearColor", 0, 0, 0, 1)
	r.gl.Call("clear", r.consts.colorBufferBit)

	r.gl.Call("useProgram", r.compositeProgram)
	r.gl.Call("bindVertexArray", r.compositeVao)
	r.gl.Call("uniform2f", r.compositeViewportUniform, width, height)
	r.gl.Call("activeTexture", r.consts.texture0)
	r.gl.Call("uniform1i", r.compositeTexUniform, 0)

	for _, frame := range paneFrames {
		state := r.paneStates[frame.pane]
		if state == nil || state.texture.IsUndefined() || state.texture.IsNull() {
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
		r.gl.Call("uniform4f", r.compositeRectUniform, x0, y0, x1, y1)
		r.gl.Call("uniform4f", r.compositeTexRectUniform, 0, 0, 1, 1)
		r.gl.Call("bindTexture", r.consts.texture2D, state.texture)
		r.gl.Call("drawArrays", r.consts.triangles, 0, 6)
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
		if state.texture.Truthy() {
			r.gl.Call("deleteTexture", state.texture)
		}
		if state.fbo.Truthy() {
			r.gl.Call("deleteFramebuffer", state.fbo)
		}
		for _, bucket := range state.buckets {
			if bucket == nil {
				continue
			}
			if bucket.instanceVbo.Truthy() {
				r.gl.Call("deleteBuffer", bucket.instanceVbo)
			}
			if bucket.vao.Truthy() {
				r.gl.Call("deleteVertexArray", bucket.vao)
			}
		}
	}
	for _, state := range r.paneStates {
		if state == nil {
			continue
		}
		if state.texture.Truthy() {
			r.gl.Call("deleteTexture", state.texture)
		}
		if state.fbo.Truthy() {
			r.gl.Call("deleteFramebuffer", state.fbo)
		}
	}
	if r.quadVbo.Truthy() {
		r.gl.Call("deleteBuffer", r.quadVbo)
	}
	if r.compositeVao.Truthy() {
		r.gl.Call("deleteVertexArray", r.compositeVao)
	}
	if r.colorProgram.Truthy() {
		r.gl.Call("deleteProgram", r.colorProgram)
	}
	if r.compositeProgram.Truthy() {
		r.gl.Call("deleteProgram", r.compositeProgram)
	}
	r.layerStates = nil
	r.paneStates = nil
	r.initialized = false
}

func (r *renderer) ensureInit() {
	if r.initialized {
		return
	}
	r.initConsts()

	r.colorProgram = r.buildProgram("PASS_COLOR")
	r.compositeProgram = r.buildProgram("PASS_COMPOSITE")

	r.colorViewportUniform = r.gl.Call("getUniformLocation", r.colorProgram, "uViewport")
	r.colorOriginUniform = r.gl.Call("getUniformLocation", r.colorProgram, "uOrigin")
	r.colorWorldUniform = r.gl.Call("getUniformLocation", r.colorProgram, "uWorld")
	r.compositeViewportUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uViewport")
	r.compositeRectUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uRect")
	r.compositeTexUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uTex")
	r.compositeTexRectUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uTexRect")

	r.initQuad()

	r.gl.Call("enable", r.consts.blend)
	r.gl.Call("blendFunc", r.consts.srcAlpha, r.consts.oneMinusSrcAlpha)

	r.initialized = true
}

func (r *renderer) initConsts() {
	r.consts = glConsts{
		arrayBuffer:      r.gl.Get("ARRAY_BUFFER").Int(),
		staticDraw:       r.gl.Get("STATIC_DRAW").Int(),
		dynamicDraw:      r.gl.Get("DYNAMIC_DRAW").Int(),
		floatType:        r.gl.Get("FLOAT").Int(),
		triangles:        r.gl.Get("TRIANGLES").Int(),
		framebuffer:      r.gl.Get("FRAMEBUFFER").Int(),
		colorAttachment0: r.gl.Get("COLOR_ATTACHMENT0").Int(),
		texture2D:        r.gl.Get("TEXTURE_2D").Int(),
		rgba8:            r.gl.Get("RGBA8").Int(),
		rgba:             r.gl.Get("RGBA").Int(),
		unsignedByte:     r.gl.Get("UNSIGNED_BYTE").Int(),
		textureMinFilter: r.gl.Get("TEXTURE_MIN_FILTER").Int(),
		textureMagFilter: r.gl.Get("TEXTURE_MAG_FILTER").Int(),
		nearest:          r.gl.Get("NEAREST").Int(),
		clampToEdge:      r.gl.Get("CLAMP_TO_EDGE").Int(),
		colorBufferBit:   r.gl.Get("COLOR_BUFFER_BIT").Int(),
		blend:            r.gl.Get("BLEND").Int(),
		srcAlpha:         r.gl.Get("SRC_ALPHA").Int(),
		oneMinusSrcAlpha: r.gl.Get("ONE_MINUS_SRC_ALPHA").Int(),
		compileStatus:    r.gl.Get("COMPILE_STATUS").Int(),
		linkStatus:       r.gl.Get("LINK_STATUS").Int(),
		vertexShader:     r.gl.Get("VERTEX_SHADER").Int(),
		fragmentShader:   r.gl.Get("FRAGMENT_SHADER").Int(),
		textureWrapS:     r.gl.Get("TEXTURE_WRAP_S").Int(),
		textureWrapT:     r.gl.Get("TEXTURE_WRAP_T").Int(),
		texture0:         r.gl.Get("TEXTURE0").Int(),
		scissorTest:      r.gl.Get("SCISSOR_TEST").Int(),
	}
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
	r.quadVbo = r.gl.Call("createBuffer")
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, r.quadVbo)
	typed := float32Array(quad)
	r.gl.Call("bufferData", r.consts.arrayBuffer, typed, r.consts.staticDraw)

	r.compositeVao = r.gl.Call("createVertexArray")
	r.gl.Call("bindVertexArray", r.compositeVao)
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, r.quadVbo)
	r.gl.Call("enableVertexAttribArray", 0)
	r.gl.Call("vertexAttribPointer", 0, 2, r.consts.floatType, false, 2*4, 0)
}

func (r *renderer) renderLayerBuckets(layer *Layer, plan grid.LayerPlan, worldSize geom.Vec[int]) {
	if layer == nil {
		return
	}
	manager := layer.GridManager()
	if manager == nil {
		return
	}
	cacheRect := plan.CacheRect
	cacheWidth := cacheRect.BottomRight.X - cacheRect.TopLeft.X
	cacheHeight := cacheRect.BottomRight.Y - cacheRect.TopLeft.Y
	if cacheWidth <= 0 || cacheHeight <= 0 {
		return
	}
	state := r.ensureLayerState(layer, cacheWidth, cacheHeight)
	r.syncBucketStates(layer, manager, state)
	if len(plan.Buckets) == 0 {
		return
	}
	bgColor := colorToFloat(layer.Background())

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, state.fbo)
	r.gl.Call("viewport", 0, 0, state.width, state.height)
	r.gl.Call("useProgram", r.colorProgram)
	r.gl.Call("uniform2f", r.colorViewportUniform, float32(state.width), float32(state.height))
	r.gl.Call("uniform2f", r.colorOriginUniform, float32(cacheRect.TopLeft.X), float32(cacheRect.TopLeft.Y))
	r.gl.Call("uniform2f", r.colorWorldUniform, float32(worldSize.X), float32(worldSize.Y))
	r.gl.Call("enable", r.consts.scissorTest)

	for _, bucket := range plan.Buckets {
		scissor := bucketScissor(bucket, cacheRect, worldSize, state.width, state.height)
		if scissor.W <= 0 || scissor.H <= 0 {
			continue
		}
		r.gl.Call("scissor", scissor.X, scissor.Y, scissor.W, scissor.H)
		r.gl.Call("clearColor", bgColor[0], bgColor[1], bgColor[2], bgColor[3])
		r.gl.Call("clear", r.consts.colorBufferBit)

		bucketIdx, ok := manager.BucketIndex(bucket)
		if !ok {
			continue
		}
		bucketState := state.buckets[bucketIdx]
		if bucketState == nil || len(bucketState.entries) == 0 {
			continue
		}
		r.gl.Call("bindVertexArray", bucketState.vao)
		r.gl.Call("drawArraysInstanced", r.consts.triangles, 0, 6, len(bucketState.entries))
	}

	r.gl.Call("disable", r.consts.scissorTest)
}

func (r *renderer) compositePane(pane *Pane, layers []*Layer, layerPlans map[*Layer]grid.LayerPlan, frame grid.FramePlan, worldSize geom.Vec[int]) {
	if pane == nil || pane.Config == nil {
		return
	}
	state := r.ensurePaneState(pane, pane.Config.Width, pane.Config.Height)
	if state == nil || state.texture.IsUndefined() || state.texture.IsNull() {
		return
	}
	if len(frame.CompositeRects) == 0 {
		return
	}

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, state.fbo)
	r.gl.Call("viewport", 0, 0, state.width, state.height)
	r.gl.Call("useProgram", r.compositeProgram)
	r.gl.Call("bindVertexArray", r.compositeVao)
	r.gl.Call("uniform2f", r.compositeViewportUniform, float32(state.width), float32(state.height))
	r.gl.Call("activeTexture", r.consts.texture0)
	r.gl.Call("uniform1i", r.compositeTexUniform, 0)
	r.gl.Call("uniform4f", r.compositeRectUniform, 0, 0, float32(state.width), float32(state.height))

	r.gl.Call("enable", r.consts.scissorTest)
	for _, rect := range frame.CompositeRects {
		scissor := paneScissor(rect, state.height)
		if scissor.W <= 0 || scissor.H <= 0 {
			continue
		}
		r.gl.Call("scissor", scissor.X, scissor.Y, scissor.W, scissor.H)
		r.gl.Call("clearColor", 0, 0, 0, 0)
		r.gl.Call("clear", r.consts.colorBufferBit)
		for _, layer := range layers {
			plan, ok := layerPlans[layer]
			if !ok {
				continue
			}
			layerState := r.layerStates[layer]
			if layerState == nil || layerState.texture.IsUndefined() || layerState.texture.IsNull() {
				continue
			}
			uv := texRect(frame.ViewRect, plan.CacheRect, worldSize)
			r.gl.Call("uniform4f", r.compositeTexRectUniform, uv[0], uv[1], uv[2], uv[3])
			r.gl.Call("bindTexture", r.consts.texture2D, layerState.texture)
			r.gl.Call("drawArrays", r.consts.triangles, 0, 6)
		}
	}
	r.gl.Call("disable", r.consts.scissorTest)
}

func (r *renderer) ensurePaneState(pane *Pane, width, height int) *paneState {
	state := r.paneStates[pane]
	if state == nil {
		state = &paneState{}
		state.texture = r.gl.Call("createTexture")
		state.fbo = r.gl.Call("createFramebuffer")
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
	r.gl.Call("bindTexture", r.consts.texture2D, state.texture)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureMinFilter, r.consts.nearest)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureMagFilter, r.consts.nearest)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureWrapS, r.consts.clampToEdge)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureWrapT, r.consts.clampToEdge)
	r.gl.Call("texImage2D", r.consts.texture2D, 0, r.consts.rgba8, state.width, state.height, 0, r.consts.rgba, r.consts.unsignedByte, nil)

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, state.fbo)
	r.gl.Call("framebufferTexture2D", r.consts.framebuffer, r.consts.colorAttachment0, r.consts.texture2D, state.texture, 0)
}

type scissorRect struct {
	X int
	Y int
	W int
	H int
}

func bucketScissor(bucket, cacheRect geom.AABB[int], worldSize geom.Vec[int], cacheWidth, cacheHeight int) scissorRect {
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
	localX0 := x0 - origin.X
	localX1 := x1 - origin.X
	localY0 := y0 - origin.Y
	localY1 := y1 - origin.Y
	return scissorRect{
		X: localX0,
		Y: cacheHeight - localY1,
		W: localX1 - localX0,
		H: localY1 - localY0,
	}
}

func paneScissor(rect geom.AABB[int], paneHeight int) scissorRect {
	return scissorRect{
		X: rect.TopLeft.X,
		Y: paneHeight - rect.BottomRight.Y,
		W: rect.BottomRight.X - rect.TopLeft.X,
		H: rect.BottomRight.Y - rect.TopLeft.Y,
	}
}

func unwrapAABBToCache(aabb geom.AABB[int], cacheOrigin, worldSize geom.Vec[int]) geom.AABB[int] {
	x0 := unwrapCoord(aabb.TopLeft.X, cacheOrigin.X, worldSize.X)
	x1 := unwrapCoord(aabb.BottomRight.X, cacheOrigin.X, worldSize.X)
	y0 := unwrapCoord(aabb.TopLeft.Y, cacheOrigin.Y, worldSize.Y)
	y1 := unwrapCoord(aabb.BottomRight.Y, cacheOrigin.Y, worldSize.Y)
	if worldSize.X > 0 && x1 < x0 {
		x1 += worldSize.X
	}
	if worldSize.Y > 0 && y1 < y0 {
		y1 += worldSize.Y
	}
	return geom.NewAABB(geom.NewVec(x0, y0), geom.NewVec(x1, y1))
}

func unwrapCoord(value, origin, worldSize int) int {
	if worldSize > 0 && value < origin {
		return value + worldSize
	}
	return value
}

func texRect(viewRect, cacheRect geom.AABB[int], worldSize geom.Vec[int]) [4]float32 {
	viewW := viewRect.BottomRight.X - viewRect.TopLeft.X
	viewH := viewRect.BottomRight.Y - viewRect.TopLeft.Y
	cacheW := cacheRect.BottomRight.X - cacheRect.TopLeft.X
	cacheH := cacheRect.BottomRight.Y - cacheRect.TopLeft.Y
	if cacheW <= 0 || cacheH <= 0 {
		return [4]float32{0, 0, 1, 1}
	}
	offsetX := viewRect.TopLeft.X - cacheRect.TopLeft.X
	offsetY := viewRect.TopLeft.Y - cacheRect.TopLeft.Y
	if worldSize.X > 0 && offsetX < 0 {
		offsetX += worldSize.X
	}
	if worldSize.Y > 0 && offsetY < 0 {
		offsetY += worldSize.Y
	}
	return [4]float32{
		float32(offsetX) / float32(cacheW),
		float32(offsetY) / float32(cacheH),
		float32(offsetX+viewW) / float32(cacheW),
		float32(offsetY+viewH) / float32(cacheH),
	}
}

func (r *renderer) ensureLayerState(layer *Layer, width, height int) *layerState {
	state := r.layerStates[layer]
	if state == nil {
		state = &layerState{
			buckets: make(map[uint32]*bucketState),
		}
		state.texture = r.gl.Call("createTexture")
		state.fbo = r.gl.Call("createFramebuffer")
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
	r.gl.Call("bindTexture", r.consts.texture2D, state.texture)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureMinFilter, r.consts.nearest)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureMagFilter, r.consts.nearest)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureWrapS, r.consts.clampToEdge)
	r.gl.Call("texParameteri", r.consts.texture2D, r.consts.textureWrapT, r.consts.clampToEdge)
	r.gl.Call("texImage2D", r.consts.texture2D, 0, r.consts.rgba8, state.width, state.height, 0, r.consts.rgba, r.consts.unsignedByte, nil)

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, state.fbo)
	r.gl.Call("framebufferTexture2D", r.consts.framebuffer, r.consts.colorAttachment0, r.consts.texture2D, state.texture, 0)
}

func (r *renderer) syncBucketStates(layer *Layer, manager *grid.BucketGridManager, state *layerState) {
	if layer == nil || manager == nil || state == nil {
		return
	}
	deltas := manager.ConsumeBucketDeltas()
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
			scratch = r.bucketAddEntry(layer, manager, bucket, entryID, scratch, &updates)
		}
		for _, entryID := range delta.Updated {
			scratch = r.bucketUpdateEntry(layer, manager, bucket, entryID, scratch, &updates)
		}
		required := len(bucket.entries) * floatsPerInstance * 4
		if r.ensureBucketCapacity(bucket, required) {
			r.uploadBucketFull(bucket)
		} else {
			r.uploadBucketUpdates(bucket, updates)
		}
	}
}

func (r *renderer) ensureBucketState(state *layerState, bucketIndex uint32) *bucketState {
	if state.buckets == nil {
		state.buckets = make(map[uint32]*bucketState)
	}
	bucket := state.buckets[bucketIndex]
	if bucket != nil {
		return bucket
	}
	bucket = &bucketState{
		index: make(map[uint64]int),
	}
	bucket.vao = r.gl.Call("createVertexArray")
	bucket.instanceVbo = r.gl.Call("createBuffer")
	r.setupBucketVAO(bucket)
	state.buckets[bucketIndex] = bucket
	return bucket
}

func (r *renderer) setupBucketVAO(bucket *bucketState) {
	r.gl.Call("bindVertexArray", bucket.vao)

	r.gl.Call("bindBuffer", r.consts.arrayBuffer, r.quadVbo)
	r.gl.Call("enableVertexAttribArray", 0)
	r.gl.Call("vertexAttribPointer", 0, 2, r.consts.floatType, false, 2*4, 0)

	r.gl.Call("bindBuffer", r.consts.arrayBuffer, bucket.instanceVbo)
	stride := floatsPerInstance * 4
	r.gl.Call("enableVertexAttribArray", 1)
	r.gl.Call("vertexAttribPointer", 1, 4, r.consts.floatType, false, stride, 0)
	r.gl.Call("vertexAttribDivisor", 1, 1)
	r.gl.Call("enableVertexAttribArray", 2)
	r.gl.Call("vertexAttribPointer", 2, 4, r.consts.floatType, false, stride, 4*4)
	r.gl.Call("vertexAttribDivisor", 2, 1)
	r.gl.Call("enableVertexAttribArray", 3)
	r.gl.Call("vertexAttribPointer", 3, 4, r.consts.floatType, false, stride, 8*4)
	r.gl.Call("vertexAttribDivisor", 3, 1)
}

func (r *renderer) bucketEntryData(layer *Layer, manager *grid.BucketGridManager, entryID uint64, scratch []float32) ([]float32, bool) {
	frag, ok := manager.EntryAABB(entryID)
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

func (r *renderer) bucketAddEntry(layer *Layer, manager *grid.BucketGridManager, bucket *bucketState, entryID uint64, scratch []float32, updates *[]int) []float32 {
	if bucket.index == nil {
		bucket.index = make(map[uint64]int)
	}
	if _, ok := bucket.index[entryID]; ok {
		return r.bucketUpdateEntry(layer, manager, bucket, entryID, scratch, updates)
	}
	data, ok := r.bucketEntryData(layer, manager, entryID, scratch)
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

func (r *renderer) bucketUpdateEntry(layer *Layer, manager *grid.BucketGridManager, bucket *bucketState, entryID uint64, scratch []float32, updates *[]int) []float32 {
	idx, ok := bucket.index[entryID]
	if !ok {
		return r.bucketAddEntry(layer, manager, bucket, entryID, scratch, updates)
	}
	data, ok := r.bucketEntryData(layer, manager, entryID, scratch)
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
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, bucket.instanceVbo)
	r.gl.Call("bufferData", r.consts.arrayBuffer, newCap, r.consts.dynamicDraw)
	bucket.instanceCap = newCap
	return true
}

func (r *renderer) uploadBucketFull(bucket *bucketState) {
	if len(bucket.data) == 0 {
		return
	}
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, bucket.instanceVbo)
	arr := float32Array(bucket.data)
	r.gl.Call("bufferSubData", r.consts.arrayBuffer, 0, arr)
}

func (r *renderer) uploadBucketUpdates(bucket *bucketState, updates []int) {
	if len(updates) == 0 {
		return
	}
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, bucket.instanceVbo)
	for _, idx := range updates {
		start := idx * floatsPerInstance
		if start+floatsPerInstance > len(bucket.data) {
			continue
		}
		arr := float32Array(bucket.data[start : start+floatsPerInstance])
		r.gl.Call("bufferSubData", r.consts.arrayBuffer, start*4, arr)
	}
}

func (r *renderer) buildProgram(pass string) js.Value {
	vertexSource := r.buildShaderSource("VERTEX", pass)
	fragmentSource := r.buildShaderSource("FRAGMENT", pass)

	vertexShader := r.compileShader(r.consts.vertexShader, vertexSource)
	fragmentShader := r.compileShader(r.consts.fragmentShader, fragmentSource)

	program := r.gl.Call("createProgram")
	r.gl.Call("attachShader", program, vertexShader)
	r.gl.Call("attachShader", program, fragmentShader)
	r.gl.Call("linkProgram", program)

	if !r.gl.Call("getProgramParameter", program, r.consts.linkStatus).Bool() {
		log := r.gl.Call("getProgramInfoLog", program).String()
		panic(fmt.Errorf("link error: %s", log))
	}

	r.gl.Call("deleteShader", vertexShader)
	r.gl.Call("deleteShader", fragmentShader)
	return program
}

func (r *renderer) compileShader(shaderType int, source string) js.Value {
	shader := r.gl.Call("createShader", shaderType)
	r.gl.Call("shaderSource", shader, source)
	r.gl.Call("compileShader", shader)
	if !r.gl.Call("getShaderParameter", shader, r.consts.compileStatus).Bool() {
		log := r.gl.Call("getShaderInfoLog", shader).String()
		panic(fmt.Errorf("compile error: %s", log))
	}
	return shader
}

func (r *renderer) buildShaderSource(stage, pass string) string {
	var sb strings.Builder
	sb.WriteString("#version 300 es\n")
	sb.WriteString("precision highp float;\n")
	sb.WriteString("precision highp int;\n")
	sb.WriteString("#define " + stage + "\n")
	sb.WriteString("#define " + pass + "\n")
	sb.WriteString(r.shaderSource)
	if !strings.HasSuffix(r.shaderSource, "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}
