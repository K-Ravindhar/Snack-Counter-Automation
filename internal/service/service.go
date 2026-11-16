package service

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"snack-counter-automation/internal/integrations"
	"snack-counter-automation/internal/models"
)

type Service struct {
	mu         sync.Mutex
	volunteers map[string]*models.Volunteer
	refills    map[string]*models.Refill
	purchases  map[string]*models.Purchase
	budgets    map[models.Office]models.OfficeBudget
	chat       integrations.Chat
	hr         integrations.HRClient
	fx         integrations.FXConverter
	sequence   int
	claimTTL   time.Duration
}

func New(chat integrations.Chat, hr integrations.HRClient, fx integrations.FXConverter) *Service {
	return &Service{
		volunteers: make(map[string]*models.Volunteer), refills: make(map[string]*models.Refill),
		purchases: make(map[string]*models.Purchase),
		budgets: map[models.Office]models.OfficeBudget{
			models.Oslo:      {Office: models.Oslo, Currency: models.NOK, Monthly: 6000},
			models.Bengaluru: {Office: models.Bengaluru, Currency: models.INR, Monthly: 15000},
		},
		chat: chat, hr: hr, fx: fx, claimTTL: 24 * time.Hour,
	}
}

func (s *Service) AddVolunteer(v models.Volunteer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == "" || v.Name == "" {
		return fmt.Errorf("volunteer ID and name are required")
	}
	if _, exists := s.volunteers[v.ID]; exists {
		return fmt.Errorf("volunteer %s already exists", v.ID)
	}
	if v.CurrentOffice == "" {
		v.CurrentOffice = v.HomeOffice
	}
	copy := v
	s.volunteers[v.ID] = &copy
	return nil
}

func (s *Service) SetAvailability(id string, available bool, currentOffice models.Office) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.volunteers[id]
	if !ok {
		return fmt.Errorf("volunteer not found")
	}
	v.Available = available
	if currentOffice != "" {
		v.CurrentOffice = currentOffice
	}
	return nil
}

// ReportEmpty creates one refill task and assigns the fairest eligible volunteer.
func (s *Service) ReportEmpty(office models.Office, snack string, now time.Time) (models.Refill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.budgets[office]; !ok {
		return models.Refill{}, fmt.Errorf("unknown office")
	}
	if snack == "" {
		return models.Refill{}, fmt.Errorf("snack is required")
	}
	s.sequence++
	r := &models.Refill{ID: fmt.Sprintf("REF-%04d", s.sequence), Office: office, Snack: snack, Status: models.StatusOpen, CreatedAt: now}
	s.refills[r.ID] = r
	s.assignLocked(r, now)
	return *r, nil
}

// ClaimNext is atomic: concurrent volunteers cannot acquire the same refill.
// The first available volunteer selected by the fairness policy is assigned.
func (s *Service) ClaimNext(refillID string, now time.Time) (models.Refill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refills[refillID]
	if !ok {
		return models.Refill{}, fmt.Errorf("refill not found")
	}
	if r.Status != models.StatusOpen {
		return models.Refill{}, fmt.Errorf("refill is not open")
	}
	s.assignLocked(r, now)
	if r.Status != models.StatusAssigned {
		return *r, fmt.Errorf("no eligible volunteer available")
	}
	return *r, nil
}

func (s *Service) assignLocked(r *models.Refill, now time.Time) {
	tried := make(map[string]bool, len(r.TriedVolunteerIDs))
	for _, id := range r.TriedVolunteerIDs {
		tried[id] = true
	}
	candidates := make([]*models.Volunteer, 0)
	for _, v := range s.volunteers {
		if v.Available && v.CurrentOffice == r.Office && !tried[v.ID] {
			candidates = append(candidates, v)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.AssignmentCount != b.AssignmentCount {
			return a.AssignmentCount < b.AssignmentCount
		}
		if !a.LastAssignedAt.Equal(b.LastAssignedAt) {
			return a.LastAssignedAt.Before(b.LastAssignedAt)
		}
		return a.ID < b.ID
	})
	if len(candidates) == 0 {
		r.Status = models.StatusOpen
		r.AssignedVolunteer = ""
		if s.chat != nil {
			_ = s.chat.Broadcast(fmt.Sprintf("Refill %s for %s needs a volunteer", r.ID, r.Office))
		}
		return
	}
	v := candidates[0]
	r.Status = models.StatusAssigned
	r.AssignedVolunteer = v.ID
	r.AssignedAt = now
	r.Deadline = now.Add(s.claimTTL)
	r.TriedVolunteerIDs = append(r.TriedVolunteerIDs, v.ID)
	v.AssignmentCount++
	v.LastAssignedAt = now
	if s.chat != nil {
		_ = s.chat.Notify(v.ID, fmt.Sprintf("Please refill %s at %s. Complete by %s", r.Snack, r.Office, r.Deadline.Format(time.RFC3339)))
	}
}

// ReassignExpired hands expired tasks to a different eligible volunteer.
func (s *Service) ReassignExpired(now time.Time) []models.Refill {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := make([]models.Refill, 0)
	for _, r := range s.refills {
		if r.Status == models.StatusAssigned && !now.Before(r.Deadline) {
			r.Status = models.StatusOpen
			r.AssignedVolunteer = ""
			s.assignLocked(r, now)
			changed = append(changed, *r)
		}
	}
	return changed
}

// CompletePurchase records the actual office-currency spend and reserves it against that month's budget.
func (s *Service) CompletePurchase(refillID, volunteerID string, amount float64, quantity int, receiptURL string, now time.Time) (models.Purchase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refills[refillID]
	if !ok {
		return models.Purchase{}, fmt.Errorf("refill not found")
	}
	if r.Status != models.StatusAssigned || r.AssignedVolunteer != volunteerID {
		return models.Purchase{}, fmt.Errorf("volunteer is not assigned to this refill")
	}
	if amount <= 0 || quantity <= 0 || receiptURL == "" {
		return models.Purchase{}, fmt.Errorf("amount, quantity and receipt are required")
	}
	v := s.volunteers[volunteerID]
	budget := s.budgets[r.Office]
	if amount+1e-9 > s.remainingLocked(r.Office, now) {
		return models.Purchase{}, fmt.Errorf("monthly %s budget exceeded", budget.Currency)
	}
	s.sequence++
	p := &models.Purchase{ID: fmt.Sprintf("PUR-%04d", s.sequence), RefillID: r.ID, Office: r.Office, VolunteerID: volunteerID, Snack: r.Snack, Quantity: quantity, Amount: amount, Currency: budget.Currency, PurchasedAt: now, ReceiptURL: receiptURL, Description: fmt.Sprintf("Snack refill: %s (%d units) at %s", r.Snack, quantity, r.Office), ReimbursementCurrency: v.PayrollCurrency}
	s.purchases[p.ID] = p
	r.PurchaseID = p.ID
	r.Status = models.StatusPurchased
	return *p, nil
}

func (s *Service) SubmitReimbursement(purchaseID string) (models.Purchase, error) {
	s.mu.Lock()
	p, ok := s.purchases[purchaseID]
	if !ok {
		s.mu.Unlock()
		return models.Purchase{}, fmt.Errorf("purchase not found")
	}
	if p.ReimbursementID != "" {
		s.mu.Unlock()
		return models.Purchase{}, fmt.Errorf("reimbursement already submitted")
	}
	volunteerID := p.VolunteerID
	amount, from, to, at := p.Amount, string(p.Currency), string(p.ReimbursementCurrency), p.PurchasedAt
	hr, fx := s.hr, s.fx
	s.mu.Unlock()
	if hr == nil || fx == nil {
		return models.Purchase{}, fmt.Errorf("HR and FX integrations are required")
	}
	converted, err := fx.Convert(amount, from, to, at.Format(time.RFC3339))
	if err != nil {
		return models.Purchase{}, err
	}
	claimID, err := hr.SubmitClaim(volunteerID, converted, to, p.Description, p.ReceiptURL)
	if err != nil {
		return models.Purchase{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Recheck after external call to avoid recording duplicate submissions from concurrent callers.
	p = s.purchases[purchaseID]
	if p.ReimbursementID != "" {
		return models.Purchase{}, fmt.Errorf("reimbursement already submitted")
	}
	p.ReimbursementID = claimID
	p.ReimbursementAmount = converted
	for _, r := range s.refills {
		if r.PurchaseID == purchaseID {
			r.Status = models.StatusSubmitted
			break
		}
	}
	return *p, nil
}

func (s *Service) MarkReimbursed(purchaseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.purchases[purchaseID]
	if !ok {
		return fmt.Errorf("purchase not found")
	}
	if p.ReimbursementID == "" {
		return fmt.Errorf("claim has not been submitted")
	}
	for _, r := range s.refills {
		if r.PurchaseID == purchaseID {
			r.Status = models.StatusReimbursed
			return nil
		}
	}
	return fmt.Errorf("refill not found")
}

func (s *Service) remainingLocked(office models.Office, now time.Time) float64 {
	b := s.budgets[office]
	spent := 0.0
	for _, p := range s.purchases {
		if p.Office == office && p.PurchasedAt.Year() == now.Year() && p.PurchasedAt.Month() == now.Month() {
			spent += p.Amount
		}
	}
	return b.Monthly - spent
}

func (s *Service) RemainingBudget(office models.Office, now time.Time) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remainingLocked(office, now)
}

// MonthlyReport keeps currencies separate; it never adds NOK and INR as if they were equivalent.
func (s *Service) MonthlyReport(year int, month time.Month) models.MonthlyReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := models.MonthlyReport{Month: fmt.Sprintf("%04d-%02d", year, month), ByOffice: make(map[models.Office]float64), ByCurrency: make(map[models.Currency]float64), Budgets: make(map[models.Office]float64), Remaining: make(map[models.Office]float64)}
	for office, b := range s.budgets {
		report.Budgets[office] = b.Monthly
		report.ByOffice[office] = 0
	}
	for _, p := range s.purchases {
		if p.PurchasedAt.Year() == year && p.PurchasedAt.Month() == month {
			report.ByOffice[p.Office] += p.Amount
			report.ByCurrency[p.Currency] += p.Amount
		}
	}
	for office, b := range s.budgets {
		report.Remaining[office] = b.Monthly - report.ByOffice[office]
	}
	return report
}

func (s *Service) GetRefill(id string) (models.Refill, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.refills[id]
	if !ok {
		return models.Refill{}, false
	}
	return *r, true
}
