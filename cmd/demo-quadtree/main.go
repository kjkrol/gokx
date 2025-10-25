package main

import (
	"fmt"
	"image/color"

	"github.com/kjkrol/gokg/pkg/geometry"
	"github.com/kjkrol/gokq/pkg/quadtree"
	"github.com/kjkrol/gokx/pkg/gfx"
)

func main() {

	config := gfx.WindowConfig{
		PositionX:   0,
		PositionY:   0,
		Width:       800,
		Height:      800,
		BorderWidth: 0,
		Title:       "Sample Window",
	}

	window := gfx.NewWindow(config)
	defer window.Close()

	layer0 := window.GetDefaultPane().GetLayer(0)
	layer0.SetBackground(color.RGBA{255, 0, 0, 255})

	window.GetDefaultPane().AddLayer(1)
	window.GetDefaultPane().AddLayer(2)

	layerTree := window.GetDefaultPane().GetLayer(2)

	plane := geometry.NewCyclicBoundedPlane(800, 800)
	qtree := quadtree.NewQuadTree(plane)
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
	plane          geometry.Plane[int]
	quadTree       *quadtree.QuadTree[int]
	quadTreeLayer  *gfx.Layer
	quadTreeFrames []*gfx.DrawableSpatial
}

type quadTreeItem struct {
	spatial geometry.Spatial[int]
}

func (qt *quadTreeItem) Value() geometry.Spatial[int] {
	return qt.spatial
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
	vec := &geometry.Vec[int]{X: px, Y: py}
	drawable := &gfx.DrawableSpatial{
		Shape: vec,
		Style: gfx.SpatialStyle{Stroke: color.White},
	}
	layer1.AddDrawable(drawable)
	item := &quadTreeItem{spatial: vec}
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

	leafs := ctx.quadTree.LeafRectangles()
	outline := color.RGBA{0, 200, 255, 255}
	for i := range leafs {
		rect := leafs[i]
		rectCopy := rect
		drawable := &gfx.DrawableSpatial{
			Shape: &rectCopy,
			Style: gfx.SpatialStyle{
				Stroke: outline,
			},
		}
		ctx.quadTreeLayer.AddDrawable(drawable)
		ctx.quadTreeFrames = append(ctx.quadTreeFrames, drawable)
	}
}
