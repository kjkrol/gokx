package platform

import (
	"image"
	"sync"
)

// DoubleBuffer trzyma dwa bufor obrazu: front (dla GPU) i back (dla CPU).
type DoubleBuffer struct {
	front *image.RGBA
	back  *image.RGBA
	mu    sync.Mutex
}

// Nowy double buffer dla obrazu o rozmiarze w x h.
func NewDoubleBuffer(w, h int) *DoubleBuffer {
	return &DoubleBuffer{
		front: image.NewRGBA(image.Rect(0, 0, w, h)),
		back:  image.NewRGBA(image.Rect(0, 0, w, h)),
	}
}

// Draw wykonuje operacje rysowania na backbufferze.
func (db *DoubleBuffer) Draw(f func(img *image.RGBA)) {
	db.mu.Lock()
	defer db.mu.Unlock()
	f(db.back)
}

// Swap zamienia bufory i zwraca frontbuffer gotowy do wysłania do GPU.
func (db *DoubleBuffer) Swap() *image.RGBA {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.front, db.back = db.back, db.front
	return db.front
}

// Size zwraca prostokąt całego obrazu.
func (db *DoubleBuffer) Size() image.Rectangle {
	return db.front.Bounds()
}
