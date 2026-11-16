package integrations

import (
	"fmt"
	"sync/atomic"
)

// HRClient is the boundary to the third-party HR reimbursement system.
type HRClient interface {
	SubmitClaim(employeeID string, amount float64, currency, description, receiptURL string) (string, error)
}

type MockHR struct{ sequence atomic.Uint64 }

func (m *MockHR) SubmitClaim(employeeID string, amount float64, currency, description, receiptURL string) (string, error) {
	if employeeID == "" || amount <= 0 || currency == "" || receiptURL == "" {
		return "", fmt.Errorf("invalid reimbursement claim")
	}
	return fmt.Sprintf("HR-%06d", m.sequence.Add(1)), nil
}

// FXConverter converts an office purchase amount into the employee's payroll currency.
// Production implementation should use an approved, auditable exchange-rate source.
type FXConverter interface {
	Convert(amount float64, from, to string, atTime string) (float64, error)
}

type SameCurrencyOnly struct{}

func (SameCurrencyOnly) Convert(amount float64, from, to string, _ string) (float64, error) {
	if from != to {
		return 0, fmt.Errorf("FX rate required to convert %s to %s", from, to)
	}
	return amount, nil
}
