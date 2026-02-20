package providers

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	cases := map[string]ErrorType{
		"insufficient_quota": ErrorQuota,
		"429 rate":           ErrorRate,
		"context too long":   ErrorContext,
		"timeout":            ErrorTransient,
		"bad request":        ErrorPermanent,
	}
	for msg, want := range cases {
		if got := ClassifyError(errors.New(msg)); got != want {
			t.Fatalf("classify %q: got %s want %s", msg, got, want)
		}
	}
}
