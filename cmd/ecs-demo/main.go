package main

import (
	"fmt"
	"time"

	"github.com/kjkrol/gokx/pkg/ecs"
)

// --- Komponenty ---
type Order struct {
	ID    string
	Total float64
}
type Status struct{ Processed bool }
type Discount struct{ Percentage float64 }

// --- System ---
type BillingSystem struct {
	view      ecs.View
	orders    map[ecs.Entity]*Order
	statuses  map[ecs.Entity]*Status
	discounts map[ecs.Entity]*Discount
}

func (s *BillingSystem) Init(api ecs.SystemAPI) {
	// System przygotowuje sobie narzędzia pracy raz, podczas startu.
	s.view = api.NewView(Order{}, Status{}, Discount{})

	// Mapy są pobierane przez API
	s.orders = ecs.Map[Order](api)
	s.statuses = ecs.Map[Status](api)
	s.discounts = ecs.Map[Discount](api)
}

func (s *BillingSystem) Update(api ecs.SystemAPI, d time.Duration) {
	api.Each(s.view, func(e ecs.Entity) {
		ord := s.orders[e]
		st := s.statuses[e]
		disc := s.discounts[e]

		if !st.Processed {
			ord.Total -= ord.Total * (disc.Percentage / 100)
			st.Processed = true
			fmt.Printf("Przetworzono %s: Nowa suma %.2f\n", ord.ID, ord.Total)
		}
	})
}

// --- Pętla Główna ---
func main() {
	engine := ecs.NewEngine()

	// Setup danych (używając Registry bezpośrednio)
	entity := engine.CreateEntity()
	ecs.Assign(engine, entity, Order{ID: "ORD-99", Total: 200.0})
	ecs.Assign(engine, entity, Status{Processed: false})
	ecs.Assign(engine, entity, Discount{Percentage: 20.0})

	// Inicjalizacja i uruchomienie systemu
	billing := &BillingSystem{}
	engine.RegisterSystems([]ecs.System{billing})

	engine.UpdateSystems(time.Duration(time.Second))
}
