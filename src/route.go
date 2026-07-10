package relay

import (
	"fmt"
	"sort"
)

type Hop struct {
	From    NodeID `json:"from"`
	To      NodeID `json:"to"`
	Latency uint64 `json:"latency"`
	FeeBps  int64  `json:"fee_bps"`
}

func (h Hop) Validate() error {
	if err := RequireNode(h.From); err != nil {
		return err
	}
	if err := RequireNode(h.To); err != nil {
		return err
	}
	if h.From == h.To {
		return Invalid("hop.validate", "hop endpoints must differ")
	}
	if h.FeeBps < 0 || h.FeeBps > BpsScale {
		return Invalid("hop.validate", "hop fee bps out of range")
	}
	return nil
}

type RouteStatus string

const (
	RouteActive  RouteStatus = "active"
	RoutePaused  RouteStatus = "paused"
	RouteDrained RouteStatus = "drained"
)

type Route struct {
	ID          RouteID     `json:"id"`
	Source      NodeID      `json:"source"`
	Target      NodeID      `json:"target"`
	Asset       AssetID     `json:"asset"`
	Operator    AccountID   `json:"operator"`
	Capacity    Amount      `json:"capacity"`
	Reserved    Amount      `json:"reserved"`
	BaseFee     Amount      `json:"base_fee"`
	FeeBps      int64       `json:"fee_bps"`
	Latency     uint64      `json:"latency"`
	NotBefore   Epoch       `json:"not_before"`
	NotAfter    Epoch       `json:"not_after"`
	Status      RouteStatus `json:"status"`
	Hops        []Hop       `json:"hops,omitempty"`
	Priority    int         `json:"priority"`
	Description string      `json:"description,omitempty"`
}

func NewRoute(id RouteID, source NodeID, target NodeID, asset AssetID, operator AccountID, capacity Amount) (Route, error) {
	route := Route{
		ID:        id,
		Source:    source,
		Target:    target,
		Asset:     asset,
		Operator:  operator,
		Capacity:  capacity,
		Status:    RouteActive,
		NotBefore: 0,
		NotAfter:  ^Epoch(0),
	}
	if err := route.ValidateStatic(); err != nil {
		return Route{}, err
	}
	return route, nil
}

func (r Route) ValidateStatic() error {
	if err := RequireRoute(r.ID); err != nil {
		return err
	}
	if err := RequireNode(r.Source); err != nil {
		return err
	}
	if err := RequireNode(r.Target); err != nil {
		return err
	}
	if r.Source == r.Target {
		return Invalid("route.validate", "route endpoints must differ")
	}
	if err := RequireAsset(r.Asset); err != nil {
		return err
	}
	if err := RequireAccount(r.Operator); err != nil {
		return err
	}
	if err := r.Capacity.Validate("route.capacity"); err != nil {
		return err
	}
	if err := r.Reserved.Validate("route.reserved"); err != nil {
		return err
	}
	if r.Reserved > r.Capacity {
		return Invalid("route.validate", "reserved exceeds capacity")
	}
	if r.FeeBps < 0 || r.FeeBps > BpsScale {
		return Invalid("route.validate", "route fee bps out of range")
	}
	if r.NotAfter != 0 && r.NotBefore > r.NotAfter {
		return Invalid("route.validate", "invalid route epoch window")
	}
	switch r.Status {
	case "", RouteActive, RoutePaused, RouteDrained:
	default:
		return Invalid("route.validate", "unknown route status")
	}
	for _, hop := range r.Hops {
		if err := hop.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r Route) ActiveAt(now Epoch) bool {
	status := r.Status
	if status == "" {
		status = RouteActive
	}
	if status != RouteActive {
		return false
	}
	if now < r.NotBefore {
		return false
	}
	if r.NotAfter != 0 && now > r.NotAfter {
		return false
	}
	return true
}

func (r Route) Available() Amount {
	if r.Reserved >= r.Capacity {
		return 0
	}
	return r.Capacity.MustSub(r.Reserved)
}

func (r Route) CanCarry(message Message, now Epoch) bool {
	if !r.ActiveAt(now) {
		return false
	}
	if r.Source != message.Source || r.Target != message.Target {
		return false
	}
	if r.Asset != message.Asset {
		return false
	}
	if r.Available() < message.Amount {
		return false
	}
	if now.Add(r.Latency) > message.ExpiresAt {
		return false
	}
	return true
}

func (r Route) FeeFor(amount Amount) (Amount, error) {
	var fee Amount
	if r.BaseFee > 0 {
		fee = fee.MustAdd(r.BaseFee)
	}
	if r.FeeBps > 0 {
		variable, err := amount.CeilMulBps(r.FeeBps)
		if err != nil {
			return 0, err
		}
		fee = fee.MustAdd(variable)
	}
	for _, hop := range r.Hops {
		if hop.FeeBps == 0 {
			continue
		}
		hopFee, err := amount.CeilMulBps(hop.FeeBps)
		if err != nil {
			return 0, err
		}
		fee = fee.MustAdd(hopFee)
	}
	return fee, nil
}

func (r Route) Digest() Digest {
	parts := []string{
		r.ID.String(),
		r.Source.String(),
		r.Target.String(),
		r.Asset.String(),
		r.Operator.String(),
		r.Capacity.String(),
		r.BaseFee.String(),
		fmt.Sprintf("%d", r.FeeBps),
		fmt.Sprintf("%d", r.Latency),
		fmt.Sprintf("%d", r.NotBefore),
		fmt.Sprintf("%d", r.NotAfter),
	}
	for _, hop := range r.Hops {
		parts = append(parts, hop.From.String(), hop.To.String(), fmt.Sprintf("%d", hop.Latency), fmt.Sprintf("%d", hop.FeeBps))
	}
	return DigestOf(parts...)
}

type RouteBook struct {
	routes map[RouteID]*Route
}

func NewRouteBook() *RouteBook {
	return &RouteBook{routes: make(map[RouteID]*Route)}
}

func (b *RouteBook) Add(route Route) error {
	if err := route.ValidateStatic(); err != nil {
		return err
	}
	copyRoute := route
	b.routes[route.ID] = &copyRoute
	return nil
}

func (b *RouteBook) Get(id RouteID) (*Route, bool) {
	route, ok := b.routes[id]
	return route, ok
}

func (b *RouteBook) Require(id RouteID) (*Route, error) {
	route, ok := b.routes[id]
	if !ok {
		return nil, NotFound("routes.require", fmt.Sprintf("route %s not found", id))
	}
	return route, nil
}

func (b *RouteBook) Select(message Message, now Epoch) (*Route, error) {
	candidates := make([]*Route, 0)
	for _, route := range b.routes {
		if route.CanCarry(message, now) {
			candidates = append(candidates, route)
		}
	}
	if len(candidates) == 0 {
		return nil, NewRelayError(ErrRouteUnavailable, "routes.select", "no route can carry message")
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftFee, _ := candidates[i].FeeFor(message.Amount)
		rightFee, _ := candidates[j].FeeFor(message.Amount)
		if leftFee != rightFee {
			return leftFee < rightFee
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0], nil
}

func (b *RouteBook) Reserve(id RouteID, amount Amount, now Epoch, events *EventLog) error {
	route, err := b.Require(id)
	if err != nil {
		return err
	}
	if !route.ActiveAt(now) {
		return NewRelayError(ErrRouteUnavailable, "routes.reserve", "route is not active")
	}
	if route.Available() < amount {
		return NewRelayError(ErrRouteUnavailable, "routes.reserve", "route capacity exhausted")
	}
	route.Reserved = route.Reserved.MustAdd(amount)
	if events != nil {
		events.Append(Event{Kind: EventRouteReserved, At: now, RouteID: id, Asset: route.Asset, Amount: amount})
	}
	return nil
}

func (b *RouteBook) Release(id RouteID, amount Amount, now Epoch, events *EventLog) error {
	route, err := b.Require(id)
	if err != nil {
		return err
	}
	if route.Reserved < amount {
		amount = route.Reserved
	}
	route.Reserved = route.Reserved.MustSub(amount)
	if events != nil && amount > 0 {
		events.Append(Event{Kind: EventRouteReleased, At: now, RouteID: id, Asset: route.Asset, Amount: amount})
	}
	return nil
}

func (b *RouteBook) Snapshot() []Route {
	ids := make([]RouteID, 0, len(b.routes))
	for id := range b.routes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	out := make([]Route, 0, len(ids))
	for _, id := range ids {
		out = append(out, *b.routes[id])
	}
	return out
}
