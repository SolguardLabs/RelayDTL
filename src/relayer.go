package relay

import "fmt"

type Relayer struct {
	policy        RelayerPolicy
	ledger        *Ledger
	nodes         *NodeRegistry
	routes        *RouteBook
	messages      *MessageBook
	receipts      *ReceiptBook
	confirmations *ConfirmationBook
	settlement    *SettlementEngine
	messageLocks  map[MessageID]Amount
	initialTotals map[AssetID]Amount
}

func NewRelayer(policy RelayerPolicy, now Epoch) (*Relayer, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	ledger := NewLedger(now)
	nodes := NewNodeRegistry()
	relayer := &Relayer{
		policy:        policy,
		ledger:        ledger,
		nodes:         nodes,
		routes:        NewRouteBook(),
		messages:      NewMessageBook(),
		receipts:      NewReceiptBook(),
		confirmations: NewConfirmationBook(nodes),
		messageLocks:  make(map[MessageID]Amount),
	}
	relayer.settlement = NewSettlementEngine(policy, ledger)
	if _, err := ledger.EnsureAccount(policy.ReserveAccount, AccountReserve); err != nil {
		return nil, err
	}
	relayer.initialTotals = ledger.TotalByAsset()
	return relayer, nil
}

func NewDefaultRelayer(now Epoch) (*Relayer, error) {
	return NewRelayer(DefaultPolicy(), now)
}

func (r *Relayer) Policy() RelayerPolicy {
	return r.policy
}

func (r *Relayer) Ledger() *Ledger {
	return r.ledger
}

func (r *Relayer) Now() Epoch {
	return r.ledger.Now()
}

func (r *Relayer) SetNow(now Epoch) {
	r.ledger.SetNow(now)
}

func (r *Relayer) Advance(delta uint64) {
	r.SetNow(r.Now().Add(delta))
}

func (r *Relayer) RegisterAccount(id AccountID, kind AccountKind, balances map[AssetID]Amount) error {
	account, err := r.ledger.EnsureAccount(id, kind)
	if err != nil {
		return err
	}
	if kind != "" {
		account.Kind = kind
	}
	for asset, amount := range balances {
		if err := r.ledger.Credit(id, asset, amount, map[string]string{"seed": "true"}); err != nil {
			return err
		}
	}
	r.initialTotals = r.ledger.TotalByAsset()
	return nil
}

func (r *Relayer) RegisterNode(node Node) error {
	return r.nodes.Register(node)
}

func (r *Relayer) RegisterRoute(route Route) error {
	return r.routes.Add(route)
}

func (r *Relayer) SubmitMessage(terms MessageTerms) (Message, error) {
	message, err := NewMessage(terms, r.Now(), r.policy)
	if err != nil {
		return Message{}, err
	}
	if _, exists := r.messages.Get(message.ID); exists {
		return Message{}, AlreadyExists("relayer.submit", fmt.Sprintf("message %s already exists", message.ID))
	}
	if err := r.ledger.Hold(message.Payer, message.Asset, message.LockedAmount(), map[string]string{"message_id": message.ID.String()}); err != nil {
		return Message{}, err
	}
	if err := r.messages.Add(message); err != nil {
		return Message{}, err
	}
	r.messageLocks[message.ID] = message.LockedAmount()
	r.ledger.Events().Append(Event{
		Kind:      EventMessageSubmitted,
		At:        r.Now(),
		Account:   message.Payer,
		Asset:     message.Asset,
		Amount:    message.LockedAmount(),
		MessageID: message.ID,
	})
	return message, nil
}

func (r *Relayer) PlanRoute(messageID MessageID) (Route, error) {
	message, err := r.messages.Require(messageID)
	if err != nil {
		return Route{}, err
	}
	if message.ExpiredAt(r.Now()) {
		return Route{}, Expired("relayer.plan_route", "message expired")
	}
	if !message.RouteHint.IsZero() {
		route, err := r.routes.Require(message.RouteHint)
		if err != nil {
			return Route{}, err
		}
		if !route.CanCarry(*message, r.Now()) {
			return Route{}, NewRelayError(ErrRouteUnavailable, "relayer.plan_route", "route hint cannot carry message")
		}
		return *route, nil
	}
	route, err := r.routes.Select(*message, r.Now())
	if err != nil {
		return Route{}, err
	}
	return *route, nil
}

func (r *Relayer) AcceptReceipt(draft ReceiptDraft) (Receipt, error) {
	message, err := r.messages.Require(draft.MessageID)
	if err != nil {
		return Receipt{}, err
	}
	if !message.CanAcceptReceipt(r.Now()) {
		return Receipt{}, Expired("relayer.receipt", "message cannot accept receipt")
	}
	var route *Route
	if draft.RouteID.IsZero() {
		selected, err := r.routes.Select(*message, r.Now())
		if err != nil {
			return Receipt{}, err
		}
		route = selected
	} else {
		route, err = r.routes.Require(draft.RouteID)
		if err != nil {
			return Receipt{}, err
		}
	}
	receipt, err := NewReceipt(*message, *route, draft, r.Now(), r.policy)
	if err != nil {
		return Receipt{}, err
	}
	if err := r.routes.Reserve(receipt.RouteID, receipt.Amount, receipt.ObservedAt, r.ledger.Events()); err != nil {
		return Receipt{}, err
	}
	if err := r.receipts.Add(receipt); err != nil {
		return Receipt{}, err
	}
	message.MarkRouted(receipt.RouteID)
	message.MarkDelivered()
	if _, err := r.settlement.EnsureEffect(*message, receipt); err != nil {
		return Receipt{}, err
	}
	r.ledger.Events().Append(Event{
		Kind:      EventReceiptAccepted,
		At:        r.Now(),
		Account:   receipt.Operator,
		Asset:     receipt.Asset,
		Amount:    receipt.Amount,
		MessageID: receipt.MessageID,
		ReceiptID: receipt.ID,
		RouteID:   receipt.RouteID,
	})
	return receipt, nil
}

func (r *Relayer) RecordConfirmation(confirmation Confirmation) (ConfirmationRecordResult, error) {
	receipt, err := r.receipts.Require(confirmation.ReceiptID)
	if err != nil {
		return ConfirmationRecordResult{}, err
	}
	if receipt.MessageID != confirmation.MessageID {
		return ConfirmationRecordResult{}, Invalid("relayer.confirm", "confirmation message mismatch")
	}
	if receipt.RouteID != confirmation.RouteID {
		return ConfirmationRecordResult{}, Invalid("relayer.confirm", "confirmation route mismatch")
	}
	if receipt.ContextID != confirmation.ContextID {
		return ConfirmationRecordResult{}, Invalid("relayer.confirm", "confirmation context mismatch")
	}
	result, err := r.confirmations.Record(confirmation)
	if err != nil {
		return ConfirmationRecordResult{}, err
	}
	kind := EventConfirmationAdded
	if result.Duplicate {
		kind = EventConfirmationIgnored
	}
	r.ledger.Events().Append(Event{
		Kind:      kind,
		At:        r.Now(),
		MessageID: confirmation.MessageID,
		ReceiptID: confirmation.ReceiptID,
		RouteID:   confirmation.RouteID,
		NodeID:    confirmation.NodeID,
		Meta: map[string]string{
			"status": string(confirmation.Status),
		},
	})
	return result, nil
}

func (r *Relayer) ConfirmReceipt(receiptID ReceiptID, nodeID NodeID, status ConfirmationStatus, observedAt Epoch) (ConfirmationRecordResult, Confirmation, error) {
	receipt, err := r.receipts.Require(receiptID)
	if err != nil {
		return ConfirmationRecordResult{}, Confirmation{}, err
	}
	node, err := r.nodes.Require(nodeID)
	if err != nil {
		return ConfirmationRecordResult{}, Confirmation{}, err
	}
	if observedAt == 0 {
		observedAt = r.Now()
	}
	confirmation, err := NewConfirmation(*receipt, node, status, observedAt)
	if err != nil {
		return ConfirmationRecordResult{}, Confirmation{}, err
	}
	result, err := r.RecordConfirmation(confirmation)
	return result, confirmation, err
}

func (r *Relayer) SettleReceipt(id ReceiptID) (SettlementResult, error) {
	receipt, err := r.receipts.Require(id)
	if err != nil {
		return SettlementResult{}, err
	}
	message, err := r.messages.Require(receipt.MessageID)
	if err != nil {
		return SettlementResult{}, err
	}
	route, err := r.routes.Require(receipt.RouteID)
	if err != nil {
		return SettlementResult{}, err
	}
	result, err := r.settlement.Apply(message, receipt, route, r.confirmations)
	if err != nil {
		return result, err
	}
	consumed := result.ChargeSource.FromHeld.MustAdd(result.ReleasedRefund)
	if current := r.messageLocks[message.ID]; current > consumed {
		r.messageLocks[message.ID] = current.MustSub(consumed)
	} else {
		r.messageLocks[message.ID] = 0
	}
	_ = r.routes.Release(receipt.RouteID, receipt.Amount, r.Now(), r.ledger.Events())
	return result, nil
}

func (r *Relayer) ExpireMessage(id MessageID) error {
	message, err := r.messages.Require(id)
	if err != nil {
		return err
	}
	if r.Now() <= message.ExpiresAt {
		return NotReady("relayer.expire", "message has not reached expiration")
	}
	if message.Status == MessageSettled {
		return NewRelayError(ErrSettlementClosed, "relayer.expire", "message already settled")
	}
	if message.Status == MessageExpired {
		return nil
	}
	refund := r.messageLocks[message.ID]
	if held := r.ledger.Held(message.Payer, message.Asset); refund > held {
		refund = held
	}
	if refund > 0 {
		if err := r.ledger.Release(message.Payer, message.Asset, refund, map[string]string{"message_id": message.ID.String()}); err != nil {
			return err
		}
	}
	message.MarkExpired()
	r.messageLocks[message.ID] = 0
	r.ledger.Events().Append(Event{
		Kind:      EventMessageExpired,
		At:        r.Now(),
		Account:   message.Payer,
		Asset:     message.Asset,
		Amount:    refund,
		MessageID: message.ID,
	})
	return nil
}

func (r *Relayer) ConfirmationView(messageID MessageID) ConfirmationView {
	return r.confirmations.ViewForMessage(messageID)
}

func (r *Relayer) Receipt(id ReceiptID) (Receipt, bool) {
	receipt, ok := r.receipts.Get(id)
	if !ok {
		return Receipt{}, false
	}
	return *receipt, true
}

func (r *Relayer) Message(id MessageID) (Message, bool) {
	message, ok := r.messages.Get(id)
	if !ok {
		return Message{}, false
	}
	return *message, true
}

func (r *Relayer) Route(id RouteID) (Route, bool) {
	route, ok := r.routes.Get(id)
	if !ok {
		return Route{}, false
	}
	return *route, true
}

func (r *Relayer) AssertConserved() error {
	return r.ledger.AssertConserved(r.initialTotals)
}
