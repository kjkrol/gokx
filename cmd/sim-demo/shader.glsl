#ifdef VERTEX
#if defined(PASS_COLOR)
layout(location = 0) in vec2 aPos;
layout(location = 1) in vec4 iRect;
layout(location = 2) in vec4 iFill;
layout(location = 3) in vec4 iStroke;

uniform vec2 uViewport;
uniform vec2 uOrigin;
uniform vec2 uWorld;

out vec2 vLocal;
out vec2 vSize;
out vec4 vFill;
out vec4 vStroke;

void main() {
	vec2 tl = iRect.xy;
	vec2 br = iRect.zw;
	if (uWorld.x > 0.0 && tl.x < uOrigin.x) {
		tl.x += uWorld.x;
		br.x += uWorld.x;
	}
	if (uWorld.y > 0.0 && tl.y < uOrigin.y) {
		tl.y += uWorld.y;
		br.y += uWorld.y;
	}
	vec2 size = br - tl;
	vec2 pos = (tl - uOrigin) + aPos * size;
	vec2 ndc = vec2(
		(pos.x / uViewport.x) * 2.0 - 1.0,
		1.0 - (pos.y / uViewport.y) * 2.0
	);
	gl_Position = vec4(ndc, 0.0, 1.0);
	vLocal = aPos;
	vSize = size;
	vFill = iFill;
	vStroke = iStroke;
}
#elif defined(PASS_COMPOSITE)
layout(location = 0) in vec2 aPos;

uniform vec2 uViewport;
uniform vec4 uRect;
uniform vec4 uTexRect;

out vec2 vUV;

void main() {
	vec2 pos = mix(uRect.xy, uRect.zw, aPos);
	vec2 ndc = vec2(
		(pos.x / uViewport.x) * 2.0 - 1.0,
		1.0 - (pos.y / uViewport.y) * 2.0
	);
	gl_Position = vec4(ndc, 0.0, 1.0);
	vec2 uv = mix(uTexRect.xy, uTexRect.zw, aPos);
	vUV = vec2(uv.x, 1.0 - uv.y);
}
#endif
#endif

#ifdef FRAGMENT
#if defined(PASS_COLOR)
in vec2 vLocal;
in vec2 vSize;
in vec4 vFill;
in vec4 vStroke;

out vec4 outColor;

void main() {
	float strokeWidth = 1.0;
	if (vStroke.a > 0.0) {
		vec2 dist = min(vLocal * vSize, (1.0 - vLocal) * vSize);
		float edge = min(dist.x, dist.y);
		if (edge < strokeWidth) {
			outColor = vStroke;
			return;
		}
	}
	if (vFill.a <= 0.0) {
		discard;
	}
	outColor = vFill;
}
#elif defined(PASS_COMPOSITE)
in vec2 vUV;

uniform sampler2D uTex;

out vec4 outColor;

void main() {
	outColor = texture(uTex, vUV);
}
#endif
#endif
