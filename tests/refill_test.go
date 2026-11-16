package tests

import (
	"sync"
	"testing"
	"time"

	"snack-counter-automation/internal/integrations"
	"snack-counter-automation/internal/models"
	"snack-counter-automation/internal/service"
)

func newService() *service.Service {
	return service.New(&integrations.MockChat{}, &integrations.MockHR{}, integrations.SameCurrencyOnly{})
}
func addVolunteer(t *testing.T, s *service.Service, id string, office models.Office, currency models.Currency) {
	t.Helper()
	if err := s.AddVolunteer(models.Volunteer{ID: id, Name: id, HomeOffice: office, CurrentOffice: office, PayrollCurrency: currency, Available: true}); err != nil {
		t.Fatal(err)
	}
}

func TestTwoVolunteersCannotBothCommitSameRefill(t *testing.T) {
	s := newService()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	addVolunteer(t, s, "v1", models.Oslo, models.NOK)
	addVolunteer(t, s, "v2", models.Oslo, models.NOK)
	r, err := s.ReportEmpty(models.Oslo, "Tea", now)
	if err != nil {
		t.Fatal(err)
	}
	assigned := r.AssignedVolunteer
	other := "v1"
	if assigned == other {
		other = "v2"
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, volunteerID := range []string{assigned, other} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := s.CompletePurchase(r.ID, id, 100, 2, "receipt-"+id, now)
			results <- err
		}(volunteerID)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one volunteer to commit purchase, got %d", successes)
	}
	if remaining := s.RemainingBudget(models.Oslo, now); remaining != 5900 {
		t.Fatalf("expected budget to be charged once, remaining %.2f", remaining)
	}
}

func TestExpiredRefillReassignedToDifferentVolunteer(t *testing.T) {
	s := newService()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	addVolunteer(t, s, "v1", models.Oslo, models.NOK)
	addVolunteer(t, s, "v2", models.Oslo, models.NOK)
	r, err := s.ReportEmpty(models.Oslo, "Coffee", now)
	if err != nil {
		t.Fatal(err)
	}
	first := r.AssignedVolunteer
	changed := s.ReassignExpired(now.Add(25 * time.Hour))
	if len(changed) != 1 {
		t.Fatalf("expected one reassignment, got %d", len(changed))
	}
	if changed[0].AssignedVolunteer == first {
		t.Fatal("task was reassigned to the same volunteer")
	}
}

func TestBudgetsAndMonthlyTotalsAcrossCurrencies(t *testing.T) {
	s := newService()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	addVolunteer(t, s, "oslo-1", models.Oslo, models.NOK)
	addVolunteer(t, s, "blr-1", models.Bengaluru, models.INR)
	r1, _ := s.ReportEmpty(models.Oslo, "Fruit", now)
	if _, err := s.CompletePurchase(r1.ID, "oslo-1", 1000, 10, "receipt-oslo", now); err != nil {
		t.Fatal(err)
	}
	r2, _ := s.ReportEmpty(models.Bengaluru, "Biscuits", now)
	if _, err := s.CompletePurchase(r2.ID, "blr-1", 2500, 20, "receipt-blr", now); err != nil {
		t.Fatal(err)
	}
	report := s.MonthlyReport(2026, time.November)
	if report.ByOffice[models.Oslo] != 1000 || report.ByOffice[models.Bengaluru] != 2500 {
		t.Fatalf("unexpected office totals: %#v", report.ByOffice)
	}
	if report.ByCurrency[models.NOK] != 1000 || report.ByCurrency[models.INR] != 2500 {
		t.Fatalf("unexpected currency totals: %#v", report.ByCurrency)
	}
	if s.RemainingBudget(models.Oslo, now) != 5000 || s.RemainingBudget(models.Bengaluru, now) != 12500 {
		t.Fatal("remaining budget calculation is incorrect")
	}
}

func TestPurchaseBlockedWhenBudgetExceeded(t *testing.T) {
	s := newService()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	addVolunteer(t, s, "v1", models.Oslo, models.NOK)
	r, _ := s.ReportEmpty(models.Oslo, "Snacks", now)
	if _, err := s.CompletePurchase(r.ID, "v1", 6001, 10, "receipt", now); err == nil {
		t.Fatal("expected budget check to reject purchase")
	}
}

func TestReimbursementSubmittedInPayrollCurrency(t *testing.T) {
	s := newService()
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	addVolunteer(t, s, "v1", models.Bengaluru, models.INR)
	r, _ := s.ReportEmpty(models.Bengaluru, "Tea", now)
	p, err := s.CompletePurchase(r.ID, "v1", 500, 2, "receipt", now)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.SubmitReimbursement(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claim.ReimbursementID == "" || claim.ReimbursementCurrency != models.INR {
		t.Fatal("claim was not submitted in payroll currency")
	}
	stored, _ := s.GetRefill(r.ID)
	if stored.Status != models.StatusSubmitted {
		t.Fatalf("expected submitted status, got %s", stored.Status)
	}
}
