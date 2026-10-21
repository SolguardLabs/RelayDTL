package relay

import (
	"os"
	"testing"
)

func TestDocumentedSettlementScenario(t *testing.T) {
	file, err := os.Open("../examples/settlement.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scenario, err := DecodeScenario(file)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunScenario(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || len(result.Steps) != 5 {
		t.Fatalf("unexpected scenario outcome: ok=%t steps=%d", result.OK, len(result.Steps))
	}
	settlement := result.Steps[4].Settlement
	if settlement == nil || settlement.Principal != 1000 || settlement.Fee != 2 {
		t.Fatalf("unexpected settlement: %+v", settlement)
	}
	if got := result.Snapshot.Totals["USDC"]; got != 15000 {
		t.Fatalf("conserved USDC = %d, want 15000", got)
	}
}
