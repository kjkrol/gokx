package main

/*
#cgo LDFLAGS: -lSDL2
#include <SDL2/SDL.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func main() {
	if C.SDL_Init(C.SDL_INIT_VIDEO) != 0 {
		fmt.Println("Nie udało się zainicjować SDL:", C.GoString(C.SDL_GetError()))
		return
	}
	defer C.SDL_Quit()

	window := C.SDL_CreateWindow(C.CString("SDL Go"), C.SDL_WINDOWPOS_CENTERED, C.SDL_WINDOWPOS_CENTERED, 800, 600, C.SDL_WINDOW_SHOWN)
	if window == nil {
		fmt.Println("Nie udało się utworzyć okna:", C.GoString(C.SDL_GetError()))
		return
	}
	defer C.SDL_DestroyWindow(window)

	renderer := C.SDL_CreateRenderer(window, -1, 0)
	if renderer == nil {
		fmt.Println("Nie udało się utworzyć renderera:", C.GoString(C.SDL_GetError()))
		return
	}
	defer C.SDL_DestroyRenderer(renderer)

	// Ustawienie koloru tła na niebieski (R, G, B, A)
	C.SDL_SetRenderDrawColor(renderer, 0, 0, 255, 255)
	C.SDL_RenderClear(renderer)
	C.SDL_RenderPresent(renderer)

	// Czekaj na zdarzenie zamknięcia okna
	var event C.SDL_Event
	for {
		C.SDL_WaitEvent(&event)
		eventType := (*(*C.Uint32)(unsafe.Pointer(&event)))
		if eventType == C.SDL_QUIT {
			break
		}
	}
}
