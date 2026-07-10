package relay

import "fmt"

type MessageStatus string

const (
	MessageOpen      MessageStatus = "open"
	MessageRouted    MessageStatus = "routed"
	MessageDelivered MessageStatus = "delivered"
	MessageSettled   MessageStatus = "settled"
	MessageExpired   MessageStatus = "expired"
	MessageCancelled MessageStatus = "cancelled"
)

type Message struct {
	ID          MessageID     `json:"id"`
	Source      NodeID        `json:"source"`
	Target      NodeID        `json:"target"`
	Payer       AccountID     `json:"payer"`
	Beneficiary AccountID     `json:"beneficiary"`
	Asset       AssetID       `json:"asset"`
	Amount      Amount        `json:"amount"`
	FeeLimit    Amount        `json:"fee_limit"`
	CreatedAt   Epoch         `json:"created_at"`
	ExpiresAt   Epoch         `json:"expires_at"`
	Status      MessageStatus `json:"status"`
	RouteHint   RouteID       `json:"route_hint,omitempty"`
	Memo        string        `json:"memo,omitempty"`
	Digest      Digest        `json:"digest"`
}

type MessageTerms struct {
	ID          MessageID `json:"id"`
	Source      NodeID    `json:"source"`
	Target      NodeID    `json:"target"`
	Payer       AccountID `json:"payer"`
	Beneficiary AccountID `json:"beneficiary"`
	Asset       AssetID   `json:"asset"`
	Amount      Amount    `json:"amount"`
	FeeLimit    Amount    `json:"fee_limit"`
	ExpiresAt   Epoch     `json:"expires_at"`
	RouteHint   RouteID   `json:"route_hint,omitempty"`
	Memo        string    `json:"memo,omitempty"`
}

func (t MessageTerms) Validate(now Epoch, policy RelayerPolicy) error {
	if err := RequireMessage(t.ID); err != nil {
		return err
	}
	if err := RequireNode(t.Source); err != nil {
		return err
	}
	if err := RequireNode(t.Target); err != nil {
		return err
	}
	if t.Source == t.Target {
		return Invalid("message.validate", "source and target must differ")
	}
	if err := RequireAccount(t.Payer); err != nil {
		return err
	}
	if err := RequireAccount(t.Beneficiary); err != nil {
		return err
	}
	if err := RequireAsset(t.Asset); err != nil {
		return err
	}
	if !t.Amount.Positive() {
		return Invalid("message.validate", "message amount required")
	}
	if err := t.FeeLimit.Validate("message.fee_limit"); err != nil {
		return err
	}
	if t.ExpiresAt <= now {
		return Expired("message.validate", "message expires before current epoch")
	}
	if policy.MaxMessageTTL > 0 && policy.MaxMessageTTL != ^uint64(0) {
		if uint64(t.ExpiresAt-now) > policy.MaxMessageTTL {
			return PolicyError("message.validate", "message ttl exceeds policy")
		}
	}
	if !t.RouteHint.IsZero() {
		if err := RequireRoute(t.RouteHint); err != nil {
			return err
		}
	}
	return nil
}

func NewMessage(terms MessageTerms, now Epoch, policy RelayerPolicy) (Message, error) {
	if err := terms.Validate(now, policy); err != nil {
		return Message{}, err
	}
	message := Message{
		ID:          terms.ID,
		Source:      terms.Source,
		Target:      terms.Target,
		Payer:       terms.Payer,
		Beneficiary: terms.Beneficiary,
		Asset:       terms.Asset,
		Amount:      terms.Amount,
		FeeLimit:    terms.FeeLimit,
		CreatedAt:   now,
		ExpiresAt:   terms.ExpiresAt,
		Status:      MessageOpen,
		RouteHint:   terms.RouteHint,
		Memo:        terms.Memo,
	}
	message.Digest = message.CanonicalDigest()
	return message, nil
}

func (m Message) Validate() error {
	terms := MessageTerms{
		ID:          m.ID,
		Source:      m.Source,
		Target:      m.Target,
		Payer:       m.Payer,
		Beneficiary: m.Beneficiary,
		Asset:       m.Asset,
		Amount:      m.Amount,
		FeeLimit:    m.FeeLimit,
		ExpiresAt:   m.ExpiresAt,
		RouteHint:   m.RouteHint,
		Memo:        m.Memo,
	}
	policy := DefaultPolicy()
	policy.MaxMessageTTL = ^uint64(0)
	if err := terms.Validate(m.CreatedAt, policy); err != nil {
		return err
	}
	if m.Digest != "" && m.Digest != m.CanonicalDigest() {
		return Invalid("message.validate", "message digest mismatch")
	}
	switch m.Status {
	case "", MessageOpen, MessageRouted, MessageDelivered, MessageSettled, MessageExpired, MessageCancelled:
		return nil
	default:
		return Invalid("message.validate", "unknown message status")
	}
}

func (m Message) LockedAmount() Amount {
	return m.Amount.MustAdd(m.FeeLimit)
}

func (m Message) ExpiredAt(now Epoch) bool {
	return now > m.ExpiresAt
}

func (m Message) CanonicalDigest() Digest {
	return DigestOf(
		m.ID.String(),
		m.Source.String(),
		m.Target.String(),
		m.Payer.String(),
		m.Beneficiary.String(),
		m.Asset.String(),
		m.Amount.String(),
		m.FeeLimit.String(),
		fmt.Sprintf("%d", m.CreatedAt),
		fmt.Sprintf("%d", m.ExpiresAt),
		m.RouteHint.String(),
		m.Memo,
	)
}

func (m *Message) MarkRouted(route RouteID) {
	if m.Status == MessageOpen {
		m.Status = MessageRouted
	}
	if m.RouteHint.IsZero() {
		m.RouteHint = route
	}
}

func (m *Message) MarkDelivered() {
	if m.Status != MessageExpired && m.Status != MessageCancelled {
		m.Status = MessageDelivered
	}
}

func (m *Message) MarkSettled() {
	if m.Status != MessageExpired && m.Status != MessageCancelled {
		m.Status = MessageSettled
	}
}

func (m *Message) MarkExpired() {
	m.Status = MessageExpired
}

func (m *Message) CanAcceptReceipt(now Epoch) bool {
	if m.Status == MessageCancelled || m.Status == MessageExpired {
		return false
	}
	return now <= m.ExpiresAt
}

type MessageBook struct {
	messages map[MessageID]*Message
}

func NewMessageBook() *MessageBook {
	return &MessageBook{messages: make(map[MessageID]*Message)}
}

func (b *MessageBook) Add(message Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	if _, ok := b.messages[message.ID]; ok {
		return AlreadyExists("messages.add", fmt.Sprintf("message %s already exists", message.ID))
	}
	copyMessage := message
	b.messages[message.ID] = &copyMessage
	return nil
}

func (b *MessageBook) Get(id MessageID) (*Message, bool) {
	message, ok := b.messages[id]
	return message, ok
}

func (b *MessageBook) Require(id MessageID) (*Message, error) {
	message, ok := b.messages[id]
	if !ok {
		return nil, NotFound("messages.require", fmt.Sprintf("message %s not found", id))
	}
	return message, nil
}

func (b *MessageBook) Snapshot() []Message {
	out := make([]Message, 0, len(b.messages))
	for _, message := range b.messages {
		out = append(out, *message)
	}
	return out
}
