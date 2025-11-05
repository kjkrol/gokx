package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"time"

	"github.com/kjkrol/gokg/pkg/geometry"
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

	layer2 := window.GetDefaultPane().GetLayer(2)

	polygon1Shape := geometry.NewAABB(geometry.NewVec(50, 50), 50, 50)

	polygon1 := &gfx.Drawable{
		Shape: polygon1Shape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	rectShape := geometry.NewAABB(geometry.NewVec(150, 150), 100, 100)
	polygon2 := &gfx.Drawable{
		Shape: rectShape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	pointDrawables := make([]*gfx.Drawable, 0, 1000)
	for i := 0; i < 1000; i++ {
		randX := r.Intn(800)
		randY := r.Intn(800)
		vec := &geometry.Vec[int]{X: randX, Y: randY}
		drawable := &gfx.Drawable{
			Shape: vec.Bounds(),
			Style: gfx.SpatialStyle{Stroke: color.White},
		}
		pointDrawables = append(pointDrawables, drawable)
		layer2.AddDrawable(drawable)
	}

	layer2.AddDrawable(polygon1)

	layer2.AddDrawable(polygon2)

	window.Show()

	// ------- Animations -------------------

	plane := geometry.NewCyclicBoundedPlane(800, 800)

	drawables := make([]*gfx.Drawable, 0, len(pointDrawables)+2)
	drawables = append(drawables, pointDrawables...)
	drawables = append(drawables, polygon1, polygon2)

	animation := gfx.NewAnimation(
		layer2,
		50*time.Millisecond,
		drawables,
		func() {
			polygon1.Update(func(shape *geometry.AABB[int]) {
				plane.Translate(shape, geometry.Vec[int]{X: 1, Y: 1})
			})
			polygon2.Update(func(shape *geometry.AABB[int]) {
				plane.Translate(shape, geometry.Vec[int]{X: 0, Y: -1})
			})

			for _, drawable := range pointDrawables {
				dx := r.Intn(5) - 2
				dy := r.Intn(5) - 2
				drawable.Update(func(shape *geometry.AABB[int]) {
					plane.Translate(shape, geometry.Vec[int]{X: dx, Y: dy})

				})
			}
		},
	)

	window.StartAnimation(animation)

	// --------------------------------------

	window.RefreshRate(120)

	ctx := Context{false, window}
	window.ListenEvents(func(event gfx.Event) {
		handleEvent(event, &ctx)
	})

	fmt.Println("Program closed")

}

type Context struct {
	lmbPressed bool
	window     *gfx.Window
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
	vec := geometry.Vec[int]{X: px, Y: py}.Bounds()
	drawable := &gfx.Drawable{
		Shape: vec,
		Style: gfx.SpatialStyle{Stroke: color.White},
	}
	layer1.AddDrawable(drawable)
}
