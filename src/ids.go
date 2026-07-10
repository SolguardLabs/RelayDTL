package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type AssetID string
type AccountID string
type NodeID string
type MessageID string
type ReceiptID string
type RouteID string
type ContextID string
type Digest string
type Epoch uint64

var tokenPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9:_\-.]{1,96}$`)

func NormalizeToken(value string) string {
	return strings.TrimSpace(value)
}

func validateToken(kind string, value string) error {
	value = NormalizeToken(value)
	if value == "" {
		return Invalid("validate."+kind, "empty identifier")
	}
	if !tokenPattern.MatchString(value) {
		return Invalid("validate."+kind, fmt.Sprintf("invalid identifier %q", value))
	}
	return nil
}

func NewAssetID(value string) (AssetID, error) {
	value = strings.ToUpper(NormalizeToken(value))
	if err := validateToken("asset", value); err != nil {
		return "", err
	}
	return AssetID(value), nil
}

func NewAccountID(value string) (AccountID, error) {
	value = NormalizeToken(value)
	if err := validateToken("account", value); err != nil {
		return "", err
	}
	return AccountID(value), nil
}

func NewNodeID(value string) (NodeID, error) {
	value = NormalizeToken(value)
	if err := validateToken("node", value); err != nil {
		return "", err
	}
	return NodeID(value), nil
}

func NewMessageID(value string) (MessageID, error) {
	value = NormalizeToken(value)
	if err := validateToken("message", value); err != nil {
		return "", err
	}
	return MessageID(value), nil
}

func NewReceiptID(value string) (ReceiptID, error) {
	value = NormalizeToken(value)
	if err := validateToken("receipt", value); err != nil {
		return "", err
	}
	return ReceiptID(value), nil
}

func NewRouteID(value string) (RouteID, error) {
	value = NormalizeToken(value)
	if err := validateToken("route", value); err != nil {
		return "", err
	}
	return RouteID(value), nil
}

func NewContextID(value string) (ContextID, error) {
	value = NormalizeToken(value)
	if err := validateToken("context", value); err != nil {
		return "", err
	}
	return ContextID(value), nil
}

func (id AssetID) String() string {
	return string(id)
}

func (id AccountID) String() string {
	return string(id)
}

func (id NodeID) String() string {
	return string(id)
}

func (id MessageID) String() string {
	return string(id)
}

func (id ReceiptID) String() string {
	return string(id)
}

func (id RouteID) String() string {
	return string(id)
}

func (id ContextID) String() string {
	return string(id)
}

func (id Digest) String() string {
	return string(id)
}

func (id AssetID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id AccountID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id NodeID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id MessageID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id ReceiptID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id RouteID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func (id ContextID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

func RequireAsset(id AssetID) error {
	return validateToken("asset", id.String())
}

func RequireAccount(id AccountID) error {
	return validateToken("account", id.String())
}

func RequireNode(id NodeID) error {
	return validateToken("node", id.String())
}

func RequireMessage(id MessageID) error {
	return validateToken("message", id.String())
}

func RequireReceipt(id ReceiptID) error {
	return validateToken("receipt", id.String())
}

func RequireRoute(id RouteID) error {
	return validateToken("route", id.String())
}

func RequireContext(id ContextID) error {
	return validateToken("context", id.String())
}

func DigestOf(parts ...string) Digest {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.TrimSpace(part))
	}
	h := sha256.Sum256([]byte(strings.Join(normalized, "|")))
	return Digest(hex.EncodeToString(h[:]))
}

func DigestMap(values map[string]string) Digest {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		parts = append(parts, key, values[key])
	}
	return DigestOf(parts...)
}

func JoinIDs[T ~string](ids []T) string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, string(id))
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func NextReceiptID(message MessageID, route RouteID, ordinal int) ReceiptID {
	return ReceiptID(fmt.Sprintf("%s:%s:%03d", message, route, ordinal))
}

func NextContextID(route RouteID, epoch Epoch, suffix string) ContextID {
	if suffix == "" {
		suffix = "default"
	}
	return ContextID(fmt.Sprintf("%s:%d:%s", route, epoch, suffix))
}

func EpochFromUint(value uint64) Epoch {
	return Epoch(value)
}

func (e Epoch) Uint64() uint64 {
	return uint64(e)
}

func (e Epoch) Add(delta uint64) Epoch {
	return Epoch(uint64(e) + delta)
}

func (e Epoch) Before(other Epoch) bool {
	return e < other
}

func (e Epoch) After(other Epoch) bool {
	return e > other
}

func (e Epoch) Between(start Epoch, end Epoch) bool {
	return e >= start && e <= end
}
