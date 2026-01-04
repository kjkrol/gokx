package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"math/rand"
	"time"

	"github.com/kjkrol/gokg/pkg/geom"
	"github.com/kjkrol/gokg/pkg/plane"
	"github.com/kjkrol/gokg/pkg/spatial"
	"github.com/kjkrol/gokx/pkg/gfx"
	"github.com/kjkrol/gokx/pkg/grid"
)

//go:embed shader.glsl
var shaderSource string

func main() {

	worldRes := spatial.Size1024x1024
	viewRes := spatial.Size1024x1024

	config := gfx.WindowConfig{
		PositionX:   0,
		PositionY:   0,
		Width:       int(viewRes.Side()),
		Height:      int(viewRes.Side()),
		BorderWidth: 0,
		Title:       "Sample Window",
		Grid: gfx.GridConfig{
			WorldResolution:         worldRes,
			WorldWrap:               true,
			CacheMarginBuckets:      0,
			DefaultBucketResolution: spatial.Size64x64,
			DefaultBucketCapacity:   16,
		},
	}

	window := gfx.NewWindow(config, gfx.RendererConfig{ShaderSource: shaderSource})
	defer window.Close()

	layer0 := window.GetDefaultPane().GetLayer(0)
	layer0.SetBackground(color.RGBA{255, 0, 0, 255})

	window.GetDefaultPane().AddLayer(1)
	window.GetDefaultPane().AddLayer(2)

	layer1 := window.GetDefaultPane().GetLayer(1)
	layer2 := window.GetDefaultPane().GetLayer(2)
	if err := layer1.SetGridConfig(grid.LayerConfig{BucketResolution: spatial.Size64x64, BucketCapacity: 16}); err != nil {
		panic(err)
	}
	if err := layer2.SetGridConfig(grid.LayerConfig{BucketResolution: spatial.Size128x128, BucketCapacity: 16}); err != nil {
		panic(err)
	}

	worldSide := int(worldRes.Side())
	torus := plane.NewToroidal2D(worldSide, worldSide)

	polygon1Shape := torus.WrapAABB(geom.NewAABBAt(geom.NewVec(50, 50), 50, 50))

	polygon1 := &gfx.Drawable{
		AABB: polygon1Shape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	rectShape := torus.WrapAABB(geom.NewAABBAt(geom.NewVec(150, 150), 100, 100))
	polygon2 := &gfx.Drawable{
		AABB: rectShape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	numberOfPoints := 100
	pointDrawables := make([]*gfx.Drawable, 0, numberOfPoints)
	for range numberOfPoints {
		randX := r.Intn(worldSide)
		randY := r.Intn(worldSide)
		vec := geom.NewVec(randX, randY)
		planeBox := torus.WrapVec(vec)
		drawable := &gfx.Drawable{
			AABB:  planeBox,
			Style: gfx.SpatialStyle{Stroke: color.White},
		}
		pointDrawables = append(pointDrawables, drawable)
		layer1.AddDrawable(drawable)
	}

	layer2.AddDrawable(polygon1)
	layer2.AddDrawable(polygon2)

	window.Show()

	// ------- Animations -------------------

	drawables := make([]*gfx.Drawable, 0, len(pointDrawables)+2)
	drawables = append(drawables, pointDrawables...)
	drawables = append(drawables, polygon1, polygon2)

	animation := gfx.NewAnimation(
		layer2,
		50*time.Millisecond,
		drawables,
		func() {
			polygon1.Update(func(shape *plane.AABB[int]) {
				torus.Translate(shape, geom.Vec[int]{X: 1, Y: 1})
			})
			polygon2.Update(func(shape *plane.AABB[int]) {
				torus.Translate(shape, geom.Vec[int]{X: 0, Y: -1})
			})

			for _, drawable := range pointDrawables {
				dx := r.Intn(5) - 2
				dy := r.Intn(5) - 2
				drawable.Update(func(shape *plane.AABB[int]) {
					torus.Translate(shape, geom.Vec[int]{X: dx, Y: dy})

				})
			}
		},
	)

	window.StartAnimation(animation)

	// --------------------------------------

	window.RefreshRate(120)

	ctx := Context{false, window, torus}
	window.ListenEvents(func(event gfx.Event) {
		handleEvent(event, &ctx)
	}, gfx.DrainAll())

	fmt.Println("Program closed")

}

type Context struct {
	lmbPressed bool
	window     *gfx.Window
	plane      plane.Space2D[int]
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
	wx, wy := pane.WindowToWorldCoords(wX, wY)
	layer1 := pane.GetLayer(1)
	vec := geom.NewVec(wx, wy)
	planeBox := ctx.plane.WrapVec(vec)
	drawable := &gfx.Drawable{
		AABB:  planeBox,
		Style: gfx.SpatialStyle{Stroke: color.White},
	}
	layer1.AddDrawable(drawable)
}
