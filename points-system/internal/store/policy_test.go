package store

import (
	"fmt"
	"testing"

	"github.com/hxly520/sub2api/points-system/internal/domain"
)

func TestValidatePolicyUsesDirectPointsPerUSDRatio(t *testing.T) {
	policy := domain.Policy{
		Enabled:                false,
		Mode:                   domain.PolicyModeAllUsers,
		Basis:                  domain.PolicyBasisYesterday,
		PointsPerUSDHundredths: 1_025,
		RefreshMinute:          5,
	}
	if err := validatePolicy(policy); err != nil {
		t.Fatalf("valid 10.25 points/U policy rejected: %v", err)
	}
	policy.PointsPerUSDHundredths = 0
	if err := validatePolicy(policy); err == nil {
		t.Fatal("zero points/U ratio was accepted")
	}
}

type sqlStateError string

func (e sqlStateError) Error() string    { return string(e) }
func (e sqlStateError) SQLState() string { return string(e) }

func TestRetryableTransactionFailureIncludesSerializationAndDeadlock(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		err := fmt.Errorf("wrapped transaction error: %w", sqlStateError(code))
		if !IsRetryableTransactionFailure(err) {
			t.Fatalf("SQLSTATE %s was not retryable", code)
		}
	}
	for _, err := range []error{sqlStateError("23505"), fmt.Errorf("ordinary error")} {
		if IsRetryableTransactionFailure(err) {
			t.Fatalf("non-retryable error was accepted: %v", err)
		}
	}
}
