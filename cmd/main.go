package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"time"

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

	layer2 := window.GetDefaultPane().GetLayer(2)

	polygon1Shape := geometry.NewPolygon(
		geometry.Vec[int]{X: 150, Y: 100},
		geometry.Vec[int]{X: 200, Y: 200},
		geometry.Vec[int]{X: 100, Y: 200},
	)
	polygon1 := &xgraph.DrawableSpatial{
		Shape: &polygon1Shape,
		Style: xgraph.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	rectShape := geometry.NewRectangle(
		geometry.Vec[int]{X: 100, Y: 100},
		geometry.Vec[int]{X: 200, Y: 200},
	)
	polygon2 := &xgraph.DrawableSpatial{
		Shape: &rectShape,
		Style: xgraph.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	pointVectors := make([]*geometry.Vec[int], 1000)
	pointDrawables := make([]*xgraph.DrawableSpatial, 0, 1000)
	for i := range pointVectors {
		randX := r.Intn(800)
		randY := r.Intn(800)
		vec := &geometry.Vec[int]{X: randX, Y: randY}
		pointVectors[i] = vec
		drawable := &xgraph.DrawableSpatial{
			Shape: vec,
			Style: xgraph.SpatialStyle{Stroke: color.White},
		}
		pointDrawables = append(pointDrawables, drawable)
		layer2.Draw(drawable)
	}

	layer2.Draw(polygon1)

	layer2.Draw(polygon2)

	window.Show()

	// ------- Animations -------------------

	plane := geometry.NewCyclicBoundedPlane(800, 800)

	drawables := make([]*xgraph.DrawableSpatial, 0, len(pointDrawables)+2)
	drawables = append(drawables, pointDrawables...)
	drawables = append(drawables, polygon1, polygon2)

	animation := xgraph.NewAnimation(
		layer2,
		50*time.Millisecond,
		drawables,
		func() {
			polygon1.Fragments = plane.TranslateSpatial(polygon1.Shape, geometry.Vec[int]{X: -1, Y: -1})
			polygon2.Fragments = plane.TranslateSpatial(polygon2.Shape, geometry.Vec[int]{X: 0, Y: -1})

			for _, v := range pointVectors {
				plane.Translate(v, geometry.Vec[int]{X: r.Intn(5) - 2, Y: r.Intn(5) - 2})
			}
		},
	)

	window.StartAnimation(animation)

	// --------------------------------------

	window.Refresh(30)

	ctx := Context{false, window}
	window.ListenEvents(func(event xgraph.Event) {
		handleEvent(event, &ctx)
	})

	fmt.Println("Program closed")

}

type Context struct {
	lmbPressed bool
	window     *xgraph.Window
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
	layer1.Draw(drawable)
}
