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
