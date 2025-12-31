//go:build js && wasm

package gfx

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/kjkrol/gokx/internal/platform"
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
	compositeViewportUniform js.Value
	compositeRectUniform     js.Value
	compositeTexUniform      js.Value

	layerStates map[*Layer]*layerState
}

type layerState struct {
	vao         js.Value
	instanceVbo js.Value
	instanceCap int
	texture     js.Value
	fbo         js.Value
	width       int
	height      int
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

	for _, pane := range panes {
		if pane == nil || pane.Config == nil {
			continue
		}
		for _, layer := range pane.Layers() {
			if layer == nil {
				continue
			}
			r.renderLayer(layer, pane.Config.Width, pane.Config.Height)
		}
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

	for _, pane := range panes {
		if pane == nil || pane.Config == nil {
			continue
		}
		x0 := float32(pane.Config.OffsetX)
		y0 := float32(pane.Config.OffsetY)
		x1 := float32(pane.Config.OffsetX + pane.Config.Width)
		y1 := float32(pane.Config.OffsetY + pane.Config.Height)
		r.gl.Call("uniform4f", r.compositeRectUniform, x0, y0, x1, y1)

		for _, layer := range pane.Layers() {
			state := r.layerStates[layer]
			if state == nil || state.texture.IsUndefined() || state.texture.IsNull() {
				continue
			}
			r.gl.Call("bindTexture", r.consts.texture2D, state.texture)
			r.gl.Call("drawArrays", r.consts.triangles, 0, 6)
		}
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
		if state.instanceVbo.Truthy() {
			r.gl.Call("deleteBuffer", state.instanceVbo)
		}
		if state.vao.Truthy() {
			r.gl.Call("deleteVertexArray", state.vao)
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
	r.compositeViewportUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uViewport")
	r.compositeRectUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uRect")
	r.compositeTexUniform = r.gl.Call("getUniformLocation", r.compositeProgram, "uTex")

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
	typed := js.TypedArrayOf(quad)
	r.gl.Call("bufferData", r.consts.arrayBuffer, typed, r.consts.staticDraw)
	typed.Release()

	r.compositeVao = r.gl.Call("createVertexArray")
	r.gl.Call("bindVertexArray", r.compositeVao)
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, r.quadVbo)
	r.gl.Call("enableVertexAttribArray", 0)
	r.gl.Call("vertexAttribPointer", 0, 2, r.consts.floatType, false, 2*4, 0)
}

func (r *renderer) renderLayer(layer *Layer, width, height int) {
	state := r.layerStates[layer]
	force := state == nil || state.width != width || state.height != height
	if force {
		state = r.ensureLayerState(layer, width, height)
	}

	data, count, bg, dirty := layer.consumeInstances(force)
	if !dirty {
		return
	}

	r.updateInstanceBuffer(state, data)

	r.gl.Call("bindFramebuffer", r.consts.framebuffer, state.fbo)
	r.gl.Call("viewport", 0, 0, state.width, state.height)
	bgColor := colorToFloat(bg)
	r.gl.Call("clearColor", bgColor[0], bgColor[1], bgColor[2], bgColor[3])
	r.gl.Call("clear", r.consts.colorBufferBit)

	r.gl.Call("useProgram", r.colorProgram)
	r.gl.Call("bindVertexArray", state.vao)
	r.gl.Call("uniform2f", r.colorViewportUniform, float32(state.width), float32(state.height))
	if count > 0 {
		r.gl.Call("drawArraysInstanced", r.consts.triangles, 0, 6, count)
	}
}

func (r *renderer) ensureLayerState(layer *Layer, width, height int) *layerState {
	state := r.layerStates[layer]
	if state == nil {
		state = &layerState{}
		state.vao = r.gl.Call("createVertexArray")
		state.instanceVbo = r.gl.Call("createBuffer")
		r.setupLayerVAO(state)

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

func (r *renderer) setupLayerVAO(state *layerState) {
	r.gl.Call("bindVertexArray", state.vao)

	r.gl.Call("bindBuffer", r.consts.arrayBuffer, r.quadVbo)
	r.gl.Call("enableVertexAttribArray", 0)
	r.gl.Call("vertexAttribPointer", 0, 2, r.consts.floatType, false, 2*4, 0)

	r.gl.Call("bindBuffer", r.consts.arrayBuffer, state.instanceVbo)
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

func (r *renderer) updateInstanceBuffer(state *layerState, data []float32) {
	r.gl.Call("bindBuffer", r.consts.arrayBuffer, state.instanceVbo)
	if len(data) == 0 {
		r.gl.Call("bufferData", r.consts.arrayBuffer, 0, r.consts.dynamicDraw)
		return
	}
	byteLen := len(data) * 4
	arr := js.TypedArrayOf(data)
	defer arr.Release()
	if byteLen > state.instanceCap {
		r.gl.Call("bufferData", r.consts.arrayBuffer, arr, r.consts.dynamicDraw)
		state.instanceCap = byteLen
		return
	}
	r.gl.Call("bufferSubData", r.consts.arrayBuffer, 0, arr)
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
