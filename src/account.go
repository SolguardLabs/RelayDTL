package relay

import "sort"

type Balance struct {
	Available Amount `json:"available"`
	Held      Amount `json:"held"`
}

func (b Balance) Total() Amount {
	return b.Available.MustAdd(b.Held)
}

func (b Balance) IsZero() bool {
	return b.Available == 0 && b.Held == 0
}

func (b Balance) Copy() Balance {
	return Balance{Available: b.Available, Held: b.Held}
}

type AccountKind string

const (
	AccountUser     AccountKind = "user"
	AccountOperator AccountKind = "operator"
	AccountReserve  AccountKind = "reserve"
	AccountSystem   AccountKind = "system"
)

type Account struct {
	ID       AccountID           `json:"id"`
	Kind     AccountKind         `json:"kind"`
	Balances map[AssetID]Balance `json:"balances"`
	Labels   map[string]string   `json:"labels,omitempty"`
}

func NewAccount(id AccountID, kind AccountKind) (*Account, error) {
	if err := RequireAccount(id); err != nil {
		return nil, err
	}
	if kind == "" {
		kind = AccountUser
	}
	return &Account{
		ID:       id,
		Kind:     kind,
		Balances: make(map[AssetID]Balance),
		Labels:   make(map[string]string),
	}, nil
}

func (a *Account) Clone() Account {
	balances := make(map[AssetID]Balance, len(a.Balances))
	for asset, balance := range a.Balances {
		balances[asset] = balance.Copy()
	}
	labels := make(map[string]string, len(a.Labels))
	for key, value := range a.Labels {
		labels[key] = value
	}
	return Account{
		ID:       a.ID,
		Kind:     a.Kind,
		Balances: balances,
		Labels:   labels,
	}
}

func (a *Account) Balance(asset AssetID) Balance {
	return a.Balances[asset]
}

func (a *Account) Available(asset AssetID) Amount {
	return a.Balances[asset].Available
}

func (a *Account) Held(asset AssetID) Amount {
	return a.Balances[asset].Held
}

func (a *Account) Total(asset AssetID) Amount {
	return a.Balances[asset].Total()
}

func (a *Account) credit(asset AssetID, amount Amount) error {
	if err := RequireAsset(asset); err != nil {
		return err
	}
	if err := amount.Validate("account.credit"); err != nil {
		return err
	}
	balance := a.Balances[asset]
	next, err := balance.Available.Add(amount)
	if err != nil {
		return err
	}
	balance.Available = next
	a.Balances[asset] = balance
	return nil
}

func (a *Account) debit(asset AssetID, amount Amount) error {
	if err := RequireAsset(asset); err != nil {
		return err
	}
	if err := amount.Validate("account.debit"); err != nil {
		return err
	}
	balance := a.Balances[asset]
	next, err := balance.Available.Sub(amount)
	if err != nil {
		return err
	}
	balance.Available = next
	a.Balances[asset] = balance
	return nil
}

func (a *Account) hold(asset AssetID, amount Amount) error {
	if err := RequireAsset(asset); err != nil {
		return err
	}
	if err := amount.Validate("account.hold"); err != nil {
		return err
	}
	balance := a.Balances[asset]
	available, err := balance.Available.Sub(amount)
	if err != nil {
		return err
	}
	held, err := balance.Held.Add(amount)
	if err != nil {
		return err
	}
	balance.Available = available
	balance.Held = held
	a.Balances[asset] = balance
	return nil
}

func (a *Account) release(asset AssetID, amount Amount) error {
	if err := RequireAsset(asset); err != nil {
		return err
	}
	if err := amount.Validate("account.release"); err != nil {
		return err
	}
	balance := a.Balances[asset]
	held, err := balance.Held.Sub(amount)
	if err != nil {
		return err
	}
	available, err := balance.Available.Add(amount)
	if err != nil {
		return err
	}
	balance.Available = available
	balance.Held = held
	a.Balances[asset] = balance
	return nil
}

func (a *Account) spendHeld(asset AssetID, amount Amount) error {
	if err := RequireAsset(asset); err != nil {
		return err
	}
	if err := amount.Validate("account.spend_held"); err != nil {
		return err
	}
	balance := a.Balances[asset]
	held, err := balance.Held.Sub(amount)
	if err != nil {
		return err
	}
	balance.Held = held
	a.Balances[asset] = balance
	return nil
}

func (a *Account) releaseAvailableHeld(asset AssetID, requested Amount) (Amount, error) {
	if err := RequireAsset(asset); err != nil {
		return 0, err
	}
	if err := requested.Validate("account.release_available_held"); err != nil {
		return 0, err
	}
	balance := a.Balances[asset]
	actual := balance.Held.Min(requested)
	if actual == 0 {
		return 0, nil
	}
	balance.Held = balance.Held.MustSub(actual)
	balance.Available = balance.Available.MustAdd(actual)
	a.Balances[asset] = balance
	return actual, nil
}

func (a *Account) Assets() []AssetID {
	assets := make([]AssetID, 0, len(a.Balances))
	for asset, balance := range a.Balances {
		if !balance.IsZero() {
			assets = append(assets, asset)
		}
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i] < assets[j]
	})
	return assets
}

type AccountSnapshot struct {
	ID       AccountID           `json:"id"`
	Kind     AccountKind         `json:"kind"`
	Balances map[AssetID]Balance `json:"balances"`
}

func (a *Account) Snapshot() AccountSnapshot {
	return AccountSnapshot{
		ID:       a.ID,
		Kind:     a.Kind,
		Balances: AmountlessZeroBalances(a.Balances),
	}
}

func AmountlessZeroBalances(input map[AssetID]Balance) map[AssetID]Balance {
	out := make(map[AssetID]Balance)
	for asset, balance := range input {
		if !balance.IsZero() {
			out[asset] = balance.Copy()
		}
	}
	return out
}
