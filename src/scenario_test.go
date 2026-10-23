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

func TestSettledMessageCannotAcceptAnotherReceipt(t *testing.T) {
	scenario := DemoScenario()
	scenario.Steps = append(scenario.Steps, ScenarioStep{
		Op: "receipt", MessageID: "msg:demo", ReceiptID: "rcpt:another",
		RouteID: "route:west", ExpectError: true,
	})
	result, err := RunScenario(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Steps[5].ErrorCode != ErrExpired {
		t.Fatalf("settled message accepted a receipt: %+v", result.Steps[5])
	}
	if got := result.Snapshot.Balances["beneficiary:bob"]["USDC"].Available; got != 1_000 {
		t.Fatalf("beneficiary received %d, want 1000", got)
	}
}
