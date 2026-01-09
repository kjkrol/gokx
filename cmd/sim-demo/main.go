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
		ChannelBufferSize: 16000,
	}

	bridge := gridbridge.NewBridge()
	window := gfx.NewWindow(config, renderer.NewRendererFactory(renderer.RendererConfig{ShaderSource: shaderSource}, bridge))
	window.SetDrawableEventsApplier(bridge)
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
		BucketResolution: spatial.Size16x16,
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
		ID:   gfx.NextDrawableID(),
		AABB: polygon1Shape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	rectShape := torus.WrapAABB(geom.NewAABBAt(geom.NewVec[uint32](150, 150), 100, 100))
	polygon2 := &gfx.Drawable{
		ID:   gfx.NextDrawableID(),
		AABB: rectShape,
		Style: gfx.SpatialStyle{
			Fill:   color.RGBA{0, 255, 0, 255},
			Stroke: color.RGBA{0, 0, 255, 255},
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	numberOfPoints := 1000
	pointDrawables := make([]*gfx.Drawable, 0, numberOfPoints)
	for range numberOfPoints {
		randX := uint32(r.Intn(worldSide))
		randY := uint32(r.Intn(worldSide))
		vec := geom.NewVec(randX, randY)
		planeBox := torus.WrapAABB(geom.NewAABBAt(vec, 1, 1))
		drawable := &gfx.Drawable{
			ID:    gfx.NextDrawableID(),
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
	simulation := NewSimulation(5*time.Millisecond, func() (gfx.DrawableSetAdded, gfx.DrawableSetRemoved, gfx.DrawableSetTranslated) {
		var translated []gfx.DrawableTranslate

		// move polygon1
		oldPoly1 := polygon1.AABB
		torus.Translate(&polygon1.AABB, signedVec(1, 1))
		translated = append(translated, gfx.DrawableTranslate{
			PaneID:     pane.IDValue(),
			LayerID:    layer2.ID(),
			DrawableID: polygon1.ID,
			Old:        oldPoly1,
			New:        polygon1.AABB,
		})

		// move polygon2
		oldPoly2 := polygon2.AABB
		torus.Translate(&polygon2.AABB, signedVec(0, -1))
		translated = append(translated, gfx.DrawableTranslate{
			PaneID:     pane.IDValue(),
			LayerID:    layer2.ID(),
			DrawableID: polygon2.ID,
			Old:        oldPoly2,
			New:        polygon2.AABB,
		})

		for _, drawable := range pointDrawables {
			dx := r.Intn(15) - 7
			dy := r.Intn(15) - 7
			old := drawable.AABB
			torus.Translate(&drawable.AABB, signedVec(dx, dy))
			translated = append(translated, gfx.DrawableTranslate{
				PaneID:     pane.IDValue(),
				LayerID:    layer1.ID(),
				DrawableID: drawable.ID,
				Old:        old,
				New:        drawable.AABB,
			})
		}

		return gfx.DrawableSetAdded{}, gfx.DrawableSetRemoved{}, gfx.DrawableSetTranslated{Items: translated}
	})
	simulation.Run(window.Context(), func(event gfx.Event) {
		window.EmitEvent(event)
	})

	// --------------------------------------

	ctx := DemoContext{false, window, torus}

	window.RefreshRate(30)
	window.ECSRefreshRate(120)
	window.ListenEvents(func(event gfx.Event) {
		handleEvent(event, &ctx)
	})

	fmt.Println("Program closed")

}

type DemoContext struct {
	lmbPressed bool
	window     *gfx.Window
	plane      plane.Space2D[uint32]
}

func handleEvent(event gfx.Event, ctx *DemoContext) {
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

func drawDots(wX, wY int, ctx *DemoContext) {
	pane := ctx.window.GetDefaultPane()
	wx, wy := pane.WindowToWorldCoords(wX, wY)
	layer1 := pane.GetLayer(1)
	vec := geom.NewVec(wx, wy)
	planeBox := ctx.plane.WrapAABB(geom.NewAABBAt(vec, 1, 1))
	drawable := &gfx.Drawable{
		ID:    gfx.NextDrawableID(),
		AABB:  planeBox,
		Style: gfx.SpatialStyle{Stroke: color.White},
	}
	layer1.AddDrawable(drawable)
}

func signedVec(dx, dy int) geom.Vec[uint32] {
	return geom.NewVec(uint32(int32(dx)), uint32(int32(dy)))
}
