package relay

import "fmt"

type ReceiptStatus string

const (
	ReceiptObserved ReceiptStatus = "observed"
	ReceiptAccepted ReceiptStatus = "accepted"
	ReceiptSettled  ReceiptStatus = "settled"
	ReceiptExpired  ReceiptStatus = "expired"
	ReceiptRejected ReceiptStatus = "rejected"
)

type Receipt struct {
	ID          ReceiptID     `json:"id"`
	MessageID   MessageID     `json:"message_id"`
	RouteID     RouteID       `json:"route_id"`
	ContextID   ContextID     `json:"context_id"`
	Source      NodeID        `json:"source"`
	Target      NodeID        `json:"target"`
	Payer       AccountID     `json:"payer"`
	Beneficiary AccountID     `json:"beneficiary"`
	Operator    AccountID     `json:"operator"`
	Asset       AssetID       `json:"asset"`
	Amount      Amount        `json:"amount"`
	Fee         Amount        `json:"fee"`
	ObservedAt  Epoch         `json:"observed_at"`
	ExpiresAt   Epoch         `json:"expires_at"`
	Status      ReceiptStatus `json:"status"`
	RouteDigest Digest        `json:"route_digest"`
	Digest      Digest        `json:"digest"`
}

type ReceiptDraft struct {
	ID         ReceiptID `json:"id"`
	MessageID  MessageID `json:"message_id"`
	RouteID    RouteID   `json:"route_id,omitempty"`
	ContextID  ContextID `json:"context_id,omitempty"`
	Operator   AccountID `json:"operator,omitempty"`
	Fee        Amount    `json:"fee,omitempty"`
	ObservedAt Epoch     `json:"observed_at,omitempty"`
	ExpiresAt  Epoch     `json:"expires_at,omitempty"`
}

func NewReceipt(message Message, route Route, draft ReceiptDraft, now Epoch, policy RelayerPolicy) (Receipt, error) {
	if draft.ID.IsZero() {
		return Receipt{}, Invalid("receipt.new", "receipt id required")
	}
	if draft.MessageID != message.ID {
		return Receipt{}, Invalid("receipt.new", "receipt message mismatch")
	}
	if !draft.RouteID.IsZero() && draft.RouteID != route.ID {
		return Receipt{}, Invalid("receipt.new", "receipt route mismatch")
	}
	observedAt := draft.ObservedAt
	if observedAt == 0 {
		observedAt = now
	}
	expiresAt := draft.ExpiresAt
	if expiresAt == 0 {
		expiresAt = message.ExpiresAt
	}
	contextID := draft.ContextID
	if contextID.IsZero() {
		contextID = NextContextID(route.ID, observedAt, draft.ID.String())
	}
	operator := draft.Operator
	if operator.IsZero() {
		operator = route.Operator
	}
	fee := draft.Fee
	if fee == 0 {
		computed, err := route.FeeFor(message.Amount)
		if err != nil {
			return Receipt{}, err
		}
		fee = computed
	}
	receipt := Receipt{
		ID:          draft.ID,
		MessageID:   message.ID,
		RouteID:     route.ID,
		ContextID:   contextID,
		Source:      message.Source,
		Target:      message.Target,
		Payer:       message.Payer,
		Beneficiary: message.Beneficiary,
		Operator:    operator,
		Asset:       message.Asset,
		Amount:      message.Amount,
		Fee:         fee,
		ObservedAt:  observedAt,
		ExpiresAt:   expiresAt,
		Status:      ReceiptObserved,
		RouteDigest: route.Digest(),
	}
	if err := receipt.ValidateAgainst(message, route, now, policy); err != nil {
		return Receipt{}, err
	}
	receipt.Status = ReceiptAccepted
	receipt.Digest = receipt.CanonicalDigest()
	return receipt, nil
}

func (r Receipt) CanonicalDigest() Digest {
	return DigestOf(
		r.ID.String(),
		r.MessageID.String(),
		r.RouteID.String(),
		r.ContextID.String(),
		r.Source.String(),
		r.Target.String(),
		r.Payer.String(),
		r.Beneficiary.String(),
		r.Operator.String(),
		r.Asset.String(),
		r.Amount.String(),
		r.Fee.String(),
		fmt.Sprintf("%d", r.ObservedAt),
		fmt.Sprintf("%d", r.ExpiresAt),
		r.RouteDigest.String(),
	)
}

func (r Receipt) ValidateAgainst(message Message, route Route, now Epoch, policy RelayerPolicy) error {
	if err := RequireReceipt(r.ID); err != nil {
		return err
	}
	if r.MessageID != message.ID {
		return Invalid("receipt.validate", "message id mismatch")
	}
	if r.RouteID != route.ID {
		return Invalid("receipt.validate", "route id mismatch")
	}
	if err := RequireContext(r.ContextID); err != nil {
		return err
	}
	if r.Source != message.Source || r.Target != message.Target {
		return Invalid("receipt.validate", "receipt endpoints mismatch")
	}
	if r.Payer != message.Payer || r.Beneficiary != message.Beneficiary {
		return Invalid("receipt.validate", "receipt account mismatch")
	}
	if r.Asset != message.Asset || route.Asset != message.Asset {
		return Invalid("receipt.validate", "receipt asset mismatch")
	}
	if r.Amount != message.Amount {
		return Invalid("receipt.validate", "receipt amount mismatch")
	}
	if r.Fee > message.FeeLimit {
		return PolicyError("receipt.validate", "receipt fee exceeds message fee limit")
	}
	if r.Operator.IsZero() {
		return Invalid("receipt.validate", "operator required")
	}
	if r.ObservedAt > message.ExpiresAt {
		return Expired("receipt.validate", "receipt observed after message expiration")
	}
	if r.ExpiresAt < r.ObservedAt {
		return Expired("receipt.validate", "receipt expires before observation")
	}
	if now > r.ExpiresAt.Add(policy.MaxReceiptDrift) {
		return Expired("receipt.validate", "receipt outside drift window")
	}
	if !route.ActiveAt(r.ObservedAt) {
		return NewRelayError(ErrRouteUnavailable, "receipt.validate", "route inactive at observation")
	}
	if !route.CanCarry(message, r.ObservedAt) {
		return NewRelayError(ErrRouteUnavailable, "receipt.validate", "route cannot carry message")
	}
	if r.RouteDigest != "" && r.RouteDigest != route.Digest() {
		return Invalid("receipt.validate", "route digest mismatch")
	}
	if r.Digest != "" && r.Digest != r.CanonicalDigest() {
		return Invalid("receipt.validate", "receipt digest mismatch")
	}
	switch r.Status {
	case "", ReceiptObserved, ReceiptAccepted, ReceiptSettled, ReceiptExpired, ReceiptRejected:
		return nil
	default:
		return Invalid("receipt.validate", "unknown receipt status")
	}
}

func (r *Receipt) MarkSettled() {
	r.Status = ReceiptSettled
}

func (r *Receipt) MarkExpired() {
	r.Status = ReceiptExpired
}

type ReceiptBook struct {
	receipts map[ReceiptID]*Receipt
}

func NewReceiptBook() *ReceiptBook {
	return &ReceiptBook{receipts: make(map[ReceiptID]*Receipt)}
}

func (b *ReceiptBook) Add(receipt Receipt) error {
	if _, ok := b.receipts[receipt.ID]; ok {
		return AlreadyExists("receipts.add", fmt.Sprintf("receipt %s already exists", receipt.ID))
	}
	copyReceipt := receipt
	b.receipts[receipt.ID] = &copyReceipt
	return nil
}

func (b *ReceiptBook) Get(id ReceiptID) (*Receipt, bool) {
	receipt, ok := b.receipts[id]
	return receipt, ok
}

func (b *ReceiptBook) Require(id ReceiptID) (*Receipt, error) {
	receipt, ok := b.receipts[id]
	if !ok {
		return nil, NotFound("receipts.require", fmt.Sprintf("receipt %s not found", id))
	}
	return receipt, nil
}

func (b *ReceiptBook) Snapshot() []Receipt {
	out := make([]Receipt, 0, len(b.receipts))
	for _, receipt := range b.receipts {
		out = append(out, *receipt)
	}
	return out
}
