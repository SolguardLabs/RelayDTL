package relay

import (
	"fmt"
	"sort"
)

type ChargeSource struct {
	FromHeld    Amount `json:"from_held"`
	FromReserve Amount `json:"from_reserve"`
}

type Ledger struct {
	accounts map[AccountID]*Account
	events   *EventLog
	now      Epoch
}

func NewLedger(now Epoch) *Ledger {
	return &Ledger{
		accounts: make(map[AccountID]*Account),
		events:   NewEventLog(),
		now:      now,
	}
}

func (l *Ledger) SetNow(now Epoch) {
	l.now = now
}

func (l *Ledger) Now() Epoch {
	return l.now
}

func (l *Ledger) Events() *EventLog {
	return l.events
}

func (l *Ledger) EnsureAccount(id AccountID, kind AccountKind) (*Account, error) {
	if err := RequireAccount(id); err != nil {
		return nil, err
	}
	if existing, ok := l.accounts[id]; ok {
		if existing.Kind == "" && kind != "" {
			existing.Kind = kind
		}
		return existing, nil
	}
	account, err := NewAccount(id, kind)
	if err != nil {
		return nil, err
	}
	l.accounts[id] = account
	l.events.Append(Event{Kind: EventAccountCreated, At: l.now, Account: id})
	return account, nil
}

func (l *Ledger) Account(id AccountID) (*Account, bool) {
	account, ok := l.accounts[id]
	return account, ok
}

func (l *Ledger) RequireAccount(id AccountID) (*Account, error) {
	account, ok := l.accounts[id]
	if !ok {
		return nil, NotFound("ledger.account", fmt.Sprintf("account %s not found", id))
	}
	return account, nil
}

func (l *Ledger) Credit(id AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	account, err := l.EnsureAccount(id, AccountUser)
	if err != nil {
		return err
	}
	if err := account.credit(asset, amount); err != nil {
		return Wrap(ErrInvalidInput, "ledger.credit", err)
	}
	l.events.Append(Event{
		Kind:    EventCredit,
		At:      l.now,
		Account: id,
		Asset:   asset,
		Amount:  amount,
		Meta:    meta,
	})
	return nil
}

func (l *Ledger) Debit(id AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	account, err := l.RequireAccount(id)
	if err != nil {
		return err
	}
	if err := account.debit(asset, amount); err != nil {
		return Wrap(ErrInsufficientFunds, "ledger.debit", err)
	}
	l.events.Append(Event{
		Kind:    EventDebit,
		At:      l.now,
		Account: id,
		Asset:   asset,
		Amount:  amount,
		Meta:    meta,
	})
	return nil
}

func (l *Ledger) Hold(id AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	account, err := l.RequireAccount(id)
	if err != nil {
		return err
	}
	if err := account.hold(asset, amount); err != nil {
		return Wrap(ErrInsufficientFunds, "ledger.hold", err)
	}
	l.events.Append(Event{
		Kind:    EventHold,
		At:      l.now,
		Account: id,
		Asset:   asset,
		Amount:  amount,
		Meta:    meta,
	})
	return nil
}

func (l *Ledger) Release(id AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	account, err := l.RequireAccount(id)
	if err != nil {
		return err
	}
	if err := account.release(asset, amount); err != nil {
		return Wrap(ErrInsufficientFunds, "ledger.release", err)
	}
	l.events.Append(Event{
		Kind:    EventRelease,
		At:      l.now,
		Account: id,
		Asset:   asset,
		Amount:  amount,
		Meta:    meta,
	})
	return nil
}

func (l *Ledger) ReleaseAvailableHeld(id AccountID, asset AssetID, amount Amount, meta map[string]string) (Amount, error) {
	account, err := l.RequireAccount(id)
	if err != nil {
		return 0, err
	}
	released, err := account.releaseAvailableHeld(asset, amount)
	if err != nil {
		return 0, Wrap(ErrInsufficientFunds, "ledger.release_available_held", err)
	}
	if released > 0 {
		l.events.Append(Event{
			Kind:    EventRelease,
			At:      l.now,
			Account: id,
			Asset:   asset,
			Amount:  released,
			Meta:    meta,
		})
	}
	return released, nil
}

func (l *Ledger) SpendHeld(id AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	account, err := l.RequireAccount(id)
	if err != nil {
		return err
	}
	if err := account.spendHeld(asset, amount); err != nil {
		return Wrap(ErrInsufficientFunds, "ledger.spend_held", err)
	}
	l.events.Append(Event{
		Kind:    EventDebit,
		At:      l.now,
		Account: id,
		Asset:   asset,
		Amount:  amount,
		Meta:    meta,
	})
	return nil
}

func (l *Ledger) Transfer(from AccountID, to AccountID, asset AssetID, amount Amount, meta map[string]string) error {
	if err := l.Debit(from, asset, amount, meta); err != nil {
		return err
	}
	if err := l.Credit(to, asset, amount, meta); err != nil {
		return err
	}
	return nil
}

func (l *Ledger) ChargeHeldOrReserve(payer AccountID, reserve AccountID, asset AssetID, amount Amount, meta map[string]string) (ChargeSource, error) {
	if err := amount.Validate("ledger.charge"); err != nil {
		return ChargeSource{}, err
	}
	payerAccount, err := l.RequireAccount(payer)
	if err != nil {
		return ChargeSource{}, err
	}
	held := payerAccount.Held(asset)
	fromHeld := held.Min(amount)
	shortfall := amount.MustSub(fromHeld)
	var reserveAccount *Account
	if shortfall > 0 {
		var err error
		reserveAccount, err = l.RequireAccount(reserve)
		if err != nil {
			return ChargeSource{}, err
		}
		if reserveAccount.Available(asset) < shortfall {
			return ChargeSource{}, NewRelayError(ErrInsufficientFunds, "ledger.charge.reserve", "reserve balance below settlement shortfall")
		}
	}
	if fromHeld > 0 {
		if err := payerAccount.spendHeld(asset, fromHeld); err != nil {
			return ChargeSource{}, Wrap(ErrInsufficientFunds, "ledger.charge.held", err)
		}
		l.events.Append(Event{
			Kind:    EventDebit,
			At:      l.now,
			Account: payer,
			Asset:   asset,
			Amount:  fromHeld,
			Meta:    meta,
		})
	}
	if shortfall == 0 {
		return ChargeSource{FromHeld: fromHeld, FromReserve: 0}, nil
	}
	if err := reserveAccount.debit(asset, shortfall); err != nil {
		return ChargeSource{}, Wrap(ErrInsufficientFunds, "ledger.charge.reserve", err)
	}
	l.events.Append(Event{
		Kind:    EventReserveDebit,
		At:      l.now,
		Account: reserve,
		Asset:   asset,
		Amount:  shortfall,
		Meta:    meta,
	})
	return ChargeSource{FromHeld: fromHeld, FromReserve: shortfall}, nil
}

func (l *Ledger) Available(id AccountID, asset AssetID) Amount {
	account, ok := l.accounts[id]
	if !ok {
		return 0
	}
	return account.Available(asset)
}

func (l *Ledger) Held(id AccountID, asset AssetID) Amount {
	account, ok := l.accounts[id]
	if !ok {
		return 0
	}
	return account.Held(asset)
}

func (l *Ledger) TotalByAsset() map[AssetID]Amount {
	totals := make(map[AssetID]Amount)
	for _, account := range l.accounts {
		for asset, balance := range account.Balances {
			next := totals[asset].MustAdd(balance.Total())
			totals[asset] = next
		}
	}
	return totals
}

func (l *Ledger) Snapshot() []AccountSnapshot {
	ids := make([]AccountID, 0, len(l.accounts))
	for id := range l.accounts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	out := make([]AccountSnapshot, 0, len(ids))
	for _, id := range ids {
		out = append(out, l.accounts[id].Snapshot())
	}
	return out
}

func (l *Ledger) BalancesMap() map[AccountID]map[AssetID]Balance {
	out := make(map[AccountID]map[AssetID]Balance, len(l.accounts))
	for _, snapshot := range l.Snapshot() {
		out[snapshot.ID] = snapshot.Balances
	}
	return out
}

func (l *Ledger) AssertConserved(before map[AssetID]Amount) error {
	after := l.TotalByAsset()
	for asset, expected := range before {
		if after[asset] != expected {
			return NewRelayError(
				ErrConservation,
				"ledger.conservation",
				fmt.Sprintf("asset %s expected %s got %s", asset, expected, after[asset]),
			)
		}
	}
	for asset, actual := range after {
		if _, ok := before[asset]; !ok && actual != 0 {
			return NewRelayError(
				ErrConservation,
				"ledger.conservation",
				fmt.Sprintf("asset %s unexpected total %s", asset, actual),
			)
		}
	}
	return nil
}
