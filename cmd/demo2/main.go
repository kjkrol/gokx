package main

import (
	"fmt"
	"image/color"

	"github.com/kjkrol/goka/pkg/quadtree"
	"github.com/kjkrol/gokg/pkg/geometry"
	"github.com/kjkrol/gokx/pkg/xgraph"
)

func main() {

	config := xgraph.WindowConfig{
		PositionX:   0,
		PositionY:   0,
		Width:       800,
		Height:      800,
		BorderWidth: 0,
		Title:       "Sample Window",
	}

	window := xgraph.NewWindow(config)
	defer window.Close()

	layer0 := window.GetDefaultPane().GetLayer(0)
	layer0.SetBackground(color.RGBA{255, 0, 0, 255})

	window.GetDefaultPane().AddLayer(1)
	window.GetDefaultPane().AddLayer(2)

	layerTree := window.GetDefaultPane().GetLayer(2)

	plane := geometry.NewCyclicBoundedPlane(800, 800)
	qtree := quadtree.NewQuadTree(plane, geometry.BoundingBoxDistanceForPlane(plane))
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

	window.ListenEvents(func(event xgraph.Event) {
		handleEvent(event, &ctx)
	})

	fmt.Println("Program closed")

}

type Context struct {
	lmbPressed     bool
	window         *xgraph.Window
	plane          geometry.Plane[int]
	quadTree       *quadtree.QuadTree[int]
	quadTreeLayer  *xgraph.Layer
	quadTreeFrames []*xgraph.DrawableSpatial
}

type quadTreeItem struct {
	spatial geometry.Spatial[int]
}

func (qt *quadTreeItem) Value() geometry.Spatial[int] {
	return qt.spatial
}

func handleEvent(event xgraph.Event, ctx *Context) {
	switch e := event.(type) {
	case xgraph.Expose:
		fmt.Println("Window exposed")
	case xgraph.KeyPress:
		fmt.Printf("Key pressed [code=%d lable=%s]\n", e.Code, e.Label)
		if e.Code == 65307 {
			ctx.window.Stop()
		}
	case xgraph.KeyRelease:
		fmt.Println("Key released")
	case xgraph.ButtonPress:
		if e.Button == 1 {
			fmt.Printf("Left Mouse Button pressed %d %d\n", e.X, e.Y)
			ctx.lmbPressed = true
			drawDots(e.X, e.Y, ctx)
		}
	case xgraph.ButtonRelease:
		if e.Button == 1 {
			fmt.Printf("Left Mouse Button released %d %d\n", e.X, e.Y)
			ctx.lmbPressed = false
		}
	case xgraph.MotionNotify:
		if ctx.lmbPressed {
			drawDots(e.X, e.Y, ctx)
		}
	case xgraph.EnterNotify:
		fmt.Println("Mouse enter notify")
	case xgraph.LeaveNotify:
		fmt.Println("Mouse leave notify")
	case xgraph.CreateNotify:
		fmt.Println("Window created")
	case xgraph.DestroyNotify:
		fmt.Println("Window destroyed")
		ctx.window.Stop()
	case xgraph.ClientMessage:
		ctx.window.Stop()
	case xgraph.MouseWheel:
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
	drawable := &xgraph.DrawableSpatial{
		Shape: vec,
		Style: xgraph.SpatialStyle{Stroke: color.White},
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
		drawable := &xgraph.DrawableSpatial{
			Shape: &rectCopy,
			Style: xgraph.SpatialStyle{
				Stroke: outline,
			},
		}
		ctx.quadTreeLayer.AddDrawable(drawable)
		ctx.quadTreeFrames = append(ctx.quadTreeFrames, drawable)
	}
}
