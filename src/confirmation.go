package relay

import "fmt"

type ConfirmationStatus string

const (
	ConfirmDelivered ConfirmationStatus = "delivered"
	ConfirmDeferred  ConfirmationStatus = "deferred"
	ConfirmRejected  ConfirmationStatus = "rejected"
)

type Confirmation struct {
	ID         string             `json:"id"`
	MessageID  MessageID          `json:"message_id"`
	ReceiptID  ReceiptID          `json:"receipt_id"`
	RouteID    RouteID            `json:"route_id"`
	ContextID  ContextID          `json:"context_id"`
	NodeID     NodeID             `json:"node_id"`
	Status     ConfirmationStatus `json:"status"`
	ObservedAt Epoch              `json:"observed_at"`
	Weight     uint64             `json:"weight"`
	Digest     Digest             `json:"digest"`
}

func NewConfirmation(receipt Receipt, node Node, status ConfirmationStatus, observedAt Epoch) (Confirmation, error) {
	if status == "" {
		status = ConfirmDelivered
	}
	confirmation := Confirmation{
		ID:         fmt.Sprintf("%s:%s:%s", receipt.MessageID, receipt.ID, node.ID),
		MessageID:  receipt.MessageID,
		ReceiptID:  receipt.ID,
		RouteID:    receipt.RouteID,
		ContextID:  receipt.ContextID,
		NodeID:     node.ID,
		Status:     status,
		ObservedAt: observedAt,
		Weight:     node.Weight,
	}
	confirmation.Digest = confirmation.CanonicalDigest()
	if err := confirmation.ValidateBasic(); err != nil {
		return Confirmation{}, err
	}
	return confirmation, nil
}

func (c Confirmation) CanonicalDigest() Digest {
	return DigestOf(
		c.ID,
		c.MessageID.String(),
		c.ReceiptID.String(),
		c.RouteID.String(),
		c.ContextID.String(),
		c.NodeID.String(),
		string(c.Status),
		fmt.Sprintf("%d", c.ObservedAt),
		fmt.Sprintf("%d", c.Weight),
	)
}

func (c Confirmation) ValidateBasic() error {
	if c.ID == "" {
		return Invalid("confirmation.validate", "confirmation id required")
	}
	if err := RequireMessage(c.MessageID); err != nil {
		return err
	}
	if err := RequireReceipt(c.ReceiptID); err != nil {
		return err
	}
	if err := RequireRoute(c.RouteID); err != nil {
		return err
	}
	if err := RequireContext(c.ContextID); err != nil {
		return err
	}
	if err := RequireNode(c.NodeID); err != nil {
		return err
	}
	switch c.Status {
	case ConfirmDelivered, ConfirmDeferred, ConfirmRejected:
	default:
		return Invalid("confirmation.validate", "unknown confirmation status")
	}
	if c.Weight == 0 {
		return Invalid("confirmation.validate", "confirmation weight required")
	}
	if c.Digest != "" && c.Digest != c.CanonicalDigest() {
		return Invalid("confirmation.validate", "confirmation digest mismatch")
	}
	return nil
}

type ConfirmationRecordResult struct {
	Accepted  bool   `json:"accepted"`
	Duplicate bool   `json:"duplicate"`
	Reason    string `json:"reason,omitempty"`
}

type ConfirmationView struct {
	MessageID       MessageID            `json:"message_id"`
	DeliveredWeight uint64               `json:"delivered_weight"`
	RejectedWeight  uint64               `json:"rejected_weight"`
	DeferredWeight  uint64               `json:"deferred_weight"`
	Nodes           []NodeID             `json:"nodes"`
	Receipts        map[ReceiptID]uint64 `json:"receipts"`
	LatestEpoch     Epoch                `json:"latest_epoch"`
}

func (v ConfirmationView) Ready(policy RelayerPolicy) bool {
	return v.DeliveredWeight >= policy.QuorumWeight
}

func (v ConfirmationView) Rejected(policy RelayerPolicy) bool {
	return v.RejectedWeight >= policy.RejectWeight
}

type ConfirmationBook struct {
	registry  *NodeRegistry
	byMessage map[MessageID]map[NodeID]Confirmation
	sequence  []Confirmation
}

func NewConfirmationBook(registry *NodeRegistry) *ConfirmationBook {
	return &ConfirmationBook{
		registry:  registry,
		byMessage: make(map[MessageID]map[NodeID]Confirmation),
		sequence:  make([]Confirmation, 0),
	}
}

func (b *ConfirmationBook) Record(confirmation Confirmation) (ConfirmationRecordResult, error) {
	if err := confirmation.ValidateBasic(); err != nil {
		return ConfirmationRecordResult{}, err
	}
	node, err := b.registry.Require(confirmation.NodeID)
	if err != nil {
		return ConfirmationRecordResult{}, err
	}
	if !node.Active() {
		return ConfirmationRecordResult{}, NewRelayError(ErrConfirmation, "confirmations.record", "node is not active")
	}
	if confirmation.Weight != node.Weight {
		confirmation.Weight = node.Weight
		confirmation.Digest = confirmation.CanonicalDigest()
	}
	bucket, ok := b.byMessage[confirmation.MessageID]
	if !ok {
		bucket = make(map[NodeID]Confirmation)
		b.byMessage[confirmation.MessageID] = bucket
	}
	if _, exists := bucket[confirmation.NodeID]; exists {
		return ConfirmationRecordResult{Accepted: false, Duplicate: true, Reason: "node already observed"}, nil
	}
	bucket[confirmation.NodeID] = confirmation
	b.sequence = append(b.sequence, confirmation)
	return ConfirmationRecordResult{Accepted: true}, nil
}

func (b *ConfirmationBook) ViewForMessage(messageID MessageID) ConfirmationView {
	bucket := b.byMessage[messageID]
	view := ConfirmationView{
		MessageID: messageID,
		Nodes:     make([]NodeID, 0, len(bucket)),
		Receipts:  make(map[ReceiptID]uint64),
	}
	for nodeID, confirmation := range bucket {
		view.Nodes = append(view.Nodes, nodeID)
		if confirmation.ObservedAt > view.LatestEpoch {
			view.LatestEpoch = confirmation.ObservedAt
		}
		switch confirmation.Status {
		case ConfirmDelivered:
			view.DeliveredWeight += confirmation.Weight
			view.Receipts[confirmation.ReceiptID] += confirmation.Weight
		case ConfirmRejected:
			view.RejectedWeight += confirmation.Weight
		case ConfirmDeferred:
			view.DeferredWeight += confirmation.Weight
		}
	}
	return view
}

func (b *ConfirmationBook) Snapshot() []Confirmation {
	out := make([]Confirmation, len(b.sequence))
	copy(out, b.sequence)
	return out
}
