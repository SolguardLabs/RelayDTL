package relay

import "fmt"

type RelayerPolicy struct {
	NetworkID              string    `json:"network_id"`
	ReserveAccount         AccountID `json:"reserve_account"`
	QuorumWeight           uint64    `json:"quorum_weight"`
	RejectWeight           uint64    `json:"reject_weight"`
	MaxMessageTTL          uint64    `json:"max_message_ttl"`
	MaxReceiptDrift        uint64    `json:"max_receipt_drift"`
	MaxRouteLatency        uint64    `json:"max_route_latency"`
	DefaultFeeBps          int64     `json:"default_fee_bps"`
	AllowReserveCompletion bool      `json:"allow_reserve_completion"`
}

func DefaultPolicy() RelayerPolicy {
	return RelayerPolicy{
		NetworkID:              "relaydtl-local",
		ReserveAccount:         AccountID("relay:reserve"),
		QuorumWeight:           2,
		RejectWeight:           2,
		MaxMessageTTL:          120,
		MaxReceiptDrift:        10,
		MaxRouteLatency:        40,
		DefaultFeeBps:          30,
		AllowReserveCompletion: true,
	}
}

func (p RelayerPolicy) Validate() error {
	if p.NetworkID == "" {
		return Invalid("policy.validate", "network id required")
	}
	if err := RequireAccount(p.ReserveAccount); err != nil {
		return err
	}
	if p.QuorumWeight == 0 {
		return Invalid("policy.validate", "quorum weight required")
	}
	if p.RejectWeight == 0 {
		return Invalid("policy.validate", "reject weight required")
	}
	if p.MaxMessageTTL == 0 {
		return Invalid("policy.validate", "max message ttl required")
	}
	if p.MaxReceiptDrift > p.MaxMessageTTL {
		return Invalid("policy.validate", "receipt drift exceeds ttl")
	}
	if p.MaxRouteLatency == 0 {
		return Invalid("policy.validate", "max route latency required")
	}
	if p.DefaultFeeBps < 0 || p.DefaultFeeBps > BpsScale {
		return Invalid("policy.validate", "default fee bps out of range")
	}
	return nil
}

func (p RelayerPolicy) CanonicalDigest() Digest {
	return DigestOf(
		p.NetworkID,
		p.ReserveAccount.String(),
		fmt.Sprintf("%d", p.QuorumWeight),
		fmt.Sprintf("%d", p.RejectWeight),
		fmt.Sprintf("%d", p.MaxMessageTTL),
		fmt.Sprintf("%d", p.MaxReceiptDrift),
		fmt.Sprintf("%d", p.MaxRouteLatency),
		fmt.Sprintf("%d", p.DefaultFeeBps),
		fmt.Sprintf("%t", p.AllowReserveCompletion),
	)
}

type NodeStatus string

const (
	NodeActive  NodeStatus = "active"
	NodePaused  NodeStatus = "paused"
	NodeRetired NodeStatus = "retired"
)

type Node struct {
	ID        NodeID     `json:"id"`
	Weight    uint64     `json:"weight"`
	Status    NodeStatus `json:"status"`
	Region    string     `json:"region,omitempty"`
	Operator  AccountID  `json:"operator,omitempty"`
	LastEpoch Epoch      `json:"last_epoch,omitempty"`
}

func NewNode(id NodeID, weight uint64, operator AccountID) (Node, error) {
	if err := RequireNode(id); err != nil {
		return Node{}, err
	}
	if weight == 0 {
		return Node{}, Invalid("node.new", "node weight required")
	}
	if !operator.IsZero() {
		if err := RequireAccount(operator); err != nil {
			return Node{}, err
		}
	}
	return Node{
		ID:       id,
		Weight:   weight,
		Status:   NodeActive,
		Operator: operator,
	}, nil
}

func (n Node) Active() bool {
	return n.Status == "" || n.Status == NodeActive
}

func (n Node) Validate() error {
	if err := RequireNode(n.ID); err != nil {
		return err
	}
	if n.Weight == 0 {
		return Invalid("node.validate", "node weight required")
	}
	switch n.Status {
	case "", NodeActive, NodePaused, NodeRetired:
		return nil
	default:
		return Invalid("node.validate", "unknown node status")
	}
}

type NodeRegistry struct {
	nodes map[NodeID]Node
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{nodes: make(map[NodeID]Node)}
}

func (r *NodeRegistry) Register(node Node) error {
	if err := node.Validate(); err != nil {
		return err
	}
	r.nodes[node.ID] = node
	return nil
}

func (r *NodeRegistry) Get(id NodeID) (Node, bool) {
	node, ok := r.nodes[id]
	return node, ok
}

func (r *NodeRegistry) Require(id NodeID) (Node, error) {
	node, ok := r.nodes[id]
	if !ok {
		return Node{}, NotFound("nodes.require", fmt.Sprintf("node %s not found", id))
	}
	return node, nil
}

func (r *NodeRegistry) ActiveWeight(ids []NodeID) uint64 {
	var total uint64
	seen := make(map[NodeID]struct{})
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		node, ok := r.nodes[id]
		if !ok || !node.Active() {
			continue
		}
		total += node.Weight
	}
	return total
}

func (r *NodeRegistry) Snapshot() []Node {
	out := make([]Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		out = append(out, node)
	}
	return out
}
