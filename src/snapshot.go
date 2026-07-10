package relay

import "sort"

type RelayerSnapshot struct {
	Now           Epoch                             `json:"now"`
	PolicyDigest  Digest                            `json:"policy_digest"`
	Accounts      []AccountSnapshot                 `json:"accounts"`
	Balances      map[AccountID]map[AssetID]Balance `json:"balances"`
	Totals        map[AssetID]Amount                `json:"totals"`
	Nodes         []Node                            `json:"nodes"`
	Routes        []Route                           `json:"routes"`
	Messages      []Message                         `json:"messages"`
	Receipts      []Receipt                         `json:"receipts"`
	Confirmations []Confirmation                    `json:"confirmations"`
	Effects       []ReceiptEffect                   `json:"effects"`
	Events        []Event                           `json:"events"`
}

func (r *Relayer) Snapshot() RelayerSnapshot {
	nodes := r.nodes.Snapshot()
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	messages := r.messages.Snapshot()
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].ID < messages[j].ID
	})
	receipts := r.receipts.Snapshot()
	sort.Slice(receipts, func(i, j int) bool {
		return receipts[i].ID < receipts[j].ID
	})
	effects := r.settlement.Snapshot()
	sort.Slice(effects, func(i, j int) bool {
		return effects[i].ReceiptID < effects[j].ReceiptID
	})
	return RelayerSnapshot{
		Now:           r.Now(),
		PolicyDigest:  r.policy.CanonicalDigest(),
		Accounts:      r.ledger.Snapshot(),
		Balances:      r.ledger.BalancesMap(),
		Totals:        r.ledger.TotalByAsset(),
		Nodes:         nodes,
		Routes:        r.routes.Snapshot(),
		Messages:      messages,
		Receipts:      receipts,
		Confirmations: r.confirmations.Snapshot(),
		Effects:       effects,
		Events:        r.ledger.Events().All(),
	}
}
