package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type Scenario struct {
	Name     string            `json:"name,omitempty"`
	Now      Epoch             `json:"now,omitempty"`
	Policy   ScenarioPolicy    `json:"policy,omitempty"`
	Accounts []ScenarioAccount `json:"accounts,omitempty"`
	Nodes    []ScenarioNode    `json:"nodes,omitempty"`
	Routes   []ScenarioRoute   `json:"routes,omitempty"`
	Steps    []ScenarioStep    `json:"steps,omitempty"`
}

type ScenarioPolicy struct {
	NetworkID              string `json:"network_id,omitempty"`
	ReserveAccount         string `json:"reserve_account,omitempty"`
	QuorumWeight           uint64 `json:"quorum_weight,omitempty"`
	RejectWeight           uint64 `json:"reject_weight,omitempty"`
	MaxMessageTTL          uint64 `json:"max_message_ttl,omitempty"`
	MaxReceiptDrift        uint64 `json:"max_receipt_drift,omitempty"`
	MaxRouteLatency        uint64 `json:"max_route_latency,omitempty"`
	DefaultFeeBps          int64  `json:"default_fee_bps,omitempty"`
	AllowReserveCompletion *bool  `json:"allow_reserve_completion,omitempty"`
}

type ScenarioAccount struct {
	ID       string           `json:"id"`
	Kind     AccountKind      `json:"kind,omitempty"`
	Balances map[string]int64 `json:"balances,omitempty"`
}

type ScenarioNode struct {
	ID       string     `json:"id"`
	Weight   uint64     `json:"weight"`
	Status   NodeStatus `json:"status,omitempty"`
	Region   string     `json:"region,omitempty"`
	Operator string     `json:"operator,omitempty"`
}

type ScenarioHop struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Latency uint64 `json:"latency,omitempty"`
	FeeBps  int64  `json:"fee_bps,omitempty"`
}

type ScenarioRoute struct {
	ID          string        `json:"id"`
	Source      string        `json:"source"`
	Target      string        `json:"target"`
	Asset       string        `json:"asset"`
	Operator    string        `json:"operator"`
	Capacity    int64         `json:"capacity"`
	Reserved    int64         `json:"reserved,omitempty"`
	BaseFee     int64         `json:"base_fee,omitempty"`
	FeeBps      int64         `json:"fee_bps,omitempty"`
	Latency     uint64        `json:"latency,omitempty"`
	NotBefore   Epoch         `json:"not_before,omitempty"`
	NotAfter    Epoch         `json:"not_after,omitempty"`
	Status      RouteStatus   `json:"status,omitempty"`
	Hops        []ScenarioHop `json:"hops,omitempty"`
	Priority    int           `json:"priority,omitempty"`
	Description string        `json:"description,omitempty"`
}

type ScenarioStep struct {
	Op          string             `json:"op"`
	ExpectError bool               `json:"expect_error,omitempty"`
	At          Epoch              `json:"at,omitempty"`
	Delta       uint64             `json:"delta,omitempty"`
	To          Epoch              `json:"to,omitempty"`
	MessageID   string             `json:"message_id,omitempty"`
	ReceiptID   string             `json:"receipt_id,omitempty"`
	RouteID     string             `json:"route_id,omitempty"`
	ContextID   string             `json:"context_id,omitempty"`
	NodeID      string             `json:"node_id,omitempty"`
	Status      ConfirmationStatus `json:"status,omitempty"`
	Source      string             `json:"source,omitempty"`
	Target      string             `json:"target,omitempty"`
	Payer       string             `json:"payer,omitempty"`
	Beneficiary string             `json:"beneficiary,omitempty"`
	Operator    string             `json:"operator,omitempty"`
	Asset       string             `json:"asset,omitempty"`
	Amount      int64              `json:"amount,omitempty"`
	FeeLimit    int64              `json:"fee_limit,omitempty"`
	Fee         int64              `json:"fee,omitempty"`
	ExpiresAt   Epoch              `json:"expires_at,omitempty"`
	ObservedAt  Epoch              `json:"observed_at,omitempty"`
	RouteHint   string             `json:"route_hint,omitempty"`
	Memo        string             `json:"memo,omitempty"`
	Label       string             `json:"label,omitempty"`
}

type ScenarioStepResult struct {
	Index         int                       `json:"index"`
	Op            string                    `json:"op"`
	Label         string                    `json:"label,omitempty"`
	OK            bool                      `json:"ok"`
	ExpectedError bool                      `json:"expected_error,omitempty"`
	Error         string                    `json:"error,omitempty"`
	ErrorCode     ErrorCode                 `json:"error_code,omitempty"`
	Message       *Message                  `json:"message,omitempty"`
	Receipt       *Receipt                  `json:"receipt,omitempty"`
	Confirmation  *Confirmation             `json:"confirmation,omitempty"`
	Record        *ConfirmationRecordResult `json:"record,omitempty"`
	Settlement    *SettlementResult         `json:"settlement,omitempty"`
	Route         *Route                    `json:"route,omitempty"`
	View          *ConfirmationView         `json:"view,omitempty"`
}

type ScenarioResult struct {
	Name     string               `json:"name,omitempty"`
	OK       bool                 `json:"ok"`
	Error    string               `json:"error,omitempty"`
	Steps    []ScenarioStepResult `json:"steps"`
	Snapshot RelayerSnapshot      `json:"snapshot"`
}

func DecodeScenario(reader io.Reader) (Scenario, error) {
	var scenario Scenario
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scenario); err != nil {
		return Scenario{}, Wrap(ErrScenario, "scenario.decode", err)
	}
	return scenario, nil
}

func RunScenario(scenario Scenario) (ScenarioResult, error) {
	if scenario.Now == 0 {
		scenario.Now = 1
	}
	policy, err := scenario.Policy.ToPolicy()
	if err != nil {
		return ScenarioResult{}, err
	}
	relayer, err := NewRelayer(policy, scenario.Now)
	if err != nil {
		return ScenarioResult{}, err
	}
	if err := seedScenario(relayer, scenario); err != nil {
		return ScenarioResult{}, err
	}
	result := ScenarioResult{Name: scenario.Name, OK: true, Steps: make([]ScenarioStepResult, 0, len(scenario.Steps))}
	for index, step := range scenario.Steps {
		stepResult := executeScenarioStep(relayer, index, step)
		result.Steps = append(result.Steps, stepResult)
		if !stepResult.OK {
			result.OK = false
			result.Error = stepResult.Error
			result.Snapshot = relayer.Snapshot()
			return result, NewRelayError(ErrScenario, "scenario.run", stepResult.Error)
		}
	}
	result.Snapshot = relayer.Snapshot()
	return result, nil
}

func seedScenario(relayer *Relayer, scenario Scenario) error {
	for _, account := range scenario.Accounts {
		id, err := NewAccountID(account.ID)
		if err != nil {
			return err
		}
		balances := make(map[AssetID]Amount)
		for rawAsset, rawAmount := range account.Balances {
			asset, err := NewAssetID(rawAsset)
			if err != nil {
				return err
			}
			amount, err := NewAmount(rawAmount)
			if err != nil {
				return err
			}
			balances[asset] = amount
		}
		if err := relayer.RegisterAccount(id, account.Kind, balances); err != nil {
			return err
		}
	}
	for _, nodeSeed := range scenario.Nodes {
		id, err := NewNodeID(nodeSeed.ID)
		if err != nil {
			return err
		}
		operator := AccountID("")
		if nodeSeed.Operator != "" {
			operator, err = NewAccountID(nodeSeed.Operator)
			if err != nil {
				return err
			}
		}
		node, err := NewNode(id, nodeSeed.Weight, operator)
		if err != nil {
			return err
		}
		if nodeSeed.Status != "" {
			node.Status = nodeSeed.Status
		}
		node.Region = nodeSeed.Region
		if err := relayer.RegisterNode(node); err != nil {
			return err
		}
	}
	for _, routeSeed := range scenario.Routes {
		route, err := routeSeed.ToRoute()
		if err != nil {
			return err
		}
		if err := relayer.RegisterRoute(route); err != nil {
			return err
		}
	}
	return nil
}

func executeScenarioStep(relayer *Relayer, index int, step ScenarioStep) ScenarioStepResult {
	if step.At != 0 {
		relayer.SetNow(step.At)
	}
	out := ScenarioStepResult{Index: index, Op: step.Op, Label: step.Label, ExpectedError: step.ExpectError}
	var err error
	switch step.Op {
	case "advance":
		if step.To != 0 {
			relayer.SetNow(step.To)
		} else {
			relayer.Advance(step.Delta)
		}
	case "submit_message":
		var message Message
		message, err = relayer.SubmitMessage(step.ToMessageTerms(relayer.Now()))
		if err == nil {
			out.Message = &message
		}
	case "plan_route":
		var route Route
		route, err = relayer.PlanRoute(MessageID(step.MessageID))
		if err == nil {
			out.Route = &route
		}
	case "receipt":
		var receipt Receipt
		receipt, err = relayer.AcceptReceipt(step.ToReceiptDraft())
		if err == nil {
			out.Receipt = &receipt
		}
	case "confirm":
		var record ConfirmationRecordResult
		var confirmation Confirmation
		record, confirmation, err = relayer.ConfirmReceipt(ReceiptID(step.ReceiptID), NodeID(step.NodeID), step.Status, step.ObservedAt)
		if err == nil {
			out.Record = &record
			out.Confirmation = &confirmation
		}
	case "settle":
		var settlement SettlementResult
		settlement, err = relayer.SettleReceipt(ReceiptID(step.ReceiptID))
		if err == nil {
			out.Settlement = &settlement
		}
	case "expire":
		err = relayer.ExpireMessage(MessageID(step.MessageID))
	case "view":
		view := relayer.ConfirmationView(MessageID(step.MessageID))
		out.View = &view
	case "snapshot":
	default:
		err = Invalid("scenario.step", fmt.Sprintf("unknown op %q", step.Op))
	}
	if err != nil {
		out.Error = err.Error()
		out.ErrorCode = ErrorCodeOf(err)
		out.OK = step.ExpectError
		return out
	}
	if step.ExpectError {
		out.OK = false
		out.Error = "expected error but step succeeded"
		out.ErrorCode = ErrScenario
		return out
	}
	out.OK = true
	return out
}

func (p ScenarioPolicy) ToPolicy() (RelayerPolicy, error) {
	policy := DefaultPolicy()
	if p.NetworkID != "" {
		policy.NetworkID = p.NetworkID
	}
	if p.ReserveAccount != "" {
		id, err := NewAccountID(p.ReserveAccount)
		if err != nil {
			return RelayerPolicy{}, err
		}
		policy.ReserveAccount = id
	}
	if p.QuorumWeight != 0 {
		policy.QuorumWeight = p.QuorumWeight
	}
	if p.RejectWeight != 0 {
		policy.RejectWeight = p.RejectWeight
	}
	if p.MaxMessageTTL != 0 {
		policy.MaxMessageTTL = p.MaxMessageTTL
	}
	if p.MaxReceiptDrift != 0 {
		policy.MaxReceiptDrift = p.MaxReceiptDrift
	}
	if p.MaxRouteLatency != 0 {
		policy.MaxRouteLatency = p.MaxRouteLatency
	}
	if p.DefaultFeeBps != 0 {
		policy.DefaultFeeBps = p.DefaultFeeBps
	}
	if p.AllowReserveCompletion != nil {
		policy.AllowReserveCompletion = *p.AllowReserveCompletion
	}
	return policy, policy.Validate()
}

func (s ScenarioStep) ToMessageTerms(now Epoch) MessageTerms {
	expiresAt := s.ExpiresAt
	if expiresAt == 0 {
		expiresAt = now.Add(60)
	}
	routeHint := RouteID("")
	if s.RouteHint != "" {
		routeHint = RouteID(s.RouteHint)
	}
	return MessageTerms{
		ID:          MessageID(s.MessageID),
		Source:      NodeID(s.Source),
		Target:      NodeID(s.Target),
		Payer:       AccountID(s.Payer),
		Beneficiary: AccountID(s.Beneficiary),
		Asset:       AssetID(s.Asset),
		Amount:      Amount(s.Amount),
		FeeLimit:    Amount(s.FeeLimit),
		ExpiresAt:   expiresAt,
		RouteHint:   routeHint,
		Memo:        s.Memo,
	}
}

func (s ScenarioStep) ToReceiptDraft() ReceiptDraft {
	return ReceiptDraft{
		ID:         ReceiptID(s.ReceiptID),
		MessageID:  MessageID(s.MessageID),
		RouteID:    RouteID(s.RouteID),
		ContextID:  ContextID(s.ContextID),
		Operator:   AccountID(s.Operator),
		Fee:        Amount(s.Fee),
		ObservedAt: s.ObservedAt,
		ExpiresAt:  s.ExpiresAt,
	}
}

func (s ScenarioRoute) ToRoute() (Route, error) {
	id, err := NewRouteID(s.ID)
	if err != nil {
		return Route{}, err
	}
	source, err := NewNodeID(s.Source)
	if err != nil {
		return Route{}, err
	}
	target, err := NewNodeID(s.Target)
	if err != nil {
		return Route{}, err
	}
	asset, err := NewAssetID(s.Asset)
	if err != nil {
		return Route{}, err
	}
	operator, err := NewAccountID(s.Operator)
	if err != nil {
		return Route{}, err
	}
	capacity, err := NewAmount(s.Capacity)
	if err != nil {
		return Route{}, err
	}
	route, err := NewRoute(id, source, target, asset, operator, capacity)
	if err != nil {
		return Route{}, err
	}
	route.Reserved = Amount(s.Reserved)
	route.BaseFee = Amount(s.BaseFee)
	route.FeeBps = s.FeeBps
	route.Latency = s.Latency
	route.NotBefore = s.NotBefore
	route.NotAfter = s.NotAfter
	route.Status = s.Status
	route.Priority = s.Priority
	route.Description = s.Description
	if route.NotAfter == 0 {
		route.NotAfter = ^Epoch(0)
	}
	if route.Status == "" {
		route.Status = RouteActive
	}
	for _, hopSeed := range s.Hops {
		route.Hops = append(route.Hops, Hop{
			From:    NodeID(hopSeed.From),
			To:      NodeID(hopSeed.To),
			Latency: hopSeed.Latency,
			FeeBps:  hopSeed.FeeBps,
		})
	}
	if err := route.ValidateStatic(); err != nil {
		return Route{}, err
	}
	return route, nil
}

func DemoScenario() Scenario {
	return Scenario{
		Name: "demo-final-settlement",
		Now:  10,
		Policy: ScenarioPolicy{
			QuorumWeight: 2,
		},
		Accounts: []ScenarioAccount{
			{ID: "payer:alice", Kind: AccountUser, Balances: map[string]int64{"USDC": 10_000}},
			{ID: "beneficiary:bob", Kind: AccountUser, Balances: map[string]int64{"USDC": 0}},
			{ID: "operator:west", Kind: AccountOperator, Balances: map[string]int64{"USDC": 0}},
			{ID: "relay:reserve", Kind: AccountReserve, Balances: map[string]int64{"USDC": 50_000}},
		},
		Nodes: []ScenarioNode{
			{ID: "node:a", Weight: 1, Operator: "operator:west"},
			{ID: "node:b", Weight: 1, Operator: "operator:west"},
			{ID: "node:c", Weight: 1, Operator: "operator:west"},
		},
		Routes: []ScenarioRoute{
			{ID: "route:west", Source: "node:a", Target: "node:c", Asset: "USDC", Operator: "operator:west", Capacity: 5_000, FeeBps: 20, Latency: 5, Priority: 1},
		},
		Steps: []ScenarioStep{
			{Op: "submit_message", MessageID: "msg:demo", Source: "node:a", Target: "node:c", Payer: "payer:alice", Beneficiary: "beneficiary:bob", Asset: "USDC", Amount: 1_000, FeeLimit: 25, ExpiresAt: 80},
			{Op: "receipt", MessageID: "msg:demo", ReceiptID: "rcpt:demo", RouteID: "route:west"},
			{Op: "confirm", ReceiptID: "rcpt:demo", NodeID: "node:a"},
			{Op: "confirm", ReceiptID: "rcpt:demo", NodeID: "node:b"},
			{Op: "settle", ReceiptID: "rcpt:demo"},
		},
	}
}

func EncodeScenarioResult(writer io.Writer, result ScenarioResult, pretty bool) error {
	encoder := json.NewEncoder(writer)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(result)
}

func AmountFromJSONNumber(value json.RawMessage) (Amount, error) {
	var number int64
	if err := json.Unmarshal(value, &number); err == nil {
		return NewAmount(number)
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return 0, Wrap(ErrScenario, "scenario.amount", err)
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, Wrap(ErrScenario, "scenario.amount", err)
	}
	return NewAmount(parsed)
}
