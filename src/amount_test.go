package relay

import (
	"math"
	"testing"
)

func TestCeilMulBpsRoundsUp(t *testing.T) {
	got, err := MustAmount(1000).CeilMulBps(25)
	if err != nil {
		t.Fatal(err)
	}
	if got != 3 {
		t.Fatalf("rounded fee = %d, want 3", got)
	}
}

func TestCeilMulBpsHandlesMaximumProduct(t *testing.T) {
	got, err := MustAmount(math.MaxInt64).CeilMulBps(1)
	if err != nil {
		t.Fatal(err)
	}
	want := Amount(math.MaxInt64/BpsScale + 1)
	if got != want {
		t.Fatalf("rounded fee = %d, want %d", got, want)
	}
}

func TestAmountRejectsOverflow(t *testing.T) {
	if _, err := MustAmount(math.MaxInt64).Add(1); err == nil {
		t.Fatal("expected overflow error")
	}
}
