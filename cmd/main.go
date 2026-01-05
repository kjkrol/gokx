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
	"github.com/kjkrol/gokx/internal/renderer"
	"github.com/kjkrol/gokx/pkg/gfx"
	"github.com/kjkrol/gokx/pkg/grid"
	"github.com/kjkrol/gokx/pkg/gridbridge"
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
		World: gfx.WorldConfig{
			WorldResolution: worldRes,
			WorldWrap:       true,
		},
	}

	bridge := gridbridge.NewBridge()
	window := gfx.NewWindow(config, renderer.NewRendererFactory(renderer.RendererConfig{ShaderSource: shaderSource}, bridge))
	defer window.Close()

	pane := window.GetDefaultPane()
	if pane == nil {
		panic("default pane is required")
	}
	pane.AddLayer(1)
	pane.AddLayer(2)

	layer0 := pane.GetLayer(0)
	layer1 := pane.GetLayer(1)
	layer2 := pane.GetLayer(2)
	if err := bridge.SetLayerConfig(layer1, grid.GridLevelConfig{
		BucketResolution: spatial.Size64x64,
		BucketCapacity:   16,
		OpsBufferSize:    16000,
	}); err != nil {
		panic(err)
	}
	if err := bridge.SetLayerConfig(layer2, grid.GridLevelConfig{BucketResolution: spatial.Size128x128, BucketCapacity: 16}); err != nil {
		panic(err)
	}

	var space plane.Space2D[uint32]
	if config.World.WorldWrap {
		space = plane.NewToroidal2D(worldRes.Side(), worldRes.Side())
	} else {
		space = plane.NewEuclidean2D(worldRes.Side(), worldRes.Side())
	}
	manager := grid.NewMultiBucketGridManager(
		space,
		worldRes,
		0,
		spatial.Size64x64,
		16,
	)
	bridge.AttachPane(pane, manager)
	layer0.SetBackground(color.RGBA{255, 0, 0, 255})

	worldSide := int(worldRes.Side())
	worldSideU32 := worldRes.Side()
	torus := plane.NewToroidal2D(worldSideU32, worldSideU32)

	polygon1Shape := torus.WrapAABB(geom.NewAABBAt(geom.NewVec[uint32](50, 50), 50, 50))

	polygon1 := &gfx.Drawable{
		AABB: polygon1Shape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	rectShape := torus.WrapAABB(geom.NewAABBAt(geom.NewVec[uint32](150, 150), 100, 100))
	polygon2 := &gfx.Drawable{
		AABB: rectShape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	numberOfPoints := 10000
	pointDrawables := make([]*gfx.Drawable, 0, numberOfPoints)
	for range numberOfPoints {
		randX := uint32(r.Intn(worldSide))
		randY := uint32(r.Intn(worldSide))
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
			polygon1.Update(func(shape *plane.AABB[uint32]) {
				torus.Translate(shape, signedVec(1, 1))
			})
			polygon2.Update(func(shape *plane.AABB[uint32]) {
				torus.Translate(shape, signedVec(0, -1))
			})

			for _, drawable := range pointDrawables {
				dx := r.Intn(5) - 2
				dy := r.Intn(5) - 2
				drawable.Update(func(shape *plane.AABB[uint32]) {
					torus.Translate(shape, signedVec(dx, dy))

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
	plane      plane.Space2D[uint32]
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

func signedVec(dx, dy int) geom.Vec[uint32] {
	return geom.NewVec(uint32(int32(dx)), uint32(int32(dy)))
}
