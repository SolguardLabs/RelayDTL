package relay

import "testing"

func TestRouteFeeCombinesBaseAndVariableComponents(t *testing.T) {
	route, err := NewRoute("route:east", "node:a", "node:c", "USDC", "operator:east", 5000)
	if err != nil {
		t.Fatal(err)
	}
	route.BaseFee = 2
	route.FeeBps = 25
	fee, err := route.FeeFor(1000)
	if err != nil {
		t.Fatal(err)
	}
	if fee != 5 {
		t.Fatalf("fee = %d, want 5", fee)
	}
}

func TestRouteCapacityReflectsReservations(t *testing.T) {
	route, err := NewRoute("route:west", "node:a", "node:c", "USDC", "operator:west", 5000)
	if err != nil {
		t.Fatal(err)
	}
	route.Reserved = 1250
	if got := route.Available(); got != 3750 {
		t.Fatalf("available = %d, want 3750", got)
	}
}

func TestRouteQuotesInitialCongestionPremium(t *testing.T) {
	route, err := NewRoute("route:priced", "node:a", "node:c", "USDC", "operator:west", 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	route.CongestionBps = 5_000
	fee, err := route.FeeFor(400_000)
	if err != nil {
		t.Fatal(err)
	}
	if fee != 32_000 {
		t.Fatalf("initial congestion fee = %d, want 32000", fee)
	}
	if route.Digest() == "" {
		t.Fatal("priced route digest is empty")
	}
}
