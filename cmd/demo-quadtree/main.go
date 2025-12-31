package main

import (
	_ "embed"
	"fmt"
	"image/color"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokq/pkg/qtree"
	"github.com/kjkrol/gokx/pkg/gfx"
)

//go:embed shader.glsl
var shaderSource string

func main() {

	config := gfx.WindowConfig{
		PositionX:   0,
		PositionY:   0,
		Width:       800,
		Height:      800,
		BorderWidth: 0,
		Title:       "Sample Window",
	}

	window := gfx.NewWindow(config, gfx.RendererConfig{ShaderSource: shaderSource})
	defer window.Close()

	layer0 := window.GetDefaultPane().GetLayer(0)
	layer0.SetBackground(color.RGBA{255, 0, 0, 255})

	window.GetDefaultPane().AddLayer(1)
	window.GetDefaultPane().AddLayer(2)

	layerTree := window.GetDefaultPane().GetLayer(2)

	plane := plane.NewToroidal2D(800, 800)
	qtree := qtree.NewQuadTree(plane)
	defer qtree.Close()

	ctx := Context{
		window:         window,
		plane:          plane,
		quadTree:       qtree,
		quadTreeLayer:  layerTree,
		quadTreeFrames: nil,
	}

	window.Show()

	window.RefreshRate(60)

	renderQuadTree(&ctx)

	window.ListenEvents(func(event gfx.Event) {
		handleEvent(event, &ctx)
	})

	fmt.Println("Program closed")

}

type Context struct {
	lmbPressed     bool
	window         *gfx.Window
	plane          plane.Space2D[int]
	quadTree       *qtree.QuadTree[int]
	quadTreeLayer  *gfx.Layer
	quadTreeFrames []*gfx.Drawable
	counter        int
}

type quadTreeItem struct {
	shape geom.AABB[int]
	id    int
}

func (qt *quadTreeItem) Bound() geom.AABB[int] {
	return qt.shape
}

func (qt quadTreeItem) SameID(other qtree.Item[int]) bool {
	o, ok := other.(*quadTreeItem)
	if !ok {
		return false
	}
	return qt.id == o.id
}

func handleEvent(event gfx.Event, ctx *Context) {
	switch e := event.(type) {
	case gfx.Expose:
		fmt.Println("Window exposed")
	case gfx.KeyPress:
		fmt.Printf("Key pressed [code=%d lable=%s]\n", e.Code, e.Label)
		if e.Code == 65307 {
			ctx.window.Stop()
		}
	case gfx.KeyRelease:
		fmt.Println("Key released")
	case gfx.ButtonPress:
		if e.Button == 1 {
			fmt.Printf("Left Mouse Button pressed %d %d\n", e.X, e.Y)
			ctx.lmbPressed = true
			drawDots(e.X, e.Y, ctx)
		}
	case gfx.ButtonRelease:
		if e.Button == 1 {
			fmt.Printf("Left Mouse Button released %d %d\n", e.X, e.Y)
			ctx.lmbPressed = false
		}
	case gfx.MotionNotify:
		if ctx.lmbPressed {
			drawDots(e.X, e.Y, ctx)
		}
	case gfx.EnterNotify:
		fmt.Println("Mouse enter notify")
	case gfx.LeaveNotify:
		fmt.Println("Mouse leave notify")
	case gfx.CreateNotify:
		fmt.Println("Window created")
	case gfx.DestroyNotify:
		fmt.Println("Window destroyed")
		ctx.window.Stop()
	case gfx.ClientMessage:
		ctx.window.Stop()
	case gfx.MouseWheel:
		fmt.Printf("Mouse wheel dx=%.2f dy=%.2f at %d,%d\n", e.DeltaX, e.DeltaY, e.X, e.Y)
	default:
		// fmt.Printf("Unhandled event type: %d\n", e)
	}
}

func drawDots(wX, wY int, ctx *Context) {
	pane := ctx.window.GetDefaultPane()
	px, py := pane.WindowToPaneCoords(wX, wY)
	layer1 := pane.GetLayer(1)
	vec := geom.Vec[int]{X: px, Y: py}
	planeBox := ctx.plane.WrapVec(vec)
	drawable := &gfx.Drawable{
		AABB:  planeBox,
		Style: gfx.SpatialStyle{Stroke: color.White},
	}
	layer1.AddDrawable(drawable)
	id := ctx.counter + 1
	ctx.counter = id
	item := &quadTreeItem{shape: planeBox.AABB, id: id}
	if ctx.quadTree != nil {
		ctx.quadTree.Add(item)
	}
	renderQuadTree(ctx)
}

func renderQuadTree(ctx *Context) {
	if ctx == nil || ctx.quadTree == nil || ctx.quadTreeLayer == nil {
		return
	}
	for _, drawable := range ctx.quadTreeFrames {
		ctx.quadTreeLayer.RemoveDrawable(drawable)
	}
	ctx.quadTreeFrames = ctx.quadTreeFrames[:0]

	leafs := ctx.quadTree.LeafBounds()
	outline := color.RGBA{0, 200, 255, 255}
	for i := range leafs {
		rect := leafs[i]
		rectCopy := rect
		drawable := &gfx.Drawable{
			AABB: ctx.plane.WrapAABB(rectCopy),
			Style: gfx.SpatialStyle{
				Stroke: outline,
			},
		}
		ctx.quadTreeLayer.AddDrawable(drawable)
		ctx.quadTreeFrames = append(ctx.quadTreeFrames, drawable)
	}
}
