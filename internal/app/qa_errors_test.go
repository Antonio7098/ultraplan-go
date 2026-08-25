package app

import (
	"errors"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestQAUseCaseErrorKeepsStablePublicCodeAndTypedCause(t *testing.T) {
	cause := errors.New("private persistence path")
	domain := sprint.NewQAError(sprint.QAErrorPersistenceFailure, "load", "private detail", cause)
	mapped := mapQAUseCaseError(domain)
	public, ok := AsQAUseCaseError(mapped)
	if !ok || public.Code != "qa.persistence_failure" || public.Message != "The QA service is temporarily unavailable." || public.Guidance == "" {
		t.Fatalf("public QA error = %+v", public)
	}
	if !errors.Is(mapped, cause) {
		t.Fatal("mapped QA error flattened its trusted typed cause")
	}
}
