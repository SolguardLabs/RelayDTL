package relay

import "fmt"

type EffectStatus string

const (
	EffectPending EffectStatus = "pending"
	EffectApplied EffectStatus = "applied"
	EffectSkipped EffectStatus = "skipped"
)

type ReceiptEffect struct {
	ReceiptID    ReceiptID    `json:"receipt_id"`
	MessageID    MessageID    `json:"message_id"`
	RouteID      RouteID      `json:"route_id"`
	Asset        AssetID      `json:"asset"`
	Payer        AccountID    `json:"payer"`
	Beneficiary  AccountID    `json:"beneficiary"`
	Operator     AccountID    `json:"operator"`
	Principal    Amount       `json:"principal"`
	Fee          Amount       `json:"fee"`
	FeeRefund    Amount       `json:"fee_refund"`
	Status       EffectStatus `json:"status"`
	AppliedAt    Epoch        `json:"applied_at,omitempty"`
	ChargeSource ChargeSource `json:"charge_source"`
	Digest       Digest       `json:"digest"`
}

func NewReceiptEffect(message Message, receipt Receipt) (ReceiptEffect, error) {
	if receipt.MessageID != message.ID {
		return ReceiptEffect{}, Invalid("effect.new", "message mismatch")
	}
	if receipt.Fee > message.FeeLimit {
		return ReceiptEffect{}, PolicyError("effect.new", "fee exceeds limit")
	}
	refund := message.FeeLimit.MustSub(receipt.Fee)
	effect := ReceiptEffect{
		ReceiptID:   receipt.ID,
		MessageID:   message.ID,
		RouteID:     receipt.RouteID,
		Asset:       receipt.Asset,
		Payer:       message.Payer,
		Beneficiary: message.Beneficiary,
		Operator:    receipt.Operator,
		Principal:   receipt.Amount,
		Fee:         receipt.Fee,
		FeeRefund:   refund,
		Status:      EffectPending,
	}
	effect.Digest = effect.CanonicalDigest()
	return effect, nil
}

func (e ReceiptEffect) CanonicalDigest() Digest {
	return DigestOf(
		e.ReceiptID.String(),
		e.MessageID.String(),
		e.RouteID.String(),
		e.Asset.String(),
		e.Payer.String(),
		e.Beneficiary.String(),
		e.Operator.String(),
		e.Principal.String(),
		e.Fee.String(),
		e.FeeRefund.String(),
		string(e.Status),
	)
}

func (e ReceiptEffect) TotalDebit() Amount {
	return e.Principal.MustAdd(e.Fee)
}

func (e ReceiptEffect) Validate() error {
	if err := RequireReceipt(e.ReceiptID); err != nil {
		return err
	}
	if err := RequireMessage(e.MessageID); err != nil {
		return err
	}
	if err := RequireRoute(e.RouteID); err != nil {
		return err
	}
	if err := RequireAsset(e.Asset); err != nil {
		return err
	}
	if err := RequireAccount(e.Payer); err != nil {
		return err
	}
	if err := RequireAccount(e.Beneficiary); err != nil {
		return err
	}
	if err := RequireAccount(e.Operator); err != nil {
		return err
	}
	if !e.Principal.Positive() {
		return Invalid("effect.validate", "principal required")
	}
	if err := e.Fee.Validate("effect.fee"); err != nil {
		return err
	}
	if err := e.FeeRefund.Validate("effect.refund"); err != nil {
		return err
	}
	switch e.Status {
	case EffectPending, EffectApplied, EffectSkipped:
		return nil
	default:
		return Invalid("effect.validate", "unknown effect status")
	}
}

type SettlementResult struct {
	ReceiptID       ReceiptID          `json:"receipt_id"`
	MessageID       MessageID          `json:"message_id"`
	Applied         bool               `json:"applied"`
	Principal       Amount             `json:"principal"`
	Fee             Amount             `json:"fee"`
	FeeRefund       Amount             `json:"fee_refund"`
	ReleasedRefund  Amount             `json:"released_refund"`
	ChargeSource    ChargeSource       `json:"charge_source"`
	Confirmation    ConfirmationView   `json:"confirmation"`
	Beneficiary     AccountID          `json:"beneficiary"`
	Operator        AccountID          `json:"operator"`
	ReserveAccount  AccountID          `json:"reserve_account"`
	EffectDigest    Digest             `json:"effect_digest"`
	ConservedBefore map[AssetID]Amount `json:"conserved_before,omitempty"`
	ConservedAfter  map[AssetID]Amount `json:"conserved_after,omitempty"`
}

type SettlementEngine struct {
	policy  RelayerPolicy
	ledger  *Ledger
	effects map[ReceiptID]*ReceiptEffect
}

func NewSettlementEngine(policy RelayerPolicy, ledger *Ledger) *SettlementEngine {
	return &SettlementEngine{
		policy:  policy,
		ledger:  ledger,
		effects: make(map[ReceiptID]*ReceiptEffect),
	}
}

func (e *SettlementEngine) EnsureEffect(message Message, receipt Receipt) (*ReceiptEffect, error) {
	if existing, ok := e.effects[receipt.ID]; ok {
		return existing, nil
	}
	effect, err := NewReceiptEffect(message, receipt)
	if err != nil {
		return nil, err
	}
	if err := effect.Validate(); err != nil {
		return nil, err
	}
	e.effects[receipt.ID] = &effect
	return &effect, nil
}

func (e *SettlementEngine) Effect(id ReceiptID) (*ReceiptEffect, bool) {
	effect, ok := e.effects[id]
	return effect, ok
}

func (e *SettlementEngine) Ready(receipt Receipt, confirmations *ConfirmationBook) (ConfirmationView, error) {
	view := confirmations.ViewForMessage(receipt.MessageID)
	if view.Rejected(e.policy) {
		return view, NewRelayError(ErrConfirmation, "settlement.ready", "message rejected by quorum")
	}
	if !view.Ready(e.policy) {
		return view, NotReady("settlement.ready", "confirmation quorum not reached")
	}
	return view, nil
}

func (e *SettlementEngine) Apply(message *Message, receipt *Receipt, route *Route, confirmations *ConfirmationBook) (SettlementResult, error) {
	if message == nil || receipt == nil || route == nil {
		return SettlementResult{}, Invalid("settlement.apply", "nil settlement input")
	}
	if receipt.Status == ReceiptSettled {
		return SettlementResult{}, NewRelayError(ErrSettlementClosed, "settlement.apply", "receipt already settled")
	}
	if receipt.Status == ReceiptExpired || receipt.Status == ReceiptRejected {
		return SettlementResult{}, NewRelayError(ErrSettlementClosed, "settlement.apply", "receipt not settleable")
	}
	if message.Status == MessageExpired || message.Status == MessageCancelled {
		return SettlementResult{}, NewRelayError(ErrSettlementClosed, "settlement.apply", "message not settleable")
	}
	if err := receipt.ValidateAgainst(*message, *route, e.ledger.Now(), e.policy); err != nil {
		return SettlementResult{}, err
	}
	view, err := e.Ready(*receipt, confirmations)
	if err != nil {
		return SettlementResult{ReceiptID: receipt.ID, MessageID: message.ID, Confirmation: view}, err
	}
	effect, err := e.EnsureEffect(*message, *receipt)
	if err != nil {
		return SettlementResult{}, err
	}
	if effect.Status == EffectApplied {
		return SettlementResult{}, NewRelayError(ErrSettlementClosed, "settlement.apply", "effect already applied")
	}
	before := e.ledger.TotalByAsset()
	meta := map[string]string{
		"message_id": string(message.ID),
		"receipt_id": string(receipt.ID),
		"route_id":   string(receipt.RouteID),
	}
	charge, err := e.ledger.ChargeHeldOrReserve(effect.Payer, e.policy.ReserveAccount, effect.Asset, effect.TotalDebit(), meta)
	if err != nil {
		return SettlementResult{}, err
	}
	if err := e.ledger.Credit(effect.Beneficiary, effect.Asset, effect.Principal, meta); err != nil {
		return SettlementResult{}, err
	}
	if effect.Fee > 0 {
		if err := e.ledger.Credit(effect.Operator, effect.Asset, effect.Fee, meta); err != nil {
			return SettlementResult{}, err
		}
	}
	releasedRefund, err := e.ledger.ReleaseAvailableHeld(effect.Payer, effect.Asset, effect.FeeRefund, meta)
	if err != nil {
		return SettlementResult{}, err
	}
	effect.Status = EffectApplied
	effect.AppliedAt = e.ledger.Now()
	effect.ChargeSource = charge
	effect.Digest = effect.CanonicalDigest()
	receipt.MarkSettled()
	message.MarkSettled()
	after := e.ledger.TotalByAsset()
	e.ledger.Events().Append(Event{
		Kind:      EventSettlementApplied,
		At:        e.ledger.Now(),
		Account:   effect.Beneficiary,
		Asset:     effect.Asset,
		Amount:    effect.Principal,
		MessageID: message.ID,
		ReceiptID: receipt.ID,
		RouteID:   receipt.RouteID,
		Meta:      meta,
	})
	return SettlementResult{
		ReceiptID:       receipt.ID,
		MessageID:       message.ID,
		Applied:         true,
		Principal:       effect.Principal,
		Fee:             effect.Fee,
		FeeRefund:       effect.FeeRefund,
		ReleasedRefund:  releasedRefund,
		ChargeSource:    charge,
		Confirmation:    view,
		Beneficiary:     effect.Beneficiary,
		Operator:        effect.Operator,
		ReserveAccount:  e.policy.ReserveAccount,
		EffectDigest:    effect.Digest,
		ConservedBefore: before,
		ConservedAfter:  after,
	}, nil
}

func (e *SettlementEngine) ExpireEffect(receiptID ReceiptID) error {
	effect, ok := e.effects[receiptID]
	if !ok {
		return nil
	}
	if effect.Status == EffectApplied {
		return NewRelayError(ErrSettlementClosed, "settlement.expire", "applied effect cannot expire")
	}
	effect.Status = EffectSkipped
	effect.Digest = effect.CanonicalDigest()
	return nil
}

func (e *SettlementEngine) Snapshot() []ReceiptEffect {
	out := make([]ReceiptEffect, 0, len(e.effects))
	for _, effect := range e.effects {
		out = append(out, *effect)
	}
	return out
}

func (e *SettlementEngine) DescribeEffect(id ReceiptID) string {
	effect, ok := e.effects[id]
	if !ok {
		return fmt.Sprintf("%s:<missing>", id)
	}
	return fmt.Sprintf("%s:%s:%s:%s", effect.ReceiptID, effect.MessageID, effect.Status, effect.TotalDebit())
}
