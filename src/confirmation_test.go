package relay

import "testing"

func TestConfirmationBookDeduplicatesNodeWithinMessage(t *testing.T) {
	registry := NewNodeRegistry()
	node, err := NewNode("node:a", 2, "operator:west")
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(node); err != nil {
		t.Fatal(err)
	}
	book := NewConfirmationBook(registry)
	confirmation := Confirmation{
		ID: "msg:one:rcpt:one:node:a", MessageID: "msg:one", ReceiptID: "rcpt:one",
		RouteID: "route:west", ContextID: "context:one", NodeID: "node:a",
		Status: ConfirmDelivered, ObservedAt: 10, Weight: 2,
	}
	first, err := book.Record(confirmation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := book.Record(confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Accepted || !second.Duplicate {
		t.Fatalf("unexpected record results: first=%+v second=%+v", first, second)
	}
	if got := book.ViewForMessage("msg:one").DeliveredWeight; got != 2 {
		t.Fatalf("delivered weight = %d, want 2", got)
	}
}
