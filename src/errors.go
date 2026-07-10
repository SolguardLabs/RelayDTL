package relay

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrInvalidInput      ErrorCode = "invalid_input"
	ErrNotFound          ErrorCode = "not_found"
	ErrAlreadyExists     ErrorCode = "already_exists"
	ErrInsufficientFunds ErrorCode = "insufficient_funds"
	ErrPolicy            ErrorCode = "policy"
	ErrExpired           ErrorCode = "expired"
	ErrNotReady          ErrorCode = "not_ready"
	ErrSettlementClosed  ErrorCode = "settlement_closed"
	ErrConfirmation      ErrorCode = "confirmation"
	ErrRouteUnavailable  ErrorCode = "route_unavailable"
	ErrConservation      ErrorCode = "conservation"
	ErrScenario          ErrorCode = "scenario"
)

type RelayError struct {
	Code   ErrorCode `json:"code"`
	Op     string    `json:"op"`
	Detail string    `json:"detail"`
}

func (e RelayError) Error() string {
	if e.Op == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Detail)
	}
	return fmt.Sprintf("%s: %s: %s", e.Code, e.Op, e.Detail)
}

func NewRelayError(code ErrorCode, op string, detail string) error {
	return RelayError{Code: code, Op: op, Detail: detail}
}

func Invalid(op string, detail string) error {
	return NewRelayError(ErrInvalidInput, op, detail)
}

func NotFound(op string, detail string) error {
	return NewRelayError(ErrNotFound, op, detail)
}

func AlreadyExists(op string, detail string) error {
	return NewRelayError(ErrAlreadyExists, op, detail)
}

func PolicyError(op string, detail string) error {
	return NewRelayError(ErrPolicy, op, detail)
}

func Expired(op string, detail string) error {
	return NewRelayError(ErrExpired, op, detail)
}

func NotReady(op string, detail string) error {
	return NewRelayError(ErrNotReady, op, detail)
}

func Wrap(code ErrorCode, op string, err error) error {
	if err == nil {
		return nil
	}
	var relayErr RelayError
	if errors.As(err, &relayErr) {
		if relayErr.Op == "" {
			relayErr.Op = op
		} else if op != "" {
			relayErr.Op = op + "." + relayErr.Op
		}
		return relayErr
	}
	return RelayError{Code: code, Op: op, Detail: err.Error()}
}

func ErrorCodeOf(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var relayErr RelayError
	if errors.As(err, &relayErr) {
		return relayErr.Code
	}
	return ErrInvalidInput
}

func ErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	var relayErr RelayError
	if errors.As(err, &relayErr) {
		return relayErr.Detail
	}
	return err.Error()
}
