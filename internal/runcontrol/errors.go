package runcontrol

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable run-control failure category.
type ErrorCode string

const (
	CodeInvalidArgument   ErrorCode = "invalid_argument"
	CodeNotFound          ErrorCode = "not_found"
	CodeConflict          ErrorCode = "conflict"
	CodeStaleFence        ErrorCode = "stale_fence"
	CodeTerminal          ErrorCode = "terminal"
	CodeUnavailable       ErrorCode = "unavailable"
	CodeBusy              ErrorCode = "busy"
	CodePermission        ErrorCode = "permission_denied"
	CodeCorrupt           ErrorCode = "corrupt"
	CodeUnsupportedSchema ErrorCode = "unsupported_schema"
	CodeInvariant         ErrorCode = "invariant_violation"
	CodeQuota             ErrorCode = "quota_exceeded"
)

var (
	ErrInvalidArgument   = &Error{Code: CodeInvalidArgument}
	ErrNotFound          = &Error{Code: CodeNotFound}
	ErrConflict          = &Error{Code: CodeConflict}
	ErrStaleFence        = &Error{Code: CodeStaleFence}
	ErrTerminal          = &Error{Code: CodeTerminal}
	ErrUnavailable       = &Error{Code: CodeUnavailable}
	ErrBusy              = &Error{Code: CodeBusy}
	ErrPermission        = &Error{Code: CodePermission}
	ErrCorrupt           = &Error{Code: CodeCorrupt}
	ErrUnsupportedSchema = &Error{Code: CodeUnsupportedSchema}
	ErrInvariant         = &Error{Code: CodeInvariant}
	ErrQuota             = &Error{Code: CodeQuota}
)

// Error is safe to classify without parsing driver or filesystem messages.
// Cause is retained for local operator diagnostics but is never a public DTO.
type Error struct {
	Code      ErrorCode
	Operation string
	RunID     RunID
	Message   string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Message
	if message == "" {
		message = string(e.Code)
	}
	if e.Operation == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", e.Operation, message)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *Error) Is(target error) bool {
	var other *Error
	return errors.As(target, &other) && e != nil && e.Code != "" && e.Code == other.Code
}

func wrapError(code ErrorCode, operation, message string, retryable bool, cause error) error {
	return &Error{Code: code, Operation: operation, Message: message, Retryable: retryable, Cause: cause}
}

func runError(code ErrorCode, operation string, runID RunID, message string, retryable bool, cause error) error {
	return &Error{Code: code, Operation: operation, RunID: runID, Message: message, Retryable: retryable, Cause: cause}
}

func invalidField(field, message string) error {
	return wrapError(CodeInvalidArgument, "validate", field+" "+message, false, nil)
}
