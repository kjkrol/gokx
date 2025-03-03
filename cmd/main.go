package main

import (
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"time"

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

	polygon1 := xgraph.Polygon{
		Points:      []image.Point{{150, 100}, {200, 200}, {100, 200}},
		FillColor:   color.RGBA{0, 255, 0, 255},
		BorderColor: color.RGBA{0, 0, 255, 255},
	}

	polygon2 := xgraph.Polygon{
		Points:      []image.Point{{100, 100}, {200, 100}, {200, 200}, {100, 200}},
		FillColor:   color.RGBA{0, 255, 0, 255},
		BorderColor: color.RGBA{0, 0, 255, 255},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	points := make([]xgraph.Point, 1000)
	for i := range points {
		randX := r.Intn(800)
		randY := r.Intn(800)
		points[i] = xgraph.Point{Cord: image.Pt(randX, randY), Color: color.White}
		points[i].Draw(layer2)
	}

	layer2.Draw(&polygon1)

	layer2.Draw(&polygon2)

	window.Show()

	// ------- Animations -------------------

	drabawles := make([]xgraph.Drawable, 0)

	for i := range points {
		drabawles = append(drabawles, &points[i])
	}
	drabawles = append(drabawles, &polygon1)
	drabawles = append(drabawles, &polygon2)

	animation := xgraph.NewAnimation(
		layer2,
		50*time.Millisecond,
		drabawles,
		func() {
			for i := range polygon1.Points {
				polygon1.Points[i].Y += 1
				polygon1.Points[i].X += 0
			}

			// for i := range polygon2.Points {
			// 	polygon2.Points[i].Y += 0
			// 	polygon2.Points[i].X += 1
			// }

			for i := range points {
				points[i].Cord.X += r.Intn(5) - 2
				points[i].Cord.Y += r.Intn(5) - 2
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
		fmt.Printf("Unhandled event type: %d\n", e)
	}
}

func drawDots(wX, wY int, ctx *Context) {
	pane := ctx.window.GetDefaultPane()
	px, py := pane.WindowToPaneCoords(wX, wY)
	layer1 := pane.GetLayer(1)
	point := xgraph.Point{Cord: image.Pt(px, py), Color: color.White}
	layer1.Draw(&point)
}
