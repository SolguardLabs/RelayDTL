package relay

import (
	"fmt"
	"math"
	"strconv"
)

type Amount int64

const (
	ZeroAmount Amount = 0
	OneUnit    Amount = 1
	BpsScale   int64  = 10_000
)

func NewAmount(value int64) (Amount, error) {
	if value < 0 {
		return 0, Invalid("amount.new", "amount cannot be negative")
	}
	return Amount(value), nil
}

func MustAmount(value int64) Amount {
	amount, err := NewAmount(value)
	if err != nil {
		panic(err)
	}
	return amount
}

func ParseAmount(value string) (Amount, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, Invalid("amount.parse", err.Error())
	}
	return NewAmount(parsed)
}

func (a Amount) Int64() int64 {
	return int64(a)
}

func (a Amount) String() string {
	return strconv.FormatInt(int64(a), 10)
}

func (a Amount) IsZero() bool {
	return a == 0
}

func (a Amount) Positive() bool {
	return a > 0
}

func (a Amount) Validate(op string) error {
	if a < 0 {
		return Invalid(op, "negative amount")
	}
	return nil
}

func (a Amount) Add(b Amount) (Amount, error) {
	if a < 0 || b < 0 {
		return 0, Invalid("amount.add", "negative amount")
	}
	if int64(a) > math.MaxInt64-int64(b) {
		return 0, Invalid("amount.add", "amount overflow")
	}
	return a + b, nil
}

func (a Amount) MustAdd(b Amount) Amount {
	out, err := a.Add(b)
	if err != nil {
		panic(err)
	}
	return out
}

func (a Amount) Sub(b Amount) (Amount, error) {
	if a < 0 || b < 0 {
		return 0, Invalid("amount.sub", "negative amount")
	}
	if b > a {
		return 0, NewRelayError(ErrInsufficientFunds, "amount.sub", "amount below zero")
	}
	return a - b, nil
}

func (a Amount) MustSub(b Amount) Amount {
	out, err := a.Sub(b)
	if err != nil {
		panic(err)
	}
	return out
}

func (a Amount) MulBps(bps int64) (Amount, error) {
	if a < 0 {
		return 0, Invalid("amount.mul_bps", "negative amount")
	}
	if bps < 0 {
		return 0, Invalid("amount.mul_bps", "negative bps")
	}
	if bps == 0 || a == 0 {
		return 0, nil
	}
	if int64(a) > math.MaxInt64/bps {
		return 0, Invalid("amount.mul_bps", "amount overflow")
	}
	return Amount((int64(a) * bps) / BpsScale), nil
}

func (a Amount) CeilMulBps(bps int64) (Amount, error) {
	if a < 0 {
		return 0, Invalid("amount.ceil_mul_bps", "negative amount")
	}
	if bps < 0 {
		return 0, Invalid("amount.ceil_mul_bps", "negative bps")
	}
	if bps == 0 || a == 0 {
		return 0, nil
	}
	if int64(a) > math.MaxInt64/bps {
		return 0, Invalid("amount.ceil_mul_bps", "amount overflow")
	}
	value := int64(a) * bps
	return Amount((value + BpsScale - 1) / BpsScale), nil
}

func (a Amount) Split(parts int64) (Amount, Amount, error) {
	if parts <= 0 {
		return 0, 0, Invalid("amount.split", "parts must be positive")
	}
	if a < 0 {
		return 0, 0, Invalid("amount.split", "negative amount")
	}
	share := Amount(int64(a) / parts)
	remainder := Amount(int64(a) % parts)
	return share, remainder, nil
}

func (a Amount) Min(b Amount) Amount {
	if a < b {
		return a
	}
	return b
}

func (a Amount) Max(b Amount) Amount {
	if a > b {
		return a
	}
	return b
}

func SumAmounts(values ...Amount) (Amount, error) {
	var total Amount
	for _, value := range values {
		next, err := total.Add(value)
		if err != nil {
			return 0, err
		}
		total = next
	}
	return total, nil
}

func MustSumAmounts(values ...Amount) Amount {
	total, err := SumAmounts(values...)
	if err != nil {
		panic(err)
	}
	return total
}

func AmountMapCopy(input map[AssetID]Amount) map[AssetID]Amount {
	out := make(map[AssetID]Amount, len(input))
	for asset, amount := range input {
		out[asset] = amount
	}
	return out
}

func FormatAssetAmount(asset AssetID, amount Amount) string {
	return fmt.Sprintf("%s:%s", asset, amount)
}
