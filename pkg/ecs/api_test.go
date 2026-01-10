package ecs_test

import (
	"testing"
	"time"

	"github.com/kjkrol/gokx/pkg/ecs"
)

// components

type Order struct {
	ID    string
	Total float64
}

type Status struct {
	Processed bool
}

type Discount struct {
	Percentage float64
}

// system

type BillingSystem struct {
	view           ecs.View
	orders         map[ecs.Entity]*Order
	statuses       map[ecs.Entity]*Status
	discounts      map[ecs.Entity]*Discount
	processedCount int
}

func (s *BillingSystem) Init(api ecs.SystemAPI) {
	s.view = api.NewView(Order{}, Status{}, Discount{})
	s.orders = ecs.Map[Order](api)
	s.statuses = ecs.Map[Status](api)
	s.discounts = ecs.Map[Discount](api)
	s.processedCount = 0
}

func (s *BillingSystem) Update(api ecs.SystemAPI, d time.Duration) {
	api.Each(s.view, func(e ecs.Entity) {
		s.processedCount++

		ord := s.orders[e]
		st := s.statuses[e]
		disc := s.discounts[e]

		// Logika: Nalicz rabat i oznacz jako przetworzone
		ord.Total = ord.Total * (1 - disc.Percentage/100)
		st.Processed = true
	})
}

func TestECS_UseCase(t *testing.T) {
	engine := ecs.NewEngine()

	// Encja A: Spełnia wymagania (Order + Status + Discount)
	eA := engine.CreateEntity()
	ecs.Assign(engine, eA, Order{ID: "ORD-001", Total: 100.0})
	ecs.Assign(engine, eA, Status{Processed: false})
	ecs.Assign(engine, eA, Discount{Percentage: 10.0})

	// Encja B: Nie spełnia wymagań (brak Discount)
	eB := engine.CreateEntity()
	ecs.Assign(engine, eB, Order{ID: "ORD-002", Total: 50.0})
	ecs.Assign(engine, eB, Status{Processed: false})

	// 3. System Przetwarzania
	billingSystem := BillingSystem{}
	engine.RegisterSystems([]ecs.System{&billingSystem})

	engine.UpdateSystems(time.Duration(time.Second))

	// Sprawdź, czy Each znalazł tylko 1 encję (eA)
	if billingSystem.processedCount != 1 {
		t.Errorf("System powinien przetworzyć 1 encję, przetworzył %d", billingSystem.processedCount)
	}

	// Sprawdź, czy wskaźnik zadziałał (czy dane w Registry się zmieniły)
	if billingSystem.orders[eA].Total != 90.0 {
		t.Errorf("Rabat nie został naliczony poprawnie, Total: %v", billingSystem.orders[eA].Total)
	}

	if !billingSystem.statuses[eA].Processed {
		t.Error("Status nie został zmieniony na Processed")
	}

	engine.RemoveEntity(eA)

	// Sprawdź czy dane fizycznie zniknęły z mapy storage (sprzątanie deleterem)
	if _, ok := billingSystem.orders[eA]; ok {
		t.Error("Dane encji A powinny zostać usunięte z mapy Order")
	}
}
