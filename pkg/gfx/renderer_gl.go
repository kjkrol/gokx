//go:build !js

package gfx

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/kjkrol/gokx/internal/platform"
)

type renderer struct {
	shaderSource string
	initialized  bool

	colorProgram     uint32
	compositeProgram uint32
	quadVbo          uint32
	compositeVao     uint32

	colorViewportUniform     int32
	compositeViewportUniform int32
	compositeRectUniform     int32
	compositeTexUniform      int32

	layerStates map[*Layer]*layerState
}

type layerState struct {
	vao         uint32
	instanceVbo uint32
	instanceCap int
	texture     uint32
	fbo         uint32
	width       int
	height      int
}

func newRenderer(_ platform.PlatformWindowWrapper, conf RendererConfig) *renderer {
	return &renderer{
		shaderSource: conf.ShaderSource,
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

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.Viewport(0, 0, int32(width), int32(height))
	gl.ClearColor(0, 0, 0, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(r.compositeProgram)
	gl.BindVertexArray(r.compositeVao)
	gl.Uniform2f(r.compositeViewportUniform, float32(width), float32(height))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Uniform1i(r.compositeTexUniform, 0)

	for _, pane := range panes {
		if pane == nil || pane.Config == nil {
			continue
		}
		x0 := float32(pane.Config.OffsetX)
		y0 := float32(pane.Config.OffsetY)
		x1 := float32(pane.Config.OffsetX + pane.Config.Width)
		y1 := float32(pane.Config.OffsetY + pane.Config.Height)
		gl.Uniform4f(r.compositeRectUniform, x0, y0, x1, y1)

		for _, layer := range pane.Layers() {
			state := r.layerStates[layer]
			if state == nil || state.texture == 0 {
				continue
			}
			gl.BindTexture(gl.TEXTURE_2D, state.texture)
			gl.DrawArrays(gl.TRIANGLES, 0, 6)
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
		if state.texture != 0 {
			gl.DeleteTextures(1, &state.texture)
		}
		if state.fbo != 0 {
			gl.DeleteFramebuffers(1, &state.fbo)
		}
		if state.instanceVbo != 0 {
			gl.DeleteBuffers(1, &state.instanceVbo)
		}
		if state.vao != 0 {
			gl.DeleteVertexArrays(1, &state.vao)
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
	r.compositeViewportUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uViewport\x00"))
	r.compositeRectUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uRect\x00"))
	r.compositeTexUniform = gl.GetUniformLocation(r.compositeProgram, gl.Str("uTex\x00"))

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

	gl.BindFramebuffer(gl.FRAMEBUFFER, state.fbo)
	gl.Viewport(0, 0, int32(state.width), int32(state.height))
	bgColor := colorToFloat(bg)
	gl.ClearColor(bgColor[0], bgColor[1], bgColor[2], bgColor[3])
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(r.colorProgram)
	gl.BindVertexArray(state.vao)
	gl.Uniform2f(r.colorViewportUniform, float32(state.width), float32(state.height))
	if count > 0 {
		gl.DrawArraysInstanced(gl.TRIANGLES, 0, 6, int32(count))
	}
}

func (r *renderer) ensureLayerState(layer *Layer, width, height int) *layerState {
	state := r.layerStates[layer]
	if state == nil {
		state = &layerState{}
		gl.GenVertexArrays(1, &state.vao)
		gl.GenBuffers(1, &state.instanceVbo)
		r.setupLayerVAO(state)

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

func (r *renderer) setupLayerVAO(state *layerState) {
	gl.BindVertexArray(state.vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, gl.PtrOffset(0))

	gl.BindBuffer(gl.ARRAY_BUFFER, state.instanceVbo)
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

func (r *renderer) updateInstanceBuffer(state *layerState, data []float32) {
	gl.BindBuffer(gl.ARRAY_BUFFER, state.instanceVbo)
	size := len(data) * 4
	if size == 0 {
		gl.BufferData(gl.ARRAY_BUFFER, 0, nil, gl.DYNAMIC_DRAW)
		return
	}
	if size > state.instanceCap {
		gl.BufferData(gl.ARRAY_BUFFER, size, gl.Ptr(data), gl.DYNAMIC_DRAW)
		state.instanceCap = size
		return
	}
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, size, gl.Ptr(data))
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
