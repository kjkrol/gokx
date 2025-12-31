package gfx

// RendererConfig describes GPU shader inputs provided by the caller.
// ShaderSource must be a single-source shader that supports:
// - stage defines: VERTEX, FRAGMENT
// - pass defines: PASS_COLOR, PASS_COMPOSITE
type RendererConfig struct {
	ShaderSource string
}
