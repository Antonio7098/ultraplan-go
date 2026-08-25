package app

import (
	"errors"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

// QAUseCaseError is the stable adapter-facing form of a QA domain failure.
// Cause remains available to trusted operation adapters without exposing
// persistence details through public query responses.
type QAUseCaseError struct {
	Code      string
	Category  string
	Message   string
	Guidance  string
	Operation string
	Retryable bool
	Cause     error
}

func (e *QAUseCaseError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *QAUseCaseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func AsQAUseCaseError(err error) (*QAUseCaseError, bool) {
	var target *QAUseCaseError
	ok := errors.As(err, &target)
	return target, ok
}

func mapQAUseCaseError(err error) error {
	qaErr, ok := sprint.AsQAError(err)
	if !ok {
		return err
	}
	return &QAUseCaseError{
		Code:      "qa." + string(qaErr.Category),
		Category:  string(qaErr.Category),
		Message:   qaPublicErrorMessage(qaErr.Category),
		Guidance:  qaErr.Recovery,
		Operation: qaErr.Operation,
		Retryable: qaErr.Category == sprint.QAErrorConflict || qaErr.Category == sprint.QAErrorRuntimeUnavailable,
		Cause:     err,
	}
}

func qaPublicErrorMessage(category sprint.QAErrorCategory) string {
	switch category {
	case sprint.QAErrorConflict:
		return "The QA state changed while the request was being processed."
	case sprint.QAErrorPersistenceFailure, sprint.QAErrorRuntimeUnavailable:
		return "The QA service is temporarily unavailable."
	default:
		return "The QA request is not valid for the current governed state."
	}
}
