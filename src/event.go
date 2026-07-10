package relay

import "sort"

type EventKind string

const (
	EventAccountCreated      EventKind = "account_created"
	EventCredit              EventKind = "credit"
	EventDebit               EventKind = "debit"
	EventHold                EventKind = "hold"
	EventRelease             EventKind = "release"
	EventReserveDebit        EventKind = "reserve_debit"
	EventMessageSubmitted    EventKind = "message_submitted"
	EventMessageExpired      EventKind = "message_expired"
	EventReceiptAccepted     EventKind = "receipt_accepted"
	EventConfirmationAdded   EventKind = "confirmation_added"
	EventConfirmationIgnored EventKind = "confirmation_ignored"
	EventSettlementApplied   EventKind = "settlement_applied"
	EventRouteReserved       EventKind = "route_reserved"
	EventRouteReleased       EventKind = "route_released"
)

type Event struct {
	Seq       uint64            `json:"seq"`
	Kind      EventKind         `json:"kind"`
	At        Epoch             `json:"at"`
	Account   AccountID         `json:"account,omitempty"`
	Asset     AssetID           `json:"asset,omitempty"`
	Amount    Amount            `json:"amount,omitempty"`
	MessageID MessageID         `json:"message_id,omitempty"`
	ReceiptID ReceiptID         `json:"receipt_id,omitempty"`
	RouteID   RouteID           `json:"route_id,omitempty"`
	NodeID    NodeID            `json:"node_id,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

type EventLog struct {
	next   uint64
	events []Event
}

func NewEventLog() *EventLog {
	return &EventLog{next: 1, events: make([]Event, 0)}
}

func (l *EventLog) Append(event Event) Event {
	if event.Seq == 0 {
		event.Seq = l.next
		l.next++
	}
	if event.Meta != nil && len(event.Meta) == 0 {
		event.Meta = nil
	}
	l.events = append(l.events, event)
	return event
}

func (l *EventLog) All() []Event {
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

func (l *EventLog) Since(seq uint64) []Event {
	out := make([]Event, 0)
	for _, event := range l.events {
		if event.Seq >= seq {
			out = append(out, event)
		}
	}
	return out
}

func (l *EventLog) Len() int {
	return len(l.events)
}

func (l *EventLog) Clear() {
	l.next = 1
	l.events = l.events[:0]
}

func (l *EventLog) Kinds() []EventKind {
	set := make(map[EventKind]struct{})
	for _, event := range l.events {
		set[event.Kind] = struct{}{}
	}
	kinds := make([]EventKind, 0, len(set))
	for kind := range set {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	return kinds
}
